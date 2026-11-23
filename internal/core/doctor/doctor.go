// Package doctor checks that this machine's agent setup matches the repo.
//
// It exists because every one of these problems was found by hand: dangling
// links, hooks calling retired commands, agents pinned to retired models,
// skills without SKILL.md, live settings drifting from the repo. Run is
// read-only and offline; Fix is the only thing that writes.
package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/core/manifest"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	avalsync "github.com/alvaroofernaandez/aval/internal/core/sync"
)

// Status is the outcome of one check.
type Status string

const (
	OK   Status = "ok"
	Warn Status = "warn"
	Fail Status = "fail"
)

// Result is one check's verdict.
type Result struct {
	ID      string `json:"id"`
	Status  Status `json:"status"`
	Message string `json:"message"`
	Fixable bool   `json:"fixable"`
	// Details is the full list behind Message, which truncates long lists.
	Details []string `json:"-"`
	// Fix says which repair, if any, the hub offers for this result.
	Fix FixKind `json:"-"`
}

// FixKind says how a result can be repaired from the hub.
type FixKind int

const (
	FixNone FixKind = iota
	FixBrokenLinks
	FixSync
)

// Options configures a run.
type Options struct {
	Repo       string
	Home       string
	PrivateDir string
	GOOS       string
	// Security enables the extra checks of `aval doctor --security`.
	Security bool
	// Allow lists suppressed check ids ("id" or "id:subject").
	Allow []string
	// LookPath resolves a bare command name; nil means exec.LookPath.
	LookPath func(string) (string, error)
	// ConfigPath is aval's config file; empty skips the config check.
	ConfigPath string
}

// brokenLinkDirs are scanned one level deep for dangling symlinks.
var brokenLinkDirs = []string{
	".claude/skills", ".claude/agents", ".claude/commands", ".claude/hooks", ".claude",
	".config/opencode/skills", ".opencode/skills", ".agents/skills", ".local/bin",
}

// Run executes every check. It never writes.
func Run(opt Options) []Result {
	live, liveErr := readJSON(filepath.Join(opt.Home, ".claude", "settings.json"))
	rs := []Result{
		checkLinks(opt),
		checkBroken(opt),
	}
	rs = append(rs, checkHooks(opt, live, liveErr)...)
	rs = append(rs,
		checkAgents(opt),
		checkSkills(opt),
		checkPlugins(opt, live, liveErr),
		checkSettingsDrift(opt),
	)
	rs = append(rs, checkOverlay(opt)...)
	if opt.ConfigPath != "" {
		rs = append(rs, checkConfig(opt))
	}
	if opt.Security {
		rs = append(rs, checkSecurity(opt)...)
	}
	for i := range rs {
		rs[i].Fix = fixKind(rs[i])
	}
	return rs
}

// fixKind maps a fixable result to the repair that clears it.
func fixKind(r Result) FixKind {
	switch {
	case !r.Fixable:
		return FixNone
	case r.ID == "broken-links":
		return FixBrokenLinks
	}
	return FixSync
}

// RunOnly runs every check and keeps the ones whose ID equals, or starts
// with, one of ids ("overlay" also keeps "overlay-links"). It never writes.
func RunOnly(opt Options, ids ...string) []Result {
	var out []Result
	for _, r := range Run(opt) {
		for _, id := range ids {
			if r.ID == id || strings.HasPrefix(r.ID, id+"-") {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// Fix removes broken links, then runs sync for the drift it can repair.
func Fix(opt Options) ([]string, error) {
	var removed []string
	for _, p := range brokenLinks(opt) {
		if err := os.Remove(p); err != nil {
			return removed, fmt.Errorf("doctor: remove %s: %w", p, err)
		}
		removed = append(removed, p)
	}
	_, err := avalsync.Run(avalsync.Options{Repo: opt.Repo, Home: opt.Home, PrivateDir: opt.PrivateDir, GOOS: opt.GOOS})
	return removed, err
}

// RemoveBroken removes every dangling link, snapshotting each into rec first
// so backup.Store.Restore can put it back. The caller owns rec and commits it.
// The sync half of Fix is deliberately absent: txsync imports doctor.
func RemoveBroken(opt Options, rec *backup.Recorder) ([]string, error) {
	var removed []string
	rec.SetOrigin("doctor")
	for _, p := range brokenLinks(opt) {
		if _, err := rec.Snapshot(p); err != nil {
			return removed, fmt.Errorf("doctor: back up %s: %w", p, err)
		}
		if err := os.Remove(p); err != nil {
			return removed, fmt.Errorf("doctor: remove %s: %w", p, err)
		}
		removed = append(removed, p)
	}
	return removed, nil
}

// HasFail reports whether any result failed.
func HasFail(rs []Result) bool {
	for _, r := range rs {
		if r.Status == Fail {
			return true
		}
	}
	return false
}

// Render prints the aligned table and the summary line.
func Render(w io.Writer, rs []Result) { RenderStyled(w, rs, nil) }

// Styler decorates a status word for a terminal. The doctor package owns the
// interface so internal/core never imports the UI layer.
type Styler interface {
	OK(word string) string
	Warn(word string) string
	Fail(word string) string
	// Width is the visible width of a styled cell (escape codes excluded).
	Width(s string) int
}

// Text is the wording of the human table: the column headings, the word for
// each status, the hint on a fixable row and the summary line. The package
// holds English only; a caller that speaks another language passes its own,
// so internal/core never imports the catalog. The message of each check is
// data from the checks and is printed as it is.
type Text struct {
	Status, Check, Message string
	// Word is the status cell before styling.
	Word func(Status) string
	// Fixable is appended to a fixable row that is not OK, with its space.
	Fixable string
	// Summary renders the line under the table.
	Summary func(ok, warn, fail int) string
}

// EnglishText is the table's wording when no language is given.
func EnglishText() Text {
	return Text{
		Status: "STATUS", Check: "CHECK", Message: "MESSAGE",
		Word:    func(s Status) string { return strings.ToUpper(string(s)) },
		Fixable: " (fixable: aval doctor --fix)",
		Summary: func(ok, warn, fail int) string { return fmt.Sprintf("%d ok, %d warn, %d fail", ok, warn, fail) },
	}
}

// RenderStyled is Render with an optional Styler. A nil Styler prints the
// plain, byte-stable table scripts rely on.
func RenderStyled(w io.Writer, rs []Result, st Styler) {
	RenderText(w, rs, st, EnglishText())
}

// RenderText is RenderStyled with the table's wording given.
func RenderText(w io.Writer, rs []Result, st Styler, tx Text) {
	width := utf8.RuneCountInString(tx.Check)
	for _, r := range rs {
		width = max(width, utf8.RuneCountInString(r.ID))
	}
	cells := make([]string, len(rs))
	statusWidth := utf8.RuneCountInString(tx.Status)
	for i, r := range rs {
		cells[i] = statusCell(r.Status, st, tx)
		if st != nil {
			statusWidth = max(statusWidth, st.Width(cells[i]))
		} else {
			statusWidth = max(statusWidth, utf8.RuneCountInString(cells[i]))
		}
	}
	pad := func(cell string) string {
		n := statusWidth - utf8.RuneCountInString(cell)
		if st != nil {
			n = statusWidth - st.Width(cell)
		}
		return cell + strings.Repeat(" ", max(n, 0))
	}
	fmt.Fprintf(w, "%s  %-*s  %s\n", pad(tx.Status), width, tx.Check, tx.Message)
	var ok, warn, fail int
	for i, r := range rs {
		switch r.Status {
		case OK:
			ok++
		case Warn:
			warn++
		case Fail:
			fail++
		}
		msg := r.Message
		if r.Fixable && r.Status != OK {
			msg += tx.Fixable
		}
		fmt.Fprintf(w, "%s  %-*s  %s\n", pad(cells[i]), width, r.ID, msg)
	}
	fmt.Fprintf(w, "\n%s\n", tx.Summary(ok, warn, fail))
}

// statusCell is the status word, decorated when a Styler is given.
func statusCell(s Status, st Styler, tx Text) string {
	word := tx.Word(s)
	if st == nil {
		return word
	}
	switch s {
	case OK:
		return st.OK(word)
	case Warn:
		return st.Warn(word)
	case Fail:
		return st.Fail(word)
	}
	return word
}

// RenderJSON prints the results as a JSON array.
func RenderJSON(w io.Writer, rs []Result) error {
	if rs == nil {
		rs = []Result{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rs)
}

func result(id string, st Status, fixable bool, format string, a ...any) Result {
	return Result{ID: id, Status: st, Message: fmt.Sprintf(format, a...), Fixable: fixable}
}

// withDetails attaches the untruncated items behind a result's message.
func withDetails(r Result, items []string) Result {
	r.Details = items
	return r
}

// list shortens a long list for a one-line message.
func list(items []string) string {
	const max = 4
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(", … +%d more", len(items)-max)
}

func readJSON(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// --- links -----------------------------------------------------------------

type linkProblem struct {
	label string
	hard  bool // missing or dangling, as opposed to replaced or elsewhere
	what  string
}

func tilde(home, p string) string {
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + filepath.ToSlash(rel)
	}
	return p
}

// checkLink verifies one destination is a symlink that resolves under root.
func checkLink(home, root string, a plan.Action) *linkProblem {
	label := tilde(home, a.Dest)
	fi, err := os.Lstat(a.Dest)
	if err != nil {
		return &linkProblem{label, true, "missing"}
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		// Copy mode (Windows without symlink rights): a copy is healthy while
		// it still equals the repo.
		if backup.SameContent(a.Source, a.Dest) {
			return nil
		}
		return &linkProblem{label, false, "replaced by a real file or directory"}
	}
	resolved, err := filepath.EvalSymlinks(a.Dest)
	if err != nil {
		return &linkProblem{label, true, "dangling"}
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		realRoot = root
	}
	if resolved != realRoot && !strings.HasPrefix(resolved, realRoot+string(filepath.Separator)) {
		return &linkProblem{label, false, "points outside " + filepath.Base(root)}
	}
	return nil
}

func summariseLinks(id string, total int, probs []*linkProblem, what string) Result {
	if len(probs) == 0 {
		return result(id, OK, false, "%d %s in place", total, what)
	}
	st := Warn
	var parts []string
	for _, p := range probs {
		if p.hard {
			st = Fail
		}
		parts = append(parts, p.label+" ("+p.what+")")
	}
	return withDetails(result(id, st, false, "%d of %d %s need attention: %s", len(probs), total, what, list(parts)), parts)
}

func checkLinks(opt Options) Result {
	m, err := manifest.Load(filepath.Join(opt.Repo, "install.manifest"))
	if err != nil {
		return result("links", Fail, false, "cannot read install.manifest: %v", err)
	}
	p, err := plan.Build(m, plan.Options{Repo: opt.Repo, Home: opt.Home, GOOS: opt.GOOS})
	if err != nil {
		return result("links", Fail, false, "%v", err)
	}
	// settings.json is a real file by design: sync owns it (see internal/core/sync).
	managed := filepath.Join(opt.Home, ".claude", "settings.json")
	var probs []*linkProblem
	total := 0
	for _, g := range p.Groups {
		for _, a := range g.Actions {
			if a.Dest == managed {
				continue
			}
			total++
			if lp := checkLink(opt.Home, opt.Repo, a); lp != nil {
				probs = append(probs, lp)
			}
		}
	}
	return summariseLinks("links", total, probs, "manifest links")
}

// --- broken links -----------------------------------------------------------

func brokenLinks(opt Options) []string {
	var out []string
	for _, rel := range brokenLinkDirs {
		dir := filepath.Join(opt.Home, filepath.FromSlash(rel))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Type()&os.ModeSymlink == 0 {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if _, err := os.Stat(p); err != nil {
				out = append(out, p)
			}
		}
	}
	return out
}

func checkBroken(opt Options) Result {
	broken := brokenLinks(opt)
	if len(broken) == 0 {
		return result("broken-links", OK, false, "no dangling links in agent directories")
	}
	labels := make([]string, len(broken))
	for i, p := range broken {
		labels[i] = tilde(opt.Home, p)
	}
	return withDetails(result("broken-links", Fail, true, "%d broken: %s", len(broken), list(labels)), labels)
}

// --- hooks ------------------------------------------------------------------

type hookCmd struct {
	event, command string
	// prog is set for the exec form (command + args, no shell): it is the
	// program itself, not a shell line to parse.
	prog string
}

func hookCommands(settings map[string]any) []hookCmd {
	var out []hookCmd
	hooks, _ := settings["hooks"].(map[string]any)
	for event, groups := range hooks {
		gl, _ := groups.([]any)
		for _, g := range gl {
			gm, _ := g.(map[string]any)
			hl, _ := gm["hooks"].([]any)
			for _, h := range hl {
				hm, _ := h.(map[string]any)
				if c, ok := hm["command"].(string); ok {
					hc := hookCmd{event: event, command: c}
					if args, ok := hm["args"].([]any); ok {
						hc.prog = c
						for _, a := range args {
							if as, ok := a.(string); ok {
								hc.command += " " + as
							}
						}
					}
					out = append(out, hc)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].event != out[j].event {
			return out[i].event < out[j].event
		}
		return out[i].command < out[j].command
	})
	return out
}

func checkHooks(opt Options, live map[string]any, liveErr error) []Result {
	if liveErr != nil {
		msg := "~/.claude/settings.json is missing"
		if !os.IsNotExist(liveErr) {
			msg = "~/.claude/settings.json is unreadable: " + liveErr.Error()
		}
		return []Result{
			result("hooks-resolve", Warn, true, "%s; hooks not checked", msg),
			result("hooks-retired", Warn, true, "%s; hooks not checked", msg),
			result("hooks-drift", Fail, true, "%s", msg),
		}
	}
	cmds := hookCommands(live)
	lookPath := opt.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	var unresolved, retired []string
	for _, h := range cmds {
		exes := executables(h.command, opt.Home)
		if h.prog != "" {
			exes = []string{expand(h.prog, opt.Home)}
		}
		for _, ex := range exes {
			if !resolves(ex, opt.Home, lookPath) {
				unresolved = append(unresolved, h.event+": "+tilde(opt.Home, ex))
			}
		}
		if strings.Contains(h.command, "sdd-") || strings.Contains(h.command, "skill-registry") {
			retired = append(retired, h.event+": "+h.command)
		}
	}

	var rs []Result
	if len(unresolved) == 0 {
		rs = append(rs, result("hooks-resolve", OK, false, "%d hook commands resolve", len(cmds)))
	} else {
		rs = append(rs, withDetails(result("hooks-resolve", Fail, false, "%d unresolved: %s", len(unresolved), list(unresolved)), unresolved))
	}
	if len(retired) == 0 {
		rs = append(rs, result("hooks-retired", OK, false, "no retired SDD hooks"))
	} else {
		rs = append(rs, withDetails(result("hooks-retired", Fail, false, "%d retired: %s", len(retired), list(retired)), retired))
	}

	repo, err := avalsync.RepoSettings(avalsync.Options{Repo: opt.Repo, Home: opt.Home, GOOS: opt.GOOS})
	if err != nil {
		return append(rs, result("hooks-drift", Fail, false, "cannot read repo settings: %v", err))
	}
	want, got := map[string]int{}, map[string]int{}
	for _, h := range hookCommands(repo) {
		want[h.event+" "+h.command]++
	}
	for _, h := range cmds {
		got[h.event+" "+h.command]++
	}
	var missing, extra int
	for k, n := range want {
		if got[k] < n {
			missing += n - got[k]
		}
	}
	for k, n := range got {
		if want[k] < n {
			extra += n - want[k]
		}
	}
	if missing == 0 && extra == 0 {
		return append(rs, result("hooks-drift", OK, false, "live hooks match the repo"))
	}
	return append(rs, result("hooks-drift", Fail, true, "live hooks differ from the repo (%d missing, %d extra)", missing, extra))
}

func resolves(word, home string, lookPath func(string) (string, error)) bool {
	if strings.ContainsAny(word, `/\`) {
		_, err := os.Stat(word)
		return err == nil
	}
	if _, err := lookPath(word); err == nil {
		return true
	}
	// Hooks run from Claude's PATH, which usually includes ~/.local/bin even
	// when the shell running the doctor does not.
	_, err := os.Stat(filepath.Join(home, ".local", "bin", word))
	return err == nil
}

// --- agents and skills --------------------------------------------------------

// frontmatter returns the top-level keys of a leading --- block.
func frontmatter(content string) map[string]string {
	content = strings.TrimPrefix(content, "\xef\xbb\xbf")
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}
	out := map[string]string{}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			return out
		}
		if l == "" || l[0] == ' ' || l[0] == '\t' || l[0] == '#' {
			continue
		}
		if k, v, ok := strings.Cut(l, ":"); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return nil // never closed
}

var modelAliases = map[string]bool{"opus": true, "sonnet": true, "haiku": true, "inherit": true}

func checkAgents(opt Options) Result {
	dir := filepath.Join(opt.Home, ".claude", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return result("agents", OK, false, "no ~/.claude/agents directory")
	}
	var bad []string
	n := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		n++
		model, ok := frontmatter(string(b))["model"]
		if !ok {
			continue
		}
		model = strings.Trim(model, `"'`)
		if !modelAliases[model] {
			bad = append(bad, fmt.Sprintf("%s (model: %s)", e.Name(), model))
		}
	}
	if len(bad) > 0 {
		return withDetails(result("agents", Fail, false, "%d agent(s) pin a model that is not opus|sonnet|haiku|inherit: %s", len(bad), list(bad)), bad)
	}
	return result("agents", OK, false, "%d agents use model aliases", n)
}

func checkSkills(opt Options) Result {
	dir := filepath.Join(opt.Home, ".claude", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return result("skills", OK, false, "no ~/.claude/skills directory")
	}
	var bad []string
	n := 0
	for _, e := range entries {
		// Not skills: "_name" support dirs, and Claude Code's state for
		// claude.ai-synced skills.
		if strings.HasPrefix(e.Name(), "_") || e.Name() == "synced" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		fi, err := os.Stat(p) // follows links; dangling ones are the broken-links check's job
		if err != nil || !fi.IsDir() {
			continue
		}
		n++
		b, err := os.ReadFile(filepath.Join(p, "SKILL.md"))
		if err != nil {
			bad = append(bad, e.Name()+" (no SKILL.md)")
			continue
		}
		fm := frontmatter(string(b))
		_, hasName := fm["name"]
		_, hasDesc := fm["description"]
		if !hasName || !hasDesc {
			bad = append(bad, e.Name()+" (frontmatter needs name and description)")
		}
	}
	if len(bad) > 0 {
		return withDetails(result("skills", Warn, false, "%d skill(s) incomplete: %s", len(bad), list(bad)), bad)
	}
	return result("skills", OK, false, "%d skills have SKILL.md with name and description", n)
}

// --- plugins and settings ------------------------------------------------------

func checkPlugins(opt Options, live map[string]any, liveErr error) Result {
	if liveErr != nil {
		return result("plugins", OK, false, "no live settings.json to check")
	}
	enabled, _ := live["enabledPlugins"].(map[string]any)
	var want []string
	for k, v := range enabled {
		if b, _ := v.(bool); b {
			want = append(want, k)
		}
	}
	sort.Strings(want)
	if len(want) == 0 {
		return result("plugins", OK, false, "no plugins enabled")
	}
	inst, err := readJSON(filepath.Join(opt.Home, ".claude", "plugins", "installed_plugins.json"))
	if err != nil {
		return result("plugins", Warn, false, "%d plugins enabled but installed_plugins.json is unreadable", len(want))
	}
	installed, _ := inst["plugins"].(map[string]any)
	var missing []string
	for _, k := range want {
		if _, ok := installed[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return withDetails(result("plugins", Warn, false, "%d enabled but not installed: %s", len(missing), list(missing)), missing)
	}
	return result("plugins", OK, false, "%d enabled plugins are installed", len(want))
}

func checkSettingsDrift(opt Options) Result {
	keys, err := avalsync.SettingsDrift(avalsync.Options{Repo: opt.Repo, Home: opt.Home, PrivateDir: opt.PrivateDir, GOOS: opt.GOOS})
	if err != nil {
		return result("settings-drift", Fail, false, "%v", err)
	}
	if len(keys) == 0 {
		return result("settings-drift", OK, false, "managed settings keys match the repo")
	}
	return withDetails(result("settings-drift", Fail, true, "live settings differ in: %s", strings.Join(keys, ", ")), keys)
}

// --- private overlay -------------------------------------------------------------

func checkOverlay(opt Options) []Result {
	if opt.PrivateDir == "" {
		return []Result{result("overlay", OK, false, "absent (optional)")}
	}
	if fi, err := os.Stat(opt.PrivateDir); err != nil || !fi.IsDir() {
		return []Result{result("overlay", OK, false, "absent (optional): %s", tilde(opt.Home, opt.PrivateDir))}
	}
	var have []string
	for _, f := range []string{"settings.json", "links.manifest"} {
		if _, err := os.Stat(filepath.Join(opt.PrivateDir, f)); err == nil {
			have = append(have, f)
		}
	}
	msg := "present, empty"
	if len(have) > 0 {
		msg = "present (" + strings.Join(have, ", ") + ")"
	}
	rs := []Result{result("overlay", OK, false, "%s", msg)}

	actions, err := avalsync.PrivateLinks(opt.PrivateDir, opt.Home)
	if err != nil {
		return append(rs, result("overlay-links", Fail, false, "%v", err))
	}
	if len(actions) == 0 {
		return rs
	}
	var probs []*linkProblem
	for _, a := range actions {
		if _, err := os.Stat(a.Source); err != nil {
			probs = append(probs, &linkProblem{tilde(opt.Home, a.Dest), true, "source missing in the overlay"})
			continue
		}
		if lp := checkLink(opt.Home, opt.PrivateDir, a); lp != nil {
			probs = append(probs, lp)
		}
	}
	return append(rs, summariseLinks("overlay-links", len(actions), probs, "overlay links"))
}

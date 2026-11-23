// Package sync keeps the live agent configuration aligned with the repo.
//
// Claude Code and gentle-ai both rewrite ~/.claude/settings.json, and it holds
// personal keys that cannot be published, so it is a real file rather than a
// link to the repo. sync is how the repo-owned part of it stays authoritative:
// it replaces or merges only the managed keys and leaves everything else alone.
package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/manifest"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
)

// Options configures a run. Everything is a parameter so tests can point at a
// fake HOME and a fake repo.
type Options struct {
	Repo       string
	Home       string
	PrivateDir string
	DryRun     bool
	// GOOS selects the hook flavour (see ForOS); empty means runtime.GOOS.
	GOOS string
	// AvalBin is the executable Windows hooks run; empty means ResolveAvalBin.
	AvalBin string
	// Now stamps the backup directory; nil means time.Now.
	Now func() time.Time
	// Recorder receives everything sync overwrites or creates. Nil means Run
	// opens its own backup run and commits it; a caller that passes one (the
	// transactional sync) owns the commit.
	Recorder *backup.Recorder
}

// SettingsResult describes what happened to ~/.claude/settings.json.
type SettingsResult struct {
	// Changed lists the top-level keys that differ (or would, on a dry run).
	Changed []string
	Created bool
	Written bool
	Backup  string
	// Warnings are overlay entries sync refused to apply.
	Warnings []string
}

// RulesResult describes one agent-rules target.
type RulesResult struct {
	Path    string
	Changed bool
	Skipped bool
}

// Report is the outcome of Run.
type Report struct {
	Settings SettingsResult
	Rules    []RulesResult
	Links    []LinkResult
}

// replaceKeys are owned by the repo: the live value is overwritten.
var replaceKeys = []string{"hooks", "attribution", "worktree", "spinnerTipsEnabled", "subagentPromptCacheTtl", "statusLine"}

// mergeKeys are maps where the repo wins per entry but live-only entries
// survive (plugins and overrides added by hand or by a marketplace).
var mergeKeys = []string{"env", "enabledPlugins", "skillOverrides", "extraKnownMarketplaces"}

// MergeSettings merges repo and then overlay into live and returns the result
// plus the sorted top-level keys whose value changed. Inputs are not mutated.
func MergeSettings(repo, live, overlay map[string]any) (map[string]any, []string) {
	out, changed, _ := MergeSettingsWarn(repo, live, overlay)
	return out, changed
}

// MergeSettingsWarn is MergeSettings plus the warnings it raised.
//
// permissions.deny is a safety floor, not a preference: the result is the union
// of the repo's denies, the live file's own and the overlay's, so the overlay
// can add denies but never remove or replace one. Overlay permissions.allow
// entries are added to the live ones, except those a deny covers: those are
// dropped with a warning instead of silently widening access.
func MergeSettingsWarn(repo, live, overlay map[string]any) (map[string]any, []string, []string) {
	rest, overlayDeny, overlayAllow := splitOverlayPermissions(overlay)
	out, _ := mergeSettings(repo, live, rest)
	warnings := mergePermissions(out, repo, overlayDeny, overlayAllow)

	var changed []string
	for k, v := range out {
		if old, ok := live[k]; !ok || !reflect.DeepEqual(old, v) {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	return out, changed, warnings
}

// splitOverlayPermissions returns the overlay without permissions.deny and
// permissions.allow (which merge by union, not by replacement) plus those two
// lists. The input is not mutated.
func splitOverlayPermissions(overlay map[string]any) (map[string]any, []string, []string) {
	perms, ok := overlay["permissions"].(map[string]any)
	if !ok {
		return overlay, nil, nil
	}
	rest := cloneMap(overlay)
	rp := rest["permissions"].(map[string]any)
	deny, allow := stringList(perms["deny"]), stringList(perms["allow"])
	delete(rp, "deny")
	delete(rp, "allow")
	if len(rp) == 0 {
		delete(rest, "permissions")
	}
	return rest, deny, allow
}

func stringList(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, x := range l {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func union(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range lists {
		for _, s := range l {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

func toAny(l []string) []any {
	out := make([]any, len(l))
	for i, s := range l {
		out[i] = s
	}
	return out
}

func mergePermissions(out, repo map[string]any, overlayDeny, overlayAllow []string) []string {
	var repoDeny []string
	if rp, ok := repo["permissions"].(map[string]any); ok {
		repoDeny = stringList(rp["deny"])
	}
	if len(repoDeny) == 0 && len(overlayDeny) == 0 && len(overlayAllow) == 0 {
		return nil
	}
	perms, _ := out["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	deny := union(repoDeny, stringList(perms["deny"]), overlayDeny)
	if len(deny) > 0 {
		perms["deny"] = toAny(deny)
	}

	var warnings []string
	if len(overlayAllow) > 0 {
		var add []string
		for _, a := range overlayAllow {
			if d, hit := coveredByDeny(a, deny); hit {
				warnings = append(warnings, fmt.Sprintf("permissions: overlay allow %q dropped, denied by %q", a, d))
				continue
			}
			add = append(add, a)
		}
		if merged := union(stringList(perms["allow"]), add); len(merged) > 0 {
			perms["allow"] = toAny(merged)
		}
	}
	if len(perms) > 0 {
		out["permissions"] = perms
	}
	return warnings
}

// coveredByDeny reports whether a deny rule matches the allow rule: the same
// rule, a bare tool name denying every use of it, or a deny glob that matches
// the allow rule's argument ("Read(~/.ssh/**)" covers "Read(~/.ssh/id_rsa)").
func coveredByDeny(allow string, deny []string) (string, bool) {
	at, aa := splitRule(allow)
	for _, d := range deny {
		if d == allow {
			return d, true
		}
		dt, da := splitRule(d)
		if dt != at {
			continue
		}
		if da == "" {
			return d, true
		}
		if aa != "" && ruleGlob(at, da).MatchString(aa) {
			return d, true
		}
	}
	return "", false
}

// splitRule splits "Tool(arg)" into "Tool" and "arg"; a bare "Tool" has no arg.
func splitRule(r string) (tool, arg string) {
	tool, rest, ok := strings.Cut(r, "(")
	if !ok {
		return r, ""
	}
	arg = strings.TrimSuffix(rest, ")")
	return tool, strings.TrimPrefix(arg, "./")
}

// ruleGlob compiles a permission glob: ** crosses directories, "**/" also
// matches none, and * stops at "/" for file tools but not for Bash commands.
func ruleGlob(tool, pattern string) *regexp.Regexp {
	pattern = strings.TrimPrefix(pattern, "./")
	star := "[^/]*"
	if tool == "Bash" {
		star = ".*"
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i++
		case pattern[i] == '*':
			b.WriteString(star)
		default:
			// Whole characters, so a non-ASCII literal matches itself.
			r, size := utf8.DecodeRuneInString(pattern[i:])
			b.WriteString(regexp.QuoteMeta(string(r)))
			i += size - 1
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func mergeSettings(repo, live, overlay map[string]any) (map[string]any, []string) {
	out := cloneMap(live)
	for _, k := range replaceKeys {
		if v, ok := repo[k]; ok {
			out[k] = clone(v)
		}
	}
	for _, k := range mergeKeys {
		rv, ok := repo[k].(map[string]any)
		if !ok {
			continue
		}
		dst, _ := out[k].(map[string]any)
		if dst == nil {
			dst = map[string]any{}
		}
		for name, v := range rv {
			dst[name] = clone(v)
		}
		out[k] = dst
	}
	deepMerge(out, overlay)

	var changed []string
	for k, v := range out {
		if old, ok := live[k]; !ok || !reflect.DeepEqual(old, v) {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	return out, changed
}

func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		sm, srcIsMap := v.(map[string]any)
		dm, dstIsMap := dst[k].(map[string]any)
		if srcIsMap && dstIsMap {
			deepMerge(dm, sm)
			continue
		}
		dst[k] = clone(v)
	}
}

func clone(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return cloneMap(t)
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = clone(t[i])
		}
		return out
	}
	return v
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = clone(v)
	}
	return out
}

func readJSONFile(path string) (map[string]any, []byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	m, err := decodeJSON(b)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, b, nil
}

func decodeJSON(b []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// encode writes 2-space-indented JSON with a trailing newline and without
// escaping & < > — hook commands are full of them.
func encode(m map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RepoSettings reads the repo's settings.json as it must look on opt.GOOS:
// hooks and status line rewritten for Windows, untouched elsewhere.
func RepoSettings(opt Options) (map[string]any, error) {
	repo, _, err := readJSONFile(filepath.Join(opt.Repo, "config", "claude", "settings.json"))
	if err != nil {
		return nil, fmt.Errorf("sync: repo settings: %w", err)
	}
	goos := opt.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	return ForOS(repo, goos, opt.AvalBin, opt.Home), nil
}

// planSettings computes the merged settings without writing anything.
func planSettings(opt Options) (merged map[string]any, changed []string, old []byte, exists bool, warnings []string, err error) {
	repo, err := RepoSettings(opt)
	if err != nil {
		return nil, nil, nil, false, nil, err
	}
	live := map[string]any{}
	livePath := filepath.Join(opt.Home, ".claude", "settings.json")
	if m, b, rerr := readJSONFile(livePath); rerr == nil {
		live, old, exists = m, b, true
	} else if !os.IsNotExist(unwrapPathErr(rerr)) {
		return nil, nil, nil, false, nil, fmt.Errorf("sync: live settings: %w", rerr)
	}
	overlay, err := overlaySettings(opt)
	if err != nil {
		return nil, nil, nil, false, nil, err
	}
	merged, changed, warnings = MergeSettingsWarn(repo, live, overlay)
	return merged, changed, old, exists, warnings, nil
}

func unwrapPathErr(err error) error {
	for {
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return err
		}
		err = u.Unwrap()
	}
}

// overlaySettings loads the private settings fragment, if there is one.
func overlaySettings(opt Options) (map[string]any, error) {
	if opt.PrivateDir == "" {
		return nil, nil
	}
	m, _, err := readJSONFile(filepath.Join(opt.PrivateDir, "settings.json"))
	if err != nil {
		if os.IsNotExist(unwrapPathErr(err)) {
			return nil, nil
		}
		return nil, fmt.Errorf("sync: private settings: %w", err)
	}
	return m, nil
}

// SettingsDrift lists the top-level keys sync would change, writing nothing.
func SettingsDrift(opt Options) ([]string, error) {
	_, changed, _, _, _, err := planSettings(opt)
	return changed, err
}

func syncSettings(opt Options, rec *backup.Recorder) (SettingsResult, error) {
	var res SettingsResult
	merged, changed, _, exists, warnings, err := planSettings(opt)
	if err != nil {
		return res, err
	}
	res.Changed = changed
	res.Warnings = warnings
	res.Created = !exists
	if len(changed) == 0 && exists {
		return res, nil
	}
	if opt.DryRun {
		return res, nil
	}

	livePath := filepath.Join(opt.Home, ".claude", "settings.json")
	data, err := encode(merged)
	if err != nil {
		return res, err
	}
	// The saved copy keeps the name older backups used for this file.
	saved, err := rec.SnapshotAs(livePath, "claude-settings.json")
	if err != nil {
		return res, fmt.Errorf("sync: backup: %w", err)
	}
	if exists {
		res.Backup = saved
	}
	if err := rec.MkdirAll(filepath.Dir(livePath), 0o755); err != nil {
		return res, err
	}
	if err := backup.WriteFileAtomic(livePath, data, 0o600); err != nil {
		return res, fmt.Errorf("sync: write settings: %w", err)
	}
	if err := rec.Wrote(livePath); err != nil {
		return res, err
	}
	res.Written = true
	return res, nil
}

const (
	srcBegin = "<!-- shared:agent-rules -->"
	srcEnd   = "<!-- /shared:agent-rules -->"
	dstBegin = "<!-- aval:agent-rules -->"
	dstEnd   = "<!-- /aval:agent-rules -->"
)

// ApplyRules puts block between the aval markers of content: it replaces an
// existing block or appends one, and never touches anything outside the
// markers. The bool reports whether content changed.
func ApplyRules(block, content string) (string, bool) {
	lines := strings.Split(content, "\n")
	begin, end := -1, -1
	for i, l := range lines {
		switch strings.TrimSpace(l) {
		case dstBegin:
			if begin < 0 {
				begin = i
			}
		case dstEnd:
			if begin >= 0 && end < 0 {
				end = i
			}
		}
	}

	var out string
	if begin >= 0 && end > begin {
		merged := append([]string{}, lines[:begin+1]...)
		merged = append(merged, strings.Split(block, "\n")...)
		merged = append(merged, lines[end:]...)
		out = strings.Join(merged, "\n")
	} else {
		prefix := content
		if prefix != "" && !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
		}
		out = prefix + "\n" + dstBegin + "\n" + block + "\n" + dstEnd + "\n"
	}
	return out, out != content
}

// RemoveRules is the inverse of ApplyRules: it cuts the aval block (and the one
// blank line ApplyRules put before it) and nothing else. The bool reports
// whether content changed.
func RemoveRules(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	begin, end := -1, -1
	for i, l := range lines {
		switch strings.TrimSpace(l) {
		case dstBegin:
			if begin < 0 {
				begin = i
			}
		case dstEnd:
			if begin >= 0 && end < 0 {
				end = i
			}
		}
	}
	if begin < 0 || end < begin {
		return content, false
	}
	cut := begin
	if cut > 0 && lines[cut-1] == "" {
		cut--
	}
	out := append(append([]string{}, lines[:cut]...), lines[end+1:]...)
	return strings.Join(out, "\n"), true
}

func sharedBlock(repo string) (string, error) {
	path := filepath.Join(repo, "config", "claude", "CLAUDE.md")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("sync: %w", err)
	}
	var block []string
	in, found := false, false
	for _, l := range strings.Split(string(b), "\n") {
		switch strings.TrimSpace(l) {
		case srcBegin:
			in, found = true, true
			continue
		case srcEnd:
			in = false
			continue
		}
		if in {
			block = append(block, l)
		}
	}
	if !found || len(block) == 0 {
		return "", fmt.Errorf("sync: no %s block in %s", srcBegin, path)
	}
	return strings.Join(block, "\n"), nil
}

func syncRules(opt Options, rec *backup.Recorder) ([]RulesResult, error) {
	block, err := sharedBlock(opt.Repo)
	if err != nil {
		return nil, err
	}
	targets := []string{
		filepath.Join(opt.Home, ".codex", "AGENTS.md"),
		filepath.Join(opt.Home, ".gemini", "GEMINI.md"),
	}
	var out []RulesResult
	for _, path := range targets {
		res := RulesResult{Path: path}
		if fi, err := os.Stat(filepath.Dir(path)); err != nil || !fi.IsDir() {
			res.Skipped = true
			out = append(out, res)
			continue
		}
		var content string
		if b, err := os.ReadFile(path); err == nil {
			content = string(b)
		} else if !os.IsNotExist(err) {
			return out, fmt.Errorf("sync: %w", err)
		}
		next, changed := ApplyRules(block, content)
		res.Changed = changed
		if changed && !opt.DryRun {
			if _, err := rec.Snapshot(path); err != nil {
				return out, fmt.Errorf("sync: backup: %w", err)
			}
			if err := backup.WriteFileAtomic(path, []byte(next), 0o644); err != nil {
				return out, fmt.Errorf("sync: %w", err)
			}
			if err := rec.Wrote(path); err != nil {
				return out, fmt.Errorf("sync: %w", err)
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// Run applies every sync step.
func Run(opt Options) (*Report, error) {
	rec := opt.Recorder
	if rec == nil && !opt.DryRun {
		st := backup.NewStore(opt.Home)
		st.Now = opt.Now
		rec = st.Begin()
		rec.SetOrigin("sync")
		defer func() { _, _, _ = rec.Commit() }()
	}
	if rec == nil { // dry run: nothing is written, nothing is recorded
		rec = backup.NewStore(opt.Home).Begin()
	}
	opt.Recorder = rec

	rep := &Report{}
	var err error
	if rep.Settings, err = syncSettings(opt, rec); err != nil {
		return rep, err
	}
	if rep.Rules, err = syncRules(opt, rec); err != nil {
		return rep, err
	}
	if rep.Links, err = syncLinks(opt); err != nil {
		return rep, err
	}
	return rep, nil
}

// LinkResult describes one private-overlay link.
type LinkResult struct {
	Source string
	Dest   string
	Status string
	Backup string
}

// PrivateLinks reads the overlay's links.manifest and resolves both sides to
// absolute paths. A missing manifest is not an error: the overlay is optional.
func PrivateLinks(privateDir, home string) ([]plan.Action, error) {
	if privateDir == "" {
		return nil, nil
	}
	f, err := os.Open(filepath.Join(privateDir, "links.manifest"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("sync: %w", err)
	}
	defer f.Close()
	entries, err := manifest.ParseLinks(f)
	if err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}
	out := make([]plan.Action, 0, len(entries))
	for _, e := range entries {
		out = append(out, plan.Action{
			Source: filepath.Join(privateDir, filepath.FromSlash(e.Source)),
			Dest:   filepath.Join(home, filepath.FromSlash(e.Dest)),
			Label:  "~/" + e.Dest,
		})
	}
	return out, nil
}

func syncLinks(opt Options) ([]LinkResult, error) {
	actions, err := PrivateLinks(opt.PrivateDir, opt.Home)
	if err != nil || len(actions) == 0 {
		return nil, err
	}
	runner := install.New(install.Options{
		Home:     opt.Home,
		Symlinks: true,
		DryRun:   opt.DryRun,
		Recorder: opt.Recorder,
	})
	var out []LinkResult
	for _, a := range actions {
		res, err := runner.Apply(a)
		if err != nil {
			return out, fmt.Errorf("sync: %w", err)
		}
		status := "linked"
		switch res.Status {
		case install.StatusAlready:
			status = "already linked"
		case install.StatusWould:
			status = "would link"
		}
		out = append(out, LinkResult{Source: a.Source, Dest: a.Dest, Status: status, Backup: res.Backup})
	}
	return out, nil
}

// PrivateDir is where the private overlay lives: $AVAL_PRIVATE_DIR, else
// ~/.config/aval/private. It is deliberately outside the repo.
func PrivateDir(home string) string {
	if d := os.Getenv("AVAL_PRIVATE_DIR"); d != "" {
		return d
	}
	return filepath.Join(home, ".config", "aval", "private")
}

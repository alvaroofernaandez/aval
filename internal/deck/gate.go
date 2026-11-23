package deck

// The plan gate is aval's "understand before you run" rule, enforced: while the
// project's active plan (odd/tasks/*.md) has no deck for its current sections,
// edits outside odd/ are denied. A deck is recorded by
// `aval deck build ... --plan <plan>`.
//
// The active plan is the newest odd/tasks/*.md that still has an open item
// ("- [ ]") and changed in the last 72 hours, so finished or abandoned plans
// never block. A plan's identity is its headings: ticking boxes is progress and
// keeps the deck valid; adding or renaming a section asks for a new deck.
//
// Like the design gate, it fails OPEN on anything it cannot determine: a gate
// that hard-fails is a gate people delete.

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// activeWindow is how recently a plan must have changed to count as active.
const activeWindow = 72 * time.Hour

// GateOptions configures one gate run.
type GateOptions struct {
	Off   bool      // AVAL_PLAN_DECK_OFF: bypass deliberately
	Cache string    // cache base dir (XDG_CACHE_HOME or ~/.cache)
	Now   time.Time // zero means time.Now()
}

type gateInput struct {
	Cwd       string `json:"cwd"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
}

// Gate reads a PreToolUse payload on in and writes a deny decision to out, or
// nothing to allow.
func Gate(in io.Reader, out io.Writer, o GateOptions) {
	if o.Off {
		return
	}
	b, err := io.ReadAll(in)
	if err != nil || len(b) == 0 {
		return
	}
	var h gateInput
	if json.Unmarshal(b, &h) != nil {
		return
	}
	switch h.ToolName {
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
	default:
		return
	}
	file := h.ToolInput.FilePath
	if file == "" {
		file = h.ToolInput.NotebookPath
	}
	if file == "" {
		return
	}
	if !filepath.IsAbs(file) {
		if h.Cwd == "" {
			return
		}
		file = filepath.Join(h.Cwd, file)
	}
	root, ok := gitRoot(filepath.Dir(file))
	if !ok {
		return
	}
	rel, err := filepath.Rel(root, physical(file))
	if err != nil || strings.HasPrefix(rel, "..") {
		return
	}
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, "odd/") || strings.HasPrefix(rel, ".git/") || strings.HasSuffix(rel, ".deck.json") {
		return
	}
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	planRel, content, ok := activePlan(root, now)
	if !ok {
		return
	}
	acked, known := readAcks(markerPath(o.Cache, root))[planRel]
	why := "has no deck yet"
	if known {
		if acked == headingsHash(content) {
			return
		}
		why = "changed since its deck (new or renamed sections)"
	}
	reason := fmt.Sprintf("Blocked: the plan %s %s. aval shows a plan before running it: use the plan-deck skill to "+
		"write the deck spec, then `aval deck build <spec.json> -o <out.pdf> --plan %s`, and retry. "+
		"Bypass deliberately with AVAL_PLAN_DECK_OFF=1.", planRel, why, planRel)
	type specific struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(struct {
		HookSpecificOutput specific `json:"hookSpecificOutput"`
	}{specific{"PreToolUse", "deny", reason}})
}

// AckPlan records that a deck exists for the plan at planPath, as it is now.
func AckPlan(cache, planPath string, now time.Time) error {
	abs, err := filepath.Abs(planPath)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("reading the plan: %w", err)
	}
	root, ok := gitRoot(filepath.Dir(abs))
	if !ok {
		return errors.New("the plan is not inside a git repository")
	}
	rel, err := filepath.Rel(root, physical(abs))
	if err != nil {
		return err
	}
	path := markerPath(cache, root)
	acks := readAcks(path)
	acks[filepath.ToSlash(rel)] = headingsHash(content)
	keys := make([]string, 0, len(acks))
	for k := range acks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	fmt.Fprintf(&b, "# aval plan-deck acks for %s, last %s\n", root, now.UTC().Format(time.RFC3339))
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, acks[k])
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// PlanCacheDir is XDG_CACHE_HOME, else ~/.cache (same base as the design gate).
func PlanCacheDir() (string, error) { return PlanCacheDirFrom(os.Getenv, os.UserHomeDir) }

// PlanCacheDirFrom is PlanCacheDir over an injected environment and home lookup.
func PlanCacheDirFrom(getenv func(string) string, home func() (string, error)) (string, error) {
	if x := getenv("XDG_CACHE_HOME"); x != "" {
		return x, nil
	}
	h, err := home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".cache"), nil
}

func markerPath(cache, root string) string {
	sum := sha1.Sum([]byte(root))
	return filepath.Join(cache, "plan-deck", hex.EncodeToString(sum[:]))
}

func readAcks(path string) map[string]string {
	acks := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return acks
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && k != "" {
			acks[k] = v
		}
	}
	return acks
}

// activePlan is the newest odd/tasks/*.md with an open item, changed within the
// active window.
func activePlan(root string, now time.Time) (rel string, content []byte, ok bool) {
	matches, err := filepath.Glob(filepath.Join(root, "odd", "tasks", "*.md"))
	if err != nil {
		return "", nil, false
	}
	var best string
	var bestTime time.Time
	for _, m := range matches {
		st, err := os.Stat(m)
		if err != nil || !st.Mode().IsRegular() || now.Sub(st.ModTime()) > activeWindow {
			continue
		}
		if st.ModTime().After(bestTime) {
			b, err := os.ReadFile(m)
			if err != nil || !bytes.Contains(b, []byte("- [ ]")) {
				continue
			}
			best, bestTime, content = m, st.ModTime(), b
		}
	}
	if best == "" {
		return "", nil, false
	}
	r, err := filepath.Rel(root, best)
	if err != nil {
		return "", nil, false
	}
	return filepath.ToSlash(r), content, true
}

// headingsHash identifies a plan by its Markdown headings only.
func headingsHash(content []byte) string {
	h := sha1.New()
	sc := bufio.NewScanner(bytes.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") {
			h.Write([]byte(line + "\n"))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func physical(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	// The file may not exist yet (Write): resolve its directory instead.
	if d, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		return filepath.Join(d, filepath.Base(p))
	}
	return p
}

// gitRoot is the nearest ancestor holding .git (a directory or worktree file),
// as a physical path. Missing directories (a Write into a new folder) are
// walked through.
func gitRoot(dir string) (string, bool) {
	for d := dir; ; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return physical(d), true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}

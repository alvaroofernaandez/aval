// Package designhook holds the two Claude Code hooks of the design pipeline:
// the PreToolUse gate that denies UI edits until `aval design ack` has run, and
// the Stop hook that asks for a render after a UI turn.
//
// Both fail OPEN on anything they cannot determine: a gate that hard-fails is a
// gate people delete. The gate costs one JSON parse for a non-UI path and one
// sha1 for a UI path.
package designhook

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alvaroofernaandez/aval/internal/designack"
)

// GateOptions configures one gate run.
type GateOptions struct {
	Off   bool      // DESIGN_PIPELINE_OFF: bypass deliberately
	Cache string    // cache base dir (XDG_CACHE_HOME or ~/.cache)
	TTL   int       // seconds an ack stays valid
	Now   time.Time // zero means time.Now()
}

type hookInput struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
	StopHookActive bool `json:"stop_hook_active"`
}

func parseInput(in io.Reader) (hookInput, bool) {
	var h hookInput
	b, err := io.ReadAll(in)
	if err != nil || len(b) == 0 || json.Unmarshal(b, &h) != nil {
		return h, false
	}
	return h, true
}

// validSID mirrors what the stamp file name may contain; anything else is not
// used in a path.
func validSID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

var uiExts = []string{
	".tsx", ".jsx", ".vue", ".svelte", ".astro",
	".css", ".scss", ".sass", ".less", ".styl",
	".html", ".htm",
}

// isUISurface: tests assert behaviour and are not design surfaces, since
// blocking them would deadlock against strict TDD, which demands the test first.
func isUISurface(file string) bool {
	base := file[strings.LastIndex(file, "/")+1:]
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return false
	}
	if strings.Contains(file, "/__tests__/") || strings.Contains(file, "/node_modules/") {
		return false
	}
	for _, e := range uiExts {
		if strings.HasSuffix(base, e) {
			return true
		}
	}
	return false
}

// projectRoot walks up from the file looking for .git (dir or worktree file),
// falling back to cwd, then the file's directory; the result is a physical path.
func projectRoot(file, cwd string) string {
	root := ""
	for d := filepath.Dir(file); d != "" && d != "."; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			root = d
			break
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	if root == "" {
		root = cwd
		if root == "" {
			root = filepath.Dir(file)
		}
	}
	if phys, err := filepath.EvalSymlinks(root); err == nil {
		root = phys
	}
	return root
}

// Gate reads a PreToolUse payload on in and writes a deny decision to out, or
// nothing to allow.
func Gate(in io.Reader, out io.Writer, o GateOptions) {
	if o.Off {
		return
	}
	h, ok := parseInput(in)
	if !ok {
		return
	}
	switch h.ToolName {
	case "Edit", "Write", "NotebookEdit":
	default:
		return
	}
	file := h.ToolInput.FilePath
	if file == "" || !isUISurface(file) {
		return
	}
	cwd := h.Cwd
	if !filepath.IsAbs(file) {
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		file = cwd + "/" + file
	}
	root := projectRoot(file, h.Cwd)

	dhash := ""
	if st, err := os.Stat(filepath.Join(root, ".agents", "DESIGN.md")); err == nil && st.Mode().IsRegular() {
		b, err := os.ReadFile(filepath.Join(root, ".agents", "DESIGN.md"))
		if err != nil {
			return
		}
		dhash = designack.Sha1Hex(b)
	}

	why := "no design-ack for this project yet"
	if m, ok := designack.ReadMarker(designack.MarkerPath(o.Cache, root)); ok {
		epoch, err := strconv.ParseInt(m.Epoch, 10, 64)
		if err != nil || !allDigits(m.Epoch) {
			why = "the design-ack marker is unreadable"
		} else {
			now := o.Now
			if now.IsZero() {
				now = time.Now()
			}
			switch {
			case now.Unix()-epoch > int64(o.TTL):
				why = "the last design-ack is older than " + strconv.Itoa(o.TTL/60) + " min"
			case m.DesignSHA1 != dhash:
				why = ".agents/DESIGN.md changed since the last design-ack"
			default:
				if validSID(h.SessionID) { // lets the Stop hook know a UI file changed
					_ = os.WriteFile(StampPath(o.Cache, h.SessionID), []byte(root), 0o644)
				}
				return
			}
		}
	}

	name := strings.NewReplacer(`"`, "_", `\`, "_").Replace(filepath.Base(file))
	reason := "Blocked: " + name + " is a UI surface and " + why +
		". Do the design pipeline (invoke the design-pipeline skill if you have not: laws of UX, direction from .agents/DESIGN.md, gates), then run `aval design ack <surface>` and retry."
	type specific struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	}
	writeJSON(out, struct {
		HookSpecificOutput specific `json:"hookSpecificOutput"`
	}{specific{"PreToolUse", "deny", reason}})
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// StampPath is where the gate records "a UI file changed in this session".
func StampPath(cache, sid string) string {
	return filepath.Join(cache, "design-gate", "ui-edited."+sid)
}

func writeJSON(out io.Writer, v any) {
	enc := json.NewEncoder(out) // one line plus "\n", like the hook it replaces
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

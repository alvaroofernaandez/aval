package hook

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// StopOptions configures one Stop-hook run.
type StopOptions struct {
	Off   bool   // DESIGN_PIPELINE_OFF
	Cache string // cache base dir (XDG_CACHE_HOME or ~/.cache)
}

// stopMessage is verbatim what the shell hook printed.
const stopMessage = `{"decision":"block","reason":"UI files were edited this session but .design/shots has no render newer than the last edit. Run ` +
	"`node ~/.claude/skills/design-pipeline/scripts/shot.mjs <url|file.html>`" +
	` and quote its paths and numbers, or say \"not visually verified\" in your reply."}` + "\n"

// ShotStop is the Stop hook: a turn that edited a UI file must end with a render
// or an honest "not visually verified".
//
// The gate stamps <cache>/design-gate/ui-edited.<sid> (content: project root)
// whenever it allows a UI edit. If that stamp exists and the project's
// .design/shots holds no file newer than it, the stop is blocked ONCE per
// stamp: that is recorded in <stamp>.blocked, and a block is only issued while
// the record is missing or older than the stamp, so a later UI edit can block
// again but a stubborn reply never loops. The common case (no UI edit) is one
// failed stat.
func ShotStop(in io.Reader, out io.Writer, o StopOptions) {
	if o.Off {
		return
	}
	h, ok := parseInput(in)
	if !ok || !validSID(h.SessionID) || h.StopHookActive {
		return
	}
	stampPath := StampPath(o.Cache, h.SessionID)
	stamp, err := os.Stat(stampPath)
	if err != nil || !stamp.Mode().IsRegular() {
		return
	}
	blockedPath := stampPath + ".blocked"
	if b, err := os.Stat(blockedPath); err == nil && !stamp.ModTime().After(b.ModTime()) {
		return
	}
	raw, _ := os.ReadFile(stampPath)
	root, _, _ := strings.Cut(string(raw), "\n")
	if root == "" {
		return
	}
	entries, _ := os.ReadDir(filepath.Join(root, ".design", "shots"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if st, err := os.Stat(filepath.Join(root, ".design", "shots", e.Name())); err == nil &&
			st.Mode().IsRegular() && st.ModTime().After(stamp.ModTime()) {
			return
		}
	}
	// The marker carries the stamp's own mtime, not the clock's: a file's mtime
	// can read earlier than the stamp's, and then the block would repeat.
	if os.WriteFile(blockedPath, nil, 0o644) == nil {
		_ = os.Chtimes(blockedPath, stamp.ModTime(), stamp.ModTime())
	}
	_, _ = io.WriteString(out, stopMessage)
}

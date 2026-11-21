package lessons

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) Options {
	t.Helper()
	dir, state := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dir, "ui.md"), "---\ntriggers:\n  - \"**/*.tsx\"\n  - src/**/*.css\n---\nRender it before saying fixed.\n")
	write(t, filepath.Join(dir, "docker.md"), "---\ntriggers:\n  - '**/Dockerfile*'\ncommands:\n  - '(^|[;&|] *)docker( |$)'\n---\nUse Kiri.\n")
	write(t, filepath.Join(dir, "commit.md"), "---\ncommands:\n  - 'git commit'\n---\nNo attribution trailers.\n")
	write(t, filepath.Join(dir, "README.md"), "not a lesson, no frontmatter\n")
	return Options{Dir: dir, StateDir: state}
}

func run(t *testing.T, opt Options, payload string) string {
	t.Helper()
	var out bytes.Buffer
	if err := Run(strings.NewReader(payload), &out, opt); err != nil {
		t.Fatalf("Run returned %v; the hook must never fail", err)
	}
	return out.String()
}

func ctxOf(t *testing.T, out string) string {
	t.Helper()
	var r struct {
		H struct {
			Event string `json:"hookEventName"`
			Ctx   string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("output is not hook JSON: %v\n%q", err, out)
	}
	if r.H.Event != "PreToolUse" {
		t.Errorf("hookEventName = %q, want PreToolUse", r.H.Event)
	}
	return r.H.Ctx
}

func TestPathMatchEmitsAdditionalContext(t *testing.T) {
	opt := fixture(t)
	out := run(t, opt, `{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"/p/web/src/a/Button.tsx"}}`)
	c := ctxOf(t, out)
	if !strings.Contains(c, "Render it before saying fixed.") || !strings.Contains(c, "ui") {
		t.Errorf("context = %q, want the ui lesson with its name", c)
	}
}

func TestNoMatchIsSilent(t *testing.T) {
	opt := fixture(t)
	for _, p := range []string{
		`{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"/p/main.go"}}`,
		`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"ls -la"}}`,
		`not json at all`,
		``,
	} {
		if out := run(t, opt, p); out != "" {
			t.Errorf("payload %q printed %q, want nothing", p, out)
		}
	}
}

func TestGlobsRelativeToAnyDirectory(t *testing.T) {
	opt := fixture(t)
	if out := run(t, opt, `{"session_id":"g","tool_name":"Write","tool_input":{"file_path":"/p/src/deep/x/y.css"}}`); out == "" {
		t.Error("src/**/*.css should match /p/src/deep/x/y.css")
	}
	if out := run(t, opt, `{"session_id":"g2","tool_name":"Write","tool_input":{"file_path":"/p/lib/y.css"}}`); out != "" {
		t.Errorf("src/**/*.css must not match /p/lib/y.css, got %q", out)
	}
	if out := run(t, opt, `{"session_id":"g3","tool_name":"Write","tool_input":{"file_path":"/p/svc/Dockerfile.prod"}}`); out == "" {
		t.Error("**/Dockerfile* should match Dockerfile.prod")
	}
}

func TestBashCommandMatch(t *testing.T) {
	opt := fixture(t)
	c := ctxOf(t, run(t, opt, `{"session_id":"b","tool_name":"Bash","tool_input":{"command":"cd x && docker build ."}}`))
	if !strings.Contains(c, "Use Kiri.") {
		t.Errorf("context = %q", c)
	}
	// A command regex never fires on a file edit.
	if out := run(t, opt, `{"session_id":"b2","tool_name":"Edit","tool_input":{"file_path":"/p/notes-git commit.txt"}}`); out != "" {
		t.Errorf("command lesson fired on an Edit: %q", out)
	}
}

func TestDedupePerLessonPerSession(t *testing.T) {
	opt := fixture(t)
	p := `{"session_id":"sess","tool_name":"Bash","tool_input":{"command":"git commit -m x"}}`
	if run(t, opt, p) == "" {
		t.Fatal("first match must emit")
	}
	if out := run(t, opt, p); out != "" {
		t.Errorf("second match in the same session emitted %q", out)
	}
	other := `{"session_id":"sess2","tool_name":"Bash","tool_input":{"command":"git commit -m x"}}`
	if run(t, opt, other) == "" {
		t.Error("a new session must see the lesson again")
	}
	if run(t, opt, `{"session_id":"sess","tool_name":"Bash","tool_input":{"command":"docker ps"}}`) == "" {
		t.Error("another lesson must not be suppressed by the first")
	}
	if _, err := os.Stat(filepath.Join(opt.StateDir, "sess.seen")); err != nil {
		t.Errorf("dedupe file missing: %v", err)
	}
}

func TestMultipleLessonsAreCombined(t *testing.T) {
	opt := fixture(t)
	write(t, filepath.Join(opt.Dir, "extra.md"), "---\ntriggers:\n  - '**/*.tsx'\n---\nSecond.\n")
	c := ctxOf(t, run(t, opt, `{"session_id":"m","tool_name":"Edit","tool_input":{"file_path":"/p/a.tsx"}}`))
	if !strings.Contains(c, "Second.") || !strings.Contains(c, "Render it") {
		t.Errorf("context = %q, want both lessons", c)
	}
}

func TestMissingDirAndBadSessionNeverFail(t *testing.T) {
	opt := Options{Dir: filepath.Join(t.TempDir(), "nope"), StateDir: t.TempDir()}
	if out := run(t, opt, `{"session_id":"x","tool_name":"Edit","tool_input":{"file_path":"/a.tsx"}}`); out != "" {
		t.Errorf("missing lessons dir printed %q", out)
	}
	opt = fixture(t)
	run(t, opt, `{"session_id":"../../evil","tool_name":"Bash","tool_input":{"command":"git commit"}}`)
	if _, err := os.Stat(filepath.Join(opt.StateDir, "..", "..", "evil.seen")); err == nil {
		t.Error("session id escaped the state dir")
	}
}

func TestNotebookPathAndNoSession(t *testing.T) {
	opt := fixture(t)
	p := `{"tool_name":"NotebookEdit","tool_input":{"notebook_path":"/p/a.tsx"}}`
	if run(t, opt, p) == "" || run(t, opt, p) == "" {
		t.Error("without a session id there is nothing to dedupe on: emit every time")
	}
}

func TestBodyIsCapped(t *testing.T) {
	opt := fixture(t)
	write(t, filepath.Join(opt.Dir, "big.md"), "---\ncommands:\n  - 'bigcmd'\n---\n"+strings.Repeat("x", 5000))
	c := ctxOf(t, run(t, opt, `{"session_id":"c","tool_name":"Bash","tool_input":{"command":"bigcmd"}}`))
	if len(c) > 700 {
		t.Errorf("context is %d chars; a lesson body must be cut at 600", len(c))
	}
}

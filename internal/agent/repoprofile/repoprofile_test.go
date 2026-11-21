package repoprofile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func w(t *testing.T, p, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func context(t *testing.T, dir string) string {
	t.Helper()
	var out strings.Builder
	Run(strings.NewReader(`{"hook_event_name":"SessionStart","cwd":"`+filepath.ToSlash(dir)+`"}`), &out)
	if out.Len() == 0 {
		return ""
	}
	var v struct {
		H struct {
			Event string `json:"hookEventName"`
			Ctx   string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out.String()), &v); err != nil {
		t.Fatalf("not hook JSON: %v: %s", err, out.String())
	}
	if v.H.Event != "SessionStart" {
		t.Errorf("event = %q", v.H.Event)
	}
	return v.H.Ctx
}

func TestNodeProject(t *testing.T) {
	d := t.TempDir()
	w(t, filepath.Join(d, "package.json"), `{"scripts":{"test":"vitest","dev":"next dev"},"dependencies":{"next":"15","react":"19"}}`)
	w(t, filepath.Join(d, "pnpm-lock.yaml"), "")
	w(t, filepath.Join(d, "compose.yaml"), "services: {}")
	got := context(t, d)
	for _, want := range []string{"pnpm", "pnpm test", "next", "react", "up"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestGoProjectAndOpenTasks(t *testing.T) {
	d := t.TempDir()
	w(t, filepath.Join(d, "go.mod"), "module x\n")
	w(t, filepath.Join(d, "odd", "tasks", "a.md"), "- [x] done\n- [ ] open one\n")
	got := context(t, d)
	if !strings.Contains(got, "go test ./...") || !strings.Contains(got, "odd/tasks/a.md") {
		t.Errorf("got %q", got)
	}
}

func TestShortAndSilentWhenNothingKnown(t *testing.T) {
	if got := context(t, t.TempDir()); got != "" {
		t.Errorf("unknown dir should print nothing, got %q", got)
	}
	d := t.TempDir()
	w(t, filepath.Join(d, "package.json"), `{"scripts":{"test":"jest"}}`)
	if got := context(t, d); strings.Count(got, "\n") > 5 {
		t.Errorf("profile must stay within 6 lines: %q", got)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeMachine is a repo and a HOME that never touch the real ones.
func fakeMachine(t *testing.T) (repo, home string) {
	t.Helper()
	repo, home = t.TempDir(), t.TempDir()
	t.Setenv("AVAL_PRIVATE_DIR", filepath.Join(home, "private"))
	writeFile(t, filepath.Join(repo, "install.manifest"),
		"[claude]\nlabel = c\ndetail = d\nplatforms = all\n\nconfig/claude/agents .claude/agents\n")
	writeFile(t, filepath.Join(repo, "config/claude/agents/a.md"), "---\nmodel: opus\n---\n")
	writeFile(t, filepath.Join(repo, "config/claude/settings.json"), `{"spinnerTipsEnabled":false}`)
	writeFile(t, filepath.Join(repo, "config/claude/CLAUDE.md"), "<!-- shared:agent-rules -->\nr\n<!-- /shared:agent-rules -->\n")
	return repo, home
}

func TestRunDoctorJSONAndExitCode(t *testing.T) {
	repo, home := fakeMachine(t)
	var out bytes.Buffer
	err := runDoctor(repo, home, []string{"--json"}, &out)
	if !errors.Is(err, errChecksFailed) {
		t.Errorf("err = %v, want errChecksFailed (agents link is missing)", err)
	}
	var rs []map[string]any
	if jerr := json.Unmarshal(out.Bytes(), &rs); jerr != nil || len(rs) == 0 {
		t.Fatalf("output is not a JSON array: %v\n%s", jerr, out.String())
	}
}

func TestRunDoctorTable(t *testing.T) {
	repo, home := fakeMachine(t)
	var out bytes.Buffer
	_ = runDoctor(repo, home, nil, &out)
	if !strings.Contains(out.String(), "STATUS") || !strings.Contains(out.String(), "fail") && !strings.Contains(out.String(), "FAIL") {
		t.Errorf("unexpected table:\n%s", out.String())
	}
}

func TestRunDoctorRejectsUnknownFlags(t *testing.T) {
	repo, home := fakeMachine(t)
	err := runDoctor(repo, home, []string{"--bogus"}, &bytes.Buffer{})
	if err == nil || errors.Is(err, errChecksFailed) {
		t.Errorf("err = %v, want a usage error", err)
	}
}

func TestRunDoctorFix(t *testing.T) {
	repo, home := fakeMachine(t)
	dead := filepath.Join(home, ".claude/skills/dead")
	if err := os.MkdirAll(filepath.Dir(dead), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "gone"), dead); err != nil {
		t.Skip("symlinks unavailable")
	}
	var out bytes.Buffer
	_ = runDoctor(repo, home, []string{"--fix"}, &out)
	if _, err := os.Lstat(dead); err == nil {
		t.Error("--fix left the dangling link")
	}
	if !strings.Contains(out.String(), "dead") {
		t.Errorf("--fix should say what it removed:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".claude/settings.json")); err != nil {
		t.Errorf("--fix should have run sync: %v", err)
	}
}

func TestRunSyncDryRunThenReal(t *testing.T) {
	repo, home := fakeMachine(t)
	live := filepath.Join(home, ".claude/settings.json")

	var out bytes.Buffer
	if err := runSync(repo, home, []string{"--dry-run"}, false, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(live); err == nil {
		t.Error("dry run wrote the live file")
	}
	if !strings.Contains(out.String(), "spinnerTipsEnabled") {
		t.Errorf("dry run should list the keys:\n%s", out.String())
	}

	out.Reset()
	if err := runSync(repo, home, nil, false, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Errorf("live file not written: %v", err)
	}

	out.Reset()
	if err := runSync(repo, home, nil, false, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Errorf("second run should report nothing to do:\n%s", out.String())
	}
}

func TestRunSyncHonoursTheGlobalDryRunFlag(t *testing.T) {
	repo, home := fakeMachine(t)
	if err := runSync(repo, home, nil, true, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude/settings.json")); err == nil {
		t.Error("global --dry-run was ignored")
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// seedBackup records one run in which ~/f went from "before" to "after".
func seedBackup(t *testing.T, home string) string {
	t.Helper()
	p := filepath.Join(home, "f")
	writeFile(t, p, "before")
	st := backup.NewStore(home)
	rec := st.Begin()
	if _, err := rec.Snapshot(p); err != nil {
		t.Fatal(err)
	}
	writeFile(t, p, "after")
	if err := rec.Wrote(p); err != nil {
		t.Fatal(err)
	}
	run, _, err := rec.Commit()
	if err != nil {
		t.Fatal(err)
	}
	return run.ID
}

func TestRestoreListShowsIDDateEntriesAndPinned(t *testing.T) {
	repo, home := fakeMachine(t)
	id := seedBackup(t, home)
	var out bytes.Buffer
	if err := runRestore(repo, home, []string{"--list"}, false, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ID", "DATE", "ENTRIES", "PINNED", id} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list is missing %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if err := runRestore(repo, home, []string{"--pin", id}, false, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	_ = runRestore(repo, home, []string{"--list"}, false, &out)
	if !strings.Contains(out.String(), "yes") {
		t.Errorf("pinned run not shown as pinned:\n%s", out.String())
	}
}

func TestRestoreLatestPutsTheFileBack(t *testing.T) {
	repo, home := fakeMachine(t)
	seedBackup(t, home)
	var out bytes.Buffer
	if err := runRestore(repo, home, []string{"latest"}, false, &out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "f")); string(b) != "before" {
		t.Errorf("f = %q, want before\n%s", b, out.String())
	}
}

func TestRestoreUnknownIDFails(t *testing.T) {
	repo, home := fakeMachine(t)
	seedBackup(t, home)
	if err := runRestore(repo, home, []string{"no-such-run"}, false, &bytes.Buffer{}); err == nil {
		t.Error("expected an error for an unknown id")
	}
}

// installedMachine is fakeMachine with its one manifest link in place.
func installedMachine(t *testing.T) (repo, home, dest string) {
	t.Helper()
	testenv.NeedSymlinks(t)
	repo, home = fakeMachine(t)
	dest = filepath.Join(home, ".claude", "agents")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repo, "config/claude/agents"), dest); err != nil {
		t.Fatal(err)
	}
	return repo, home, dest
}

func linked(p string) bool {
	_, err := os.Readlink(p)
	return err == nil
}

func TestUninstallAsksForConfirmation(t *testing.T) {
	repo, home, dest := installedMachine(t)
	var out bytes.Buffer
	if err := runUninstall(repo, home, nil, false, false, strings.NewReader("n\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !linked(dest) {
		t.Errorf("answering n must remove nothing:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "[y/N]") {
		t.Errorf("no confirmation prompt:\n%s", out.String())
	}

	out.Reset()
	if err := runUninstall(repo, home, nil, false, false, strings.NewReader("y\n"), &out); err != nil {
		t.Fatal(err)
	}
	if linked(dest) {
		t.Errorf("answering y must uninstall:\n%s", out.String())
	}
}

func TestUninstallYesSkipsThePrompt(t *testing.T) {
	repo, home, dest := installedMachine(t)
	if err := runUninstall(repo, home, []string{"--yes"}, false, false, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if linked(dest) {
		t.Error("--yes must uninstall without asking")
	}
}

func TestUninstallDryRunNeverPromptsOrWrites(t *testing.T) {
	repo, home, dest := installedMachine(t)
	var out bytes.Buffer
	if err := runUninstall(repo, home, []string{"--dry-run"}, false, false, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if !linked(dest) {
		t.Error("dry run removed the link")
	}
	if strings.Contains(out.String(), "[y/N]") || !strings.Contains(out.String(), "would") {
		t.Errorf("dry run output unexpected:\n%s", out.String())
	}
}

func TestRestoreDryRunWritesNothing(t *testing.T) {
	repo, home := fakeMachine(t)
	seedBackup(t, home)
	if err := runRestore(repo, home, []string{"latest"}, true, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "f")); string(b) != "after" {
		t.Errorf("dry run changed f to %q", b)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func chmodX(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// fakeMachine is a repo and a HOME that never touch the real ones.
func fakeMachine(t *testing.T) (repo, home string) {
	t.Helper()
	repo, home = t.TempDir(), t.TempDir()
	t.Setenv("AVAL_PRIVATE_DIR", filepath.Join(home, "private"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config")) // install records the repo there
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

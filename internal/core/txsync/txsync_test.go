package txsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
)

func put(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func clock() func() time.Time {
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	n := 0
	return func() time.Time { n++; return t0.Add(time.Duration(n) * time.Second) }
}

// env is a repo whose only hook resolves, and an empty HOME.
func env(t *testing.T) Options {
	t.Helper()
	repo, home := t.TempDir(), t.TempDir()
	repo, _ = filepath.EvalSymlinks(repo)
	home, _ = filepath.EvalSymlinks(home)
	hook := filepath.Join(repo, "hooks", "ok.sh")
	put(t, hook, "#!/bin/sh\n", 0o755)
	put(t, filepath.Join(repo, "install.manifest"),
		"[claude]\nlabel = c\ndetail = d\nplatforms = all\n\nconfig/claude/hooks .claude/hooks\n", 0o644)
	put(t, filepath.Join(repo, "config/claude/settings.json"),
		`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"`+filepath.ToSlash(hook)+`"}]}]}}`, 0o644)
	put(t, filepath.Join(repo, "config/claude/CLAUDE.md"),
		"x\n<!-- shared:agent-rules -->\nrule\n<!-- /shared:agent-rules -->\n", 0o644)
	return Options{
		Repo: repo, Home: home, GOOS: "linux", Now: clock(),
		PrivateDir: filepath.Join(home, ".config/aval/private"),
		LookPath:   func(string) (string, error) { return "", os.ErrNotExist },
	}
}

func TestRunSucceedsAndLeavesACleanRun(t *testing.T) {
	opt := env(t)
	live := filepath.Join(opt.Home, ".claude/settings.json")
	put(t, live, `{"model":"sonnet"}`, 0o600)

	out, err := Run(opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.RolledBack || len(out.Failed) != 0 {
		t.Errorf("outcome = %+v, want a clean success", out)
	}
	if !strings.Contains(read(t, live), `"Stop"`) {
		t.Error("settings were not synced")
	}
	runs, _ := backup.NewStore(opt.Home).List()
	if len(runs) != 1 || runs[0].Pending || runs[0].Failed {
		t.Errorf("runs = %+v, want one finished run", runs)
	}
}

func TestFailingCheckRestoresSettingsAndMarksTheRunFailed(t *testing.T) {
	opt := env(t)
	live := filepath.Join(opt.Home, ".claude/settings.json")
	orig := `{"model":"sonnet"}`
	put(t, live, orig, 0o600)
	put(t, filepath.Join(opt.Home, ".codex/AGENTS.md"), "mine\n", 0o644)
	// The overlay adds a hook whose command does not exist.
	put(t, filepath.Join(opt.PrivateDir, "settings.json"),
		`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"`+filepath.ToSlash(filepath.Join(opt.Home, "nope", "missing.sh"))+`"}]}]}}`, 0o644)

	out, err := Run(opt)
	if err == nil {
		t.Fatal("expected an error when a doctor check fails")
	}
	if !strings.Contains(err.Error(), "hooks-resolve") {
		t.Errorf("error %q should name the failing check", err)
	}
	if out == nil || !out.RolledBack || len(out.Failed) == 0 {
		t.Fatalf("outcome = %+v, want rolled back with failures", out)
	}
	if got := read(t, live); got != orig {
		t.Errorf("settings not restored: %q", got)
	}
	if got := read(t, filepath.Join(opt.Home, ".codex/AGENTS.md")); got != "mine\n" {
		t.Errorf("rules file not restored: %q", got)
	}
	runs, _ := backup.NewStore(opt.Home).List()
	if len(runs) != 1 || !runs[0].Failed || runs[0].Pending {
		t.Fatalf("runs = %+v, want one failed, non-pending run", runs)
	}
	if !strings.Contains(runs[0].Reason, "hooks-resolve") {
		t.Errorf("failure reason %q should name the check", runs[0].Reason)
	}
}

func TestStalePendingRunIsRestoredBeforeSyncing(t *testing.T) {
	opt := env(t)
	live := filepath.Join(opt.Home, ".claude/settings.json")
	orig := `{"model":"sonnet"}`
	put(t, live, orig, 0o600)

	// A previous sync died mid-apply: it wrote garbage and never finished.
	store := backup.NewStore(opt.Home)
	store.Now = clock()
	rec := store.Begin()
	if err := rec.SetPending(); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Snapshot(live); err != nil {
		t.Fatal(err)
	}
	put(t, live, "garbage from a crash", 0o600)
	if err := rec.Wrote(live); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Base(rec.Dir())

	out, err := Run(opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out.Recovered) != 1 || out.Recovered[0] != stale {
		t.Errorf("Recovered = %v, want [%s]", out.Recovered, stale)
	}
	got := read(t, live)
	if strings.Contains(got, "garbage") || !strings.Contains(got, `"model": "sonnet"`) {
		t.Errorf("settings after recovery + sync = %q", got)
	}
	pend, _ := store.Pending()
	if len(pend) != 0 {
		t.Errorf("%d pending runs remain", len(pend))
	}
	runs, _ := store.List()
	var failedStale bool
	for _, r := range runs {
		if r.ID == stale && r.Failed {
			failedStale = true
		}
	}
	if !failedStale {
		t.Error("the stale run should be marked failed")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	opt := env(t)
	opt.DryRun = true
	if _, err := Run(opt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(opt.Home, ".aval-backup")); err == nil {
		t.Error("dry run created a backup")
	}
	if _, err := os.Stat(filepath.Join(opt.Home, ".claude/settings.json")); err == nil {
		t.Error("dry run wrote settings")
	}
}

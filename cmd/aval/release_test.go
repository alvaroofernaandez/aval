package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/release"
)

func relGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// releaseRepo is a work repo on main pushed to a bare "origin".
func releaseRepo(t *testing.T) (work, remote string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	work, remote = filepath.Join(root, "work"), filepath.Join(root, "origin.git")
	for _, d := range []string{work, remote} {
		os.MkdirAll(d, 0o755)
	}
	relGit(t, remote, "init", "--bare", "-b", "main")
	relGit(t, work, "init", "-b", "main")
	relGit(t, work, "config", "user.email", "t@example.com")
	relGit(t, work, "config", "user.name", "T")
	relGit(t, work, "remote", "add", "origin", remote)
	return work, remote
}

func commitIn(t *testing.T, work, subject string) {
	t.Helper()
	f := filepath.Join(work, "f.txt")
	prev, _ := os.ReadFile(f)
	os.WriteFile(f, append(prev, []byte(subject+"\n")...), 0o644)
	relGit(t, work, "add", "-A")
	relGit(t, work, "commit", "-m", subject)
}

func fakeGate(t *testing.T, testsErr error) {
	t.Helper()
	old := newReleaseGate
	newReleaseGate = func(g release.Git, wd string) release.Gate {
		return release.Gate{Git: g, Remote: "origin", Branch: "main", RunTests: func() error { return testsErr }}
	}
	t.Cleanup(func() { newReleaseGate = old })
}

func release1(t *testing.T, work, input string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runRelease(work, args, strings.NewReader(input), &out)
	return out.String(), err
}

func TestReleaseDryRunShowsVersionAndNotes(t *testing.T) {
	work, remote := releaseRepo(t)
	commitIn(t, work, "chore: init")
	relGit(t, work, "tag", "-a", "v0.1.0", "-m", "v0.1.0")
	commitIn(t, work, "feat(tui): picker")
	commitIn(t, work, "fix: crash")
	commitIn(t, work, "docs: readme")
	relGit(t, work, "push", "-u", "origin", "main")
	fakeGate(t, nil)

	out, err := release1(t, work, "", "minor", "--dry-run")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{"v0.2.0", "### Features", "picker", "### Fixes", "crash"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "readme") {
		t.Errorf("docs commits must not appear:\n%s", out)
	}
	if relGit(t, work, "tag", "--list") != "v0.1.0" || relGit(t, remote, "tag", "--list") != "" {
		t.Error("--dry-run must not tag or push")
	}
}

func TestReleaseOneShotCommands(t *testing.T) {
	work, remote := releaseRepo(t)
	commitIn(t, work, "feat: a")
	relGit(t, work, "push", "-u", "origin", "main")
	fakeGate(t, nil)

	steps := []struct {
		args []string
		want string
	}{
		{[]string{"rc", "--yes"}, "v0.1.0-rc.1"},
		{[]string{"promote", "--yes"}, "v0.1.0"},
	}
	for _, s := range steps {
		out, err := release1(t, work, "", s.args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", s.args, err, out)
		}
		if !strings.Contains(relGit(t, remote, "tag", "--list"), s.want) {
			t.Errorf("%v: %s not on the remote", s.args, s.want)
		}
	}

	commitIn(t, work, "fix: later")
	relGit(t, work, "push", "origin", "main")
	if _, err := release1(t, work, "", "patch", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(relGit(t, remote, "tag", "--list"), "v0.1.1") {
		t.Error("--dry-run pushed a tag")
	}
	if _, err := release1(t, work, "n\n", "patch"); !errors.Is(err, release.ErrAborted) {
		t.Errorf("err = %v, want ErrAborted", err)
	}
	if _, err := release1(t, work, "y\n", "patch"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(relGit(t, remote, "tag", "--list"), "v0.1.1") {
		t.Error("patch was not pushed after confirming")
	}
}

func TestReleaseFailingTestsBlockTheTag(t *testing.T) {
	work, remote := releaseRepo(t)
	commitIn(t, work, "feat: a")
	relGit(t, work, "push", "-u", "origin", "main")
	fakeGate(t, errors.New("FAIL"))
	if _, err := release1(t, work, "", "rc", "--yes"); err == nil {
		t.Fatal("expected failure")
	}
	if relGit(t, remote, "tag", "--list") != "" || relGit(t, work, "tag", "--list") != "" {
		t.Error("no tag may exist when checks fail")
	}
}

func TestReleaseStatus(t *testing.T) {
	work, _ := releaseRepo(t)
	commitIn(t, work, "feat: a")
	relGit(t, work, "tag", "-a", "v0.1.0", "-m", "x")
	commitIn(t, work, "feat: b")
	relGit(t, work, "tag", "-a", "v0.2.0-rc.1", "-m", "x")
	commitIn(t, work, "fix: c")

	out, err := release1(t, work, "", "status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"v0.1.0", "v0.2.0-rc.1", "1 commit", "v0.2.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q:\n%s", want, out)
		}
	}
}

func TestReleaseUsageErrors(t *testing.T) {
	work, _ := releaseRepo(t)
	commitIn(t, work, "feat: a")
	for _, args := range [][]string{{}, {"bogus"}} {
		if _, err := release1(t, work, "", args...); !isUsage(err) {
			t.Errorf("%v: err = %v, want usage", args, err)
		}
	}
}

func TestCIVerdict(t *testing.T) {
	cases := []struct {
		name, json, want string
		err              bool
	}{
		{"green", `[{"status":"completed","conclusion":"success"}]`, "", false},
		{"red", `[{"status":"completed","conclusion":"failure"}]`, "failure", false},
		{"running", `[{"status":"in_progress","conclusion":""}]`, "still running", false},
		{"none", `[]`, "no CI run", false},
		{"garbage", `<html>`, "", true},
		{"latest run wins", `[{"status":"completed","conclusion":"success"},{"status":"completed","conclusion":"failure"}]`, "", false},
	}
	for _, c := range cases {
		got, err := ciVerdict("abc1234", []byte(c.json))
		if (err != nil) != c.err {
			t.Errorf("%s: err = %v", c.name, err)
			continue
		}
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s: got %q, want contains %q", c.name, got, c.want)
		}
	}
}

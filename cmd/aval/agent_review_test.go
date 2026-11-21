package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func reviewRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, "a.md"), "x\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "init")
	gitIn(t, dir, "checkout", "-q", "-b", "feat")
	writeFile(t, filepath.Join(dir, "config/claude/hooks/g.sh"), "echo hi\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "hook")
	return dir
}

func TestRunReviewAssessText(t *testing.T) {
	var out bytes.Buffer
	if err := runReview(reviewRepo(t), []string{"assess", "--base", "main"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"risk      high", "hooks", "review-risk"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestRunReviewAssessJSON(t *testing.T) {
	var out bytes.Buffer
	if err := runReview(reviewRepo(t), []string{"assess", "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var r struct {
		Tier   string   `json:"tier"`
		Lenses []string `json:"lenses"`
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.Tier != "high" || len(r.Lenses) != 4 {
		t.Errorf("json = %v %+v\n%s", err, r, out.String())
	}
}

func TestRunReviewUsage(t *testing.T) {
	if err := runReview(t.TempDir(), nil, &bytes.Buffer{}); err == nil {
		t.Error("missing subcommand must be an error")
	}
	if err := runReview(t.TempDir(), []string{"nope"}, &bytes.Buffer{}); err == nil {
		t.Error("unknown subcommand must be an error")
	}
	if err := runReview(t.TempDir(), []string{"assess"}, &bytes.Buffer{}); err == nil {
		t.Error("a non-git directory must be an error")
	}
}

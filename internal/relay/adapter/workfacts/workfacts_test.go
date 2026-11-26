package workfacts

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func repo(t *testing.T) string {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/feat/relay-autoclear\n")
	write(t, filepath.Join(root, "odd", "tasks", "relay-autoclear.md"), "# Auto-clear\n- [ ] one\n")
	write(t, filepath.Join(root, "odd", "tasks", "touched.md"), "# Touched\n- [x] done\n- [ ] two\n")
	write(t, filepath.Join(root, "odd", "tasks", "finished.md"), "# Finished\n- [x] done\n")
	return root
}

func TestPlans_TouchedPlansWinOverTheBranch(t *testing.T) {
	root := repo(t)
	files := []string{filepath.Join(root, "odd", "tasks", "touched.md"), filepath.Join(root, "main.go"), filepath.Join(root, "odd", "tasks", "gone.md")}
	got := Plans(root, files, "feat/relay-autoclear")
	want := []planTitle{{"Touched", []string{"two"}}}
	if !reflect.DeepEqual(titles(got), want) {
		t.Fatalf("got %+v", got)
	}
}

func TestPlans_FallsBackToThePlanOfTheBranchSuffix(t *testing.T) {
	root := repo(t)
	sub := filepath.Join(root, "internal")
	got := Plans(sub, []string{filepath.Join(root, "main.go")}, "feat/relay-autoclear")
	if want := []planTitle{{"Auto-clear", []string{"one"}}}; !reflect.DeepEqual(titles(got), want) {
		t.Fatalf("got %+v", got)
	}
	if got := Plans(root, nil, "main"); len(got) != 0 {
		t.Fatalf("no plan matches main: %+v", got)
	}
	if got := Plans(t.TempDir(), nil, "feat/relay-autoclear"); len(got) != 0 {
		t.Fatalf("outside a repository: %+v", got)
	}
}

func TestPlans_FinishedPlanIsOmitted(t *testing.T) {
	root := repo(t)
	got := Plans(root, []string{filepath.Join(root, "odd", "tasks", "finished.md")}, "")
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

type planTitle struct {
	Title string
	Items []string
}

func titles(in []relay.PlanFacts) []planTitle {
	var out []planTitle
	for _, p := range in {
		out = append(out, planTitle{p.Title, p.Items})
	}
	return out
}

func TestTree_StatusAndCommitsSinceTheSessionStart(t *testing.T) {
	var calls [][]string
	run := func(dir string, args ...string) (string, error) {
		calls = append(calls, args)
		if args[0] == "status" {
			return " M a.go\n?? b.txt\n", nil
		}
		return "abc1234 feat: x\ndef5678 fix: y\n", nil
	}
	since := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	got := tree(run, "/w", since)
	if got == nil || !reflect.DeepEqual(got.Dirty, []string{" M a.go", "?? b.txt"}) || !reflect.DeepEqual(got.Commits, []string{"abc1234 feat: x", "def5678 fix: y"}) {
		t.Fatalf("got %+v", got)
	}
	if !strings.Contains(strings.Join(calls[1], " "), "--since=2026-11-26T10:00:00Z") {
		t.Errorf("log call %v lacks the session start", calls[1])
	}
}

func TestTree_FailuresOmitWhatTheyCover(t *testing.T) {
	fail := func(string, ...string) (string, error) { return "", errors.New("no git") }
	if got := tree(fail, "/w", time.Now()); got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
	onlyStatus := func(_ string, args ...string) (string, error) {
		if args[0] == "status" {
			return "?? a\n", nil
		}
		return "", errors.New("boom")
	}
	got := tree(onlyStatus, "/w", time.Now())
	if got == nil || len(got.Dirty) != 1 || len(got.Commits) != 0 {
		t.Fatalf("got %+v", got)
	}
	// without a session start there are no commits to list
	if got := tree(onlyStatus, "/w", time.Time{}); got == nil || len(got.Commits) != 0 {
		t.Fatalf("got %+v", got)
	}
}

package prepush

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const zero = "0000000000000000000000000000000000000000"

// leak is employer material, assembled from fragments so this file does not
// match the scanner it tests.
func leak() string { h := "haga"; return "notes about " + h + "link-projects" }

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commit(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", name)
	git(t, dir, "commit", "-qm", "c "+name)
	return git(t, dir, "rev-parse", "HEAD")
}

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	return dir
}

func run(t *testing.T, dir, stdin string, opt Options) (bool, string) {
	t.Helper()
	opt.Repo = dir
	var errOut bytes.Buffer
	ok := Run(strings.NewReader(stdin), &errOut, opt)
	return ok, errOut.String()
}

func TestScanReportsAtMostThreeHitsPerPattern(t *testing.T) {
	added := strings.Repeat("+"+leak()+"\n", 5) + "+clean\n"
	found := Scan(added, Patterns())
	if len(found) != 1 {
		t.Fatalf("want one finding (one pattern matched), got %d: %v", len(found), found)
	}
	if got := strings.Count(found[0].Hits, "\n") + 1; got != 3 {
		t.Fatalf("want 3 hits, got %d: %q", got, found[0].Hits)
	}
}

func TestScanIsCaseInsensitiveAndIgnoresContextLines(t *testing.T) {
	up := strings.ToUpper(leak())
	if len(Scan("+"+up+"\n", Patterns())) == 0 {
		t.Fatal("upper-case employer material was not detected")
	}
	if len(Scan(" "+leak()+"\n-"+leak()+"\n", Patterns())) != 0 {
		t.Fatal("context and removed lines must not count")
	}
}

func TestPatternsAreNotWrittenOutInSource(t *testing.T) {
	src, err := os.ReadFile("patterns.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range Patterns() {
		if !strings.ContainsAny(p, `\|()`) && strings.Contains(string(src), p) {
			t.Fatalf("patterns.go spells %q out; assemble it from fragments", p)
		}
	}
}

func TestRunCleanAndLeakingPush(t *testing.T) {
	dir := newRepo(t)
	base := commit(t, dir, "a.md", "clean")
	clean := commit(t, dir, "b.md", "also clean")
	if ok, out := run(t, dir, fmt.Sprintf("refs/heads/main %s refs/heads/main %s\n", clean, base), Options{}); !ok {
		t.Fatalf("clean push refused: %s", out)
	}
	bad := commit(t, dir, "c.md", leak())
	ok, out := run(t, dir, fmt.Sprintf("refs/heads/main %s refs/heads/main %s\n", bad, clean), Options{})
	if ok || !strings.Contains(out, "employer material matching") || !strings.Contains(out, "push refused") {
		t.Fatalf("leak not refused (ok=%v): %s", ok, out)
	}
}

func TestRunNewBranchScansEverythingNotOnARemote(t *testing.T) {
	dir := newRepo(t)
	sha := commit(t, dir, "c.md", leak())
	if ok, _ := run(t, dir, fmt.Sprintf("refs/heads/x %s refs/heads/x %s\n", sha, zero), Options{}); ok {
		t.Fatal("a new branch carrying a leak must be refused")
	}
}

func TestRunBranchDeletionIsSkipped(t *testing.T) {
	dir := newRepo(t)
	commit(t, dir, "c.md", leak())
	if ok, _ := run(t, dir, fmt.Sprintf("(delete) %s refs/heads/x abc\n", zero), Options{}); !ok {
		t.Fatal("deleting a branch must not be scanned")
	}
}

func TestRunGitleaks(t *testing.T) {
	dir := newRepo(t)
	sha := commit(t, dir, "c.md", "clean")
	in := fmt.Sprintf("refs/heads/x %s refs/heads/x %s\n", sha, zero)

	var gotOpts string
	found := func(opts string) (bool, bool) { gotOpts = opts; return true, true }
	ok, out := run(t, dir, in, Options{Gitleaks: found})
	if ok || !strings.Contains(out, "gitleaks found a secret") {
		t.Fatalf("a gitleaks finding must refuse the push (ok=%v): %s", ok, out)
	}
	if gotOpts != sha+" --not --remotes" {
		t.Fatalf("log opts = %q", gotOpts)
	}
	if ok, _ := run(t, dir, in, Options{Gitleaks: func(string) (bool, bool) { return false, false }}); !ok {
		t.Fatal("a missing gitleaks must be skipped")
	}
}

func TestRunDriftWarnsOrBlocks(t *testing.T) {
	dir := newRepo(t)
	drift := func() (string, error) { return "drift  settings", fmt.Errorf("exit 1") }
	ok, out := run(t, dir, "", Options{Drift: drift})
	if !ok || !strings.Contains(out, "warning: aval sync --check found drift") || !strings.Contains(out, "drift  settings") {
		t.Fatalf("drift should only warn (ok=%v): %s", ok, out)
	}
	ok, out = run(t, dir, "", Options{Drift: drift, Strict: true})
	if ok || !strings.Contains(out, "AVAL_PREPUSH_STRICT=1") {
		t.Fatalf("strict drift should block (ok=%v): %s", ok, out)
	}
	clean := func() (string, error) { return "", nil }
	if ok, _ := run(t, dir, "", Options{Drift: clean, Strict: true}); !ok {
		t.Fatal("strict with no drift must pass")
	}
}

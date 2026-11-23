package release

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fixture is a work repo on main with a bare repo as origin. Nothing here ever
// touches a real remote.
type fixture struct {
	t      *testing.T
	work   string
	remote string
	g      Git
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	f := &fixture{t: t, work: filepath.Join(root, "work"), remote: filepath.Join(root, "origin.git")}
	for _, d := range []string{f.work, f.remote} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git(t, f.remote, "init", "--bare", "-b", "main")
	git(t, f.work, "init", "-b", "main")
	git(t, f.work, "config", "user.email", "t@example.com")
	git(t, f.work, "config", "user.name", "T")
	git(t, f.work, "remote", "add", "origin", f.remote)
	f.g = NewGit(f.work)
	return f
}

var counter int

func (f *fixture) commit(subject string) string {
	counter++
	p := filepath.Join(f.work, "file.txt")
	if err := os.WriteFile(p, []byte(subject+string(rune('a'+counter%26))+strings.Repeat("x", counter)), 0o644); err != nil {
		f.t.Fatal(err)
	}
	git(f.t, f.work, "add", "-A")
	git(f.t, f.work, "commit", "-m", subject)
	return git(f.t, f.work, "rev-parse", "HEAD")
}

func (f *fixture) tag(name string)    { git(f.t, f.work, "tag", "-a", name, "-m", name) }
func (f *fixture) push()              { git(f.t, f.work, "push", "-u", "origin", "main") }
func (f *fixture) remoteTags() string { return git(f.t, f.remote, "tag", "--list") }

func okTests() error { return nil }

func (f *fixture) opts(kind string, in string) (CutOptions, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return CutOptions{
		Kind: kind, In: strings.NewReader(in), Out: out, Remote: "origin",
		Gate: Gate{Git: f.g, Remote: "origin", Branch: "main", RunTests: okTests},
	}, out
}

func TestInspect(t *testing.T) {
	f := newFixture(t)
	f.commit("chore: init")
	f.tag("v0.1.0")
	f.tag("latest") // a non-semver tag must be ignored
	f.commit("feat: a")
	f.tag("v0.2.0-rc.1")
	f.commit("fix: b")

	st, err := Inspect(f.g)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Tags) != 2 {
		t.Errorf("Tags = %v, want 2 semver tags", st.Tags)
	}
	if len(st.Unreleased) != 2 || st.Unreleased[0].Type != "fix" {
		t.Errorf("Unreleased = %+v, want [fix, feat] since v0.1.0", st.Unreleased)
	}
	if len(st.SinceTag) != 1 || st.SinceTag[0].Subject != "b" {
		t.Errorf("SinceTag = %+v, want only the fix since the rc", st.SinceTag)
	}
	if !st.HasLastTag || st.LastTag.String() != "0.2.0-rc.1" {
		t.Errorf("LastTag = %v (%v)", st.LastTag, st.HasLastTag)
	}
	if len(st.HeadTags) != 0 {
		t.Errorf("HeadTags = %v, want none", st.HeadTags)
	}
}

func TestInspectWithoutTags(t *testing.T) {
	f := newFixture(t)
	f.commit("feat: first")
	st, err := Inspect(f.g)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Unreleased) != 1 || st.HasLastTag {
		t.Errorf("state = %+v", st)
	}
}

func TestResolve(t *testing.T) {
	f := newFixture(t)
	f.commit("chore: init")
	f.tag("v0.1.0")
	rcCommit := f.commit("feat: a")
	f.tag("v0.2.0-rc.1")
	f.commit("fix: b")
	st, _ := Inspect(f.g)

	cases := []struct {
		kind, want string
		err        bool
	}{
		{"rc", "0.2.0-rc.2", false},
		{"beta", "", true}, // beta after rc
		{"patch", "0.1.1", false},
		{"minor", "0.2.0", false},
		{"major", "1.0.0", false},
		{"promote", "0.2.0", false},
		{"bogus", "", true},
	}
	for _, c := range cases {
		p, err := Resolve(f.g, st, c.kind)
		if (err != nil) != c.err {
			t.Errorf("%s: err = %v", c.kind, err)
			continue
		}
		if !c.err && p.Version.String() != c.want {
			t.Errorf("%s: version = %s, want %s", c.kind, p.Version, c.want)
		}
	}
	p, _ := Resolve(f.g, st, "promote")
	if p.Target != rcCommit {
		t.Errorf("promote must tag the rc's commit %s, got %s", rcCommit, p.Target)
	}
	rc, _ := Resolve(f.g, st, "rc")
	if !strings.Contains(rc.Notes, "b") || strings.Contains(rc.Notes, "- a ") {
		t.Errorf("an rc's notes cover commits since the last tag only:\n%s", rc.Notes)
	}
	stable, _ := Resolve(f.g, st, "minor")
	if !strings.Contains(stable.Notes, "a") || !strings.Contains(stable.Notes, "b") {
		t.Errorf("a stable's notes cover everything since the last stable:\n%s", stable.Notes)
	}
}

func TestResolveRefusesAnAlreadyTaggedHead(t *testing.T) {
	f := newFixture(t)
	f.commit("feat: a")
	f.tag("v0.1.0-rc.1")
	st, _ := Inspect(f.g)
	if _, err := Resolve(f.g, st, "rc"); err == nil {
		t.Error("HEAD already carries v0.1.0-rc.1: cutting another tag on it must be refused")
	}
}

func TestResolveNothingToRelease(t *testing.T) {
	f := newFixture(t)
	f.commit("feat: a")
	f.tag("v0.1.0")
	f.commit("docs: b")
	st, _ := Inspect(f.g)
	if _, err := Resolve(f.g, st, "rc"); !errors.Is(err, ErrNothingToRelease) {
		t.Errorf("err = %v, want ErrNothingToRelease", err)
	}
}

func TestGateChecks(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(f *fixture)
		tests   func() error
		ci      func(string) (string, error)
		wantErr string
	}{
		{"all green", func(f *fixture) {}, okTests, nil, ""},
		{"dirty tree", func(f *fixture) {
			os.WriteFile(filepath.Join(f.work, "dirty.txt"), []byte("x"), 0o644)
		}, okTests, nil, "working tree"},
		{"wrong branch", func(f *fixture) { git(f.t, f.work, "checkout", "-b", "topic") }, okTests, nil, "main"},
		{"unpushed commit", func(f *fixture) { f.commit("feat: local only") }, okTests, nil, "origin/main"},
		{"behind origin", func(f *fixture) {
			other := filepath.Join(filepath.Dir(f.work), "other")
			git(f.t, filepath.Dir(f.work), "clone", f.remote, other)
			git(f.t, other, "config", "user.email", "o@example.com")
			git(f.t, other, "config", "user.name", "O")
			os.WriteFile(filepath.Join(other, "n.txt"), []byte("n"), 0o644)
			git(f.t, other, "add", "-A")
			git(f.t, other, "commit", "-m", "feat: remote work")
			git(f.t, other, "push", "origin", "main")
		}, okTests, nil, "origin/main"},
		{"tests fail", func(f *fixture) {}, func() error { return errors.New("FAIL pkg") }, nil, "go test"},
		{"ci red", func(f *fixture) {}, okTests, func(string) (string, error) { return "ci.yml failed", nil }, "CI"},
		{"ci unavailable is skipped", func(f *fixture) {}, okTests, func(string) (string, error) { return "", ErrNoCI }, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.commit("feat: a")
			f.push()
			c.prepare(f)
			var out bytes.Buffer
			gate := Gate{Git: f.g, Remote: "origin", Branch: "main", RunTests: c.tests, CI: c.ci}
			err := gate.Run(git(t, f.work, "rev-parse", "HEAD"), &out)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected failure: %v\n%s", err, out.String())
				}
				return
			}
			var gf gateFailed
			if !errors.As(err, &gf) || gf != 1 {
				t.Fatalf("err = %v, want one failed check\n%s", err, out.String())
			}
			if !strings.Contains(failLine(out.String()), c.wantErr) {
				t.Errorf("the failed check does not mention %q\n%s", c.wantErr, out.String())
			}
		})
	}
}

func TestGateSkipTestsInDryRun(t *testing.T) {
	f := newFixture(t)
	f.commit("feat: a")
	f.push()
	called := false
	gate := Gate{Git: f.g, Remote: "origin", Branch: "main", SkipTests: true,
		RunTests: func() error { called = true; return nil }}
	if err := gate.Run("", &bytes.Buffer{}); err != nil || called {
		t.Errorf("err = %v, tests called = %v; SkipTests must not run them", err, called)
	}
}

func TestCutCreatesAnAnnotatedTagAndPushesIt(t *testing.T) {
	f := newFixture(t)
	f.commit("feat: a")
	f.push()
	o, out := f.opts("rc", "y\n")
	if err := Cut(f.g, o); err != nil {
		t.Fatalf("Cut: %v\n%s", err, out)
	}
	if got := git(t, f.work, "cat-file", "-t", "v0.1.0-rc.1"); got != "tag" {
		t.Errorf("tag object type = %q, want an annotated tag", got)
	}
	if !strings.Contains(f.remoteTags(), "v0.1.0-rc.1") {
		t.Errorf("tag was not pushed to the remote: %q", f.remoteTags())
	}
	for _, want := range []string{"v0.1.0-rc.1", "Features", "environment", "approval"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestCutDeclineChangesNothing(t *testing.T) {
	f := newFixture(t)
	f.commit("feat: a")
	f.push()
	o, _ := f.opts("rc", "n\n")
	if err := Cut(f.g, o); !errors.Is(err, ErrAborted) {
		t.Errorf("err = %v, want ErrAborted", err)
	}
	if git(t, f.work, "tag", "--list") != "" || f.remoteTags() != "" {
		t.Error("declining must not create or push a tag")
	}
	o, _ = f.opts("rc", "") // EOF is a no too
	if err := Cut(f.g, o); !errors.Is(err, ErrAborted) {
		t.Errorf("EOF: err = %v, want ErrAborted", err)
	}
}

func TestCutDryRun(t *testing.T) {
	f := newFixture(t)
	f.commit("feat: a")
	f.push()

	o, out := f.opts("rc", "")
	o.DryRun, o.Gate.SkipTests = true, true
	if err := Cut(f.g, o); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if git(t, f.work, "tag", "--list") != "" {
		t.Error("dry run created a tag")
	}
	if !strings.Contains(out.String(), "0.1.0-rc.1") {
		t.Errorf("dry run must still show the version:\n%s", out)
	}
}

func TestCutStopsWhenChecksFail(t *testing.T) {
	f := newFixture(t)
	f.commit("feat: a")
	f.push()
	o, _ := f.opts("rc", "y\n")
	o.Gate.RunTests = func() error { return errors.New("boom") }
	if err := Cut(f.g, o); err == nil {
		t.Fatal("failing tests must stop the release")
	}
	if git(t, f.work, "tag", "--list") != "" {
		t.Error("a tag was created despite failing checks")
	}
}

func TestCutPromoteTagsTheRCCommit(t *testing.T) {
	f := newFixture(t)
	f.commit("feat: a")
	sha := git(t, f.work, "rev-parse", "HEAD")
	f.push()
	o, _ := f.opts("rc", "y\n")
	if err := Cut(f.g, o); err != nil {
		t.Fatal(err)
	}
	o, out := f.opts("promote", "y\n")
	if err := Cut(f.g, o); err != nil {
		t.Fatalf("promote: %v\n%s", err, out)
	}
	if got := git(t, f.work, "rev-list", "-n1", "v0.1.0"); got != sha {
		t.Errorf("v0.1.0 points at %s, want the rc commit %s", got, sha)
	}
	if !strings.Contains(f.remoteTags(), "v0.1.0\n") && !strings.HasSuffix(f.remoteTags(), "v0.1.0") {
		t.Errorf("stable tag not pushed: %q", f.remoteTags())
	}
}

// commitPath commits one file at a repo-relative path.
func (f *fixture) commitPath(path, subject string) string {
	f.t.Helper()
	p := filepath.Join(f.work, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(subject), 0o644); err != nil {
		f.t.Fatal(err)
	}
	git(f.t, f.work, "add", "-A")
	git(f.t, f.work, "commit", "-m", subject)
	return git(f.t, f.work, "rev-parse", "HEAD")
}

// cutRC cuts v0.1.0-rc.1 on a pushed main and returns the rc commit.
func (f *fixture) cutRC() string {
	f.t.Helper()
	f.commit("feat: a")
	f.push()
	o, out := f.opts("rc", "y\n")
	if err := Cut(f.g, o); err != nil {
		f.t.Fatalf("rc: %v\n%s", err, out)
	}
	return git(f.t, f.work, "rev-parse", "HEAD")
}

func TestCutPromoteAfterDocsOnlyCommits(t *testing.T) {
	f := newFixture(t)
	rc := f.cutRC()
	f.commitPath("docs/releasing.md", "docs: notes after the rc")
	f.commitPath("CHANGELOG.md", "docs: changelog")
	f.push()

	o, out := f.opts("promote", "y\n")
	if err := Cut(f.g, o); err != nil {
		t.Fatalf("promote after docs-only commits: %v\n%s", err, out)
	}
	if got := git(t, f.work, "rev-list", "-n1", "v0.1.0"); got != rc {
		t.Errorf("v0.1.0 points at %s, want the rc commit %s", got, rc)
	}
	if !strings.Contains(f.remoteTags(), "v0.1.0") {
		t.Errorf("stable tag not pushed: %q", f.remoteTags())
	}
	if strings.Contains(out.String(), "shipped code") {
		t.Errorf("docs-only commits must not warn about shipped code:\n%s", out)
	}
}

func TestCutPromoteWarnsAboutCodeCommitsAfterTheRC(t *testing.T) {
	f := newFixture(t)
	rc := f.cutRC()
	f.commitPath("internal/x/x.go", "fix: later code change")
	f.push()

	o, out := f.opts("promote", "y\n")
	if err := Cut(f.g, o); err != nil {
		t.Fatalf("promote: %v\n%s", err, out)
	}
	if got := git(t, f.work, "rev-list", "-n1", "v0.1.0"); got != rc {
		t.Errorf("v0.1.0 points at %s, want the rc commit %s", got, rc)
	}
	for _, want := range []string{"later code change", "shipped code", "new rc"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestCutPromoteRefusesAnRCNotOnMain(t *testing.T) {
	f := newFixture(t)
	f.commit("feat: a")
	f.push()
	git(t, f.work, "checkout", "-b", "side")
	f.commit("feat: side work")
	f.tag("v0.1.0-rc.1")
	git(t, f.work, "checkout", "main")

	o, out := f.opts("promote", "y\n")
	err := Cut(f.g, o)
	if !errors.Is(err, errRCNotOnMain) {
		t.Errorf("err = %v, want errRCNotOnMain\n%s", err, out)
	}
	if strings.Contains(git(t, f.work, "tag", "--list"), "v0.1.0\n") {
		t.Error("stable tag must not exist")
	}
}

func TestShipsCode(t *testing.T) {
	cases := map[string]bool{
		"docs/releasing.md": false, "tests/x_test.go": false, "odd/tasks/a.md": false,
		".claude/skills/x/SKILL.md": false, "CHANGELOG.md": false, "README.md": false,
		"internal/release/git.go": true, "cmd/aval/main.go": true, "go.mod": true,
		".github/workflows/release.yml": true,
	}
	for path, want := range cases {
		if got := shipsCode([]string{path}); got != want {
			t.Errorf("shipsCode(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestWorkflowURL(t *testing.T) {
	want := "https://github.com/o/r/actions/workflows/release.yml"
	for _, in := range []string{
		"git@github.com:o/r.git", "https://github.com/o/r.git", "https://github.com/o/r", "ssh://git@github.com/o/r.git",
	} {
		if got := WorkflowURL(in); got != want {
			t.Errorf("WorkflowURL(%q) = %q, want %q", in, got, want)
		}
	}
	if got := WorkflowURL("/tmp/origin.git"); got != "" {
		t.Errorf("a local path has no workflow page, got %q", got)
	}
}

// failLine returns the gate's [FAIL] line, or "" when there is none.
func failLine(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "[FAIL]") {
			return l
		}
	}
	return ""
}

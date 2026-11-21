package release

import (
	"errors"
	"strings"
	"testing"
)

func cs(subjects ...string) []Commit {
	var out []Commit
	for i, s := range subjects {
		body := ""
		if j := strings.Index(s, "\n"); j >= 0 {
			s, body = s[:j], s[j+1:]
		}
		out = append(out, ParseCommit(string(rune('a'+i))+"123456", s, body))
	}
	return out
}

func vs(tags ...string) []Version {
	var out []Version
	for _, t := range tags {
		v, err := Parse(t)
		if err != nil {
			panic(err)
		}
		out = append(out, v)
	}
	return out
}

func TestParseCommit(t *testing.T) {
	cases := []struct {
		subject, body string
		want          Commit
	}{
		{"feat: add x", "", Commit{Hash: "h", Type: "feat", Subject: "add x"}},
		{"fix(tmux): crash", "", Commit{Hash: "h", Type: "fix", Scope: "tmux", Subject: "crash"}},
		{"feat(api)!: drop v1", "", Commit{Hash: "h", Type: "feat", Scope: "api", Subject: "drop v1", Breaking: true}},
		{"refactor!: x", "", Commit{Hash: "h", Type: "refactor", Subject: "x", Breaking: true}},
		{"fix: x", "details\n\nBREAKING CHANGE: gone", Commit{Hash: "h", Type: "fix", Subject: "x", Breaking: true}},
		{"fix: x", "BREAKING-CHANGE: gone", Commit{Hash: "h", Type: "fix", Subject: "x", Breaking: true}},
		{"Merge branch 'x'", "", Commit{Hash: "h", Subject: "Merge branch 'x'"}},
		{"FEAT: loud", "", Commit{Hash: "h", Subject: "FEAT: loud"}},
	}
	for _, c := range cases {
		if got := ParseCommit("h", c.subject, c.body); got != c.want {
			t.Errorf("ParseCommit(%q, %q) = %+v, want %+v", c.subject, c.body, got, c.want)
		}
	}
}

func TestNext(t *testing.T) {
	cases := []struct {
		name    string
		tags    []Version
		commits []Commit
		pre     string
		want    string
		wantErr error
	}{
		{"first release from feat", nil, cs("feat: a", "fix: b"), "", "0.1.0", nil},
		{"first release only fix", nil, cs("fix: b"), "", "0.0.1", nil},
		{"pre-1.0 feat is minor", vs("v0.1.0"), cs("feat: a"), "", "0.2.0", nil},
		{"pre-1.0 breaking is minor", vs("v0.1.0"), cs("feat!: a"), "", "0.2.0", nil},
		{"pre-1.0 breaking footer is minor", vs("v0.1.0"), cs("fix: a\nBREAKING CHANGE: x"), "", "0.2.0", nil},
		{"pre-1.0 fix is patch", vs("v0.1.0"), cs("fix: a"), "", "0.1.1", nil},
		{"pre-1.0 perf is patch", vs("v0.1.4"), cs("perf: a"), "", "0.1.5", nil},
		{"1.x feat is minor", vs("v1.2.3"), cs("feat: a", "fix: b"), "", "1.3.0", nil},
		{"1.x breaking is major", vs("v1.2.3"), cs("feat: a", "refactor!: b"), "", "2.0.0", nil},
		{"1.x fix is patch", vs("v1.2.3"), cs("fix: a", "docs: b"), "", "1.2.4", nil},
		{"only docs and chores", vs("v1.2.3"), cs("docs: a", "chore: b", "ci: c", "test: d", "refactor: e"), "", "", ErrNothingToRelease},
		{"no commits", vs("v1.2.3"), nil, "", "", ErrNothingToRelease},
		{"non conventional commits ignored", vs("v1.2.3"), cs("wip stuff"), "", "", ErrNothingToRelease},
		{"highest stable wins over order", vs("v0.2.0", "v0.10.0", "v0.9.0"), cs("fix: a"), "", "0.10.1", nil},
		{"first rc", vs("v0.1.0"), cs("feat: a"), "rc", "0.2.0-rc.1", nil},
		{"first beta", vs("v0.1.0"), cs("fix: a"), "beta", "0.1.1-beta.1", nil},
		{"rc increments", vs("v0.1.0", "v0.2.0-rc.1"), cs("feat: a", "fix: b"), "rc", "0.2.0-rc.2", nil},
		{"rc after beta restarts at 1", vs("v0.1.0", "v0.2.0-beta.3"), cs("feat: a"), "rc", "0.2.0-rc.1", nil},
		{"beta after rc is refused", vs("v0.1.0", "v0.2.0-rc.1"), cs("feat: a"), "beta", "", errBetaAfterRC},
		{"stable promotes the rc target", vs("v0.1.0", "v0.2.0-rc.2"), cs("feat: a"), "", "0.2.0", nil},
		{"rc target grows with a breaking change", vs("v1.0.0", "v1.0.1-rc.1"), cs("fix: a", "feat!: b"), "rc", "2.0.0-rc.1", nil},
		{"unknown pre kind", vs("v0.1.0"), cs("feat: a"), "alpha", "", errBadPre},
	}
	for _, c := range cases {
		got, err := Next(c.tags, c.commits, c.pre)
		if !errors.Is(err, c.wantErr) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.wantErr)
			continue
		}
		if c.wantErr == nil && got.String() != c.want {
			t.Errorf("%s: Next = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestBump(t *testing.T) {
	cases := []struct {
		tags  []Version
		level string
		want  string
		err   bool
	}{
		{vs("v1.2.3"), "patch", "1.2.4", false},
		{vs("v1.2.3"), "minor", "1.3.0", false},
		{vs("v1.2.3"), "major", "2.0.0", false},
		{nil, "minor", "0.1.0", false},
		{nil, "patch", "0.0.1", false},
		{vs("v0.2.0", "v0.3.0-rc.1"), "minor", "0.3.0", false}, // rc is not stable: bump the stable
		{vs("v1.0.0"), "huge", "", true},
	}
	for _, c := range cases {
		got, err := Bump(c.tags, c.level)
		if (err != nil) != c.err {
			t.Errorf("Bump(%v, %s) err = %v", c.tags, c.level, err)
			continue
		}
		if !c.err && got.String() != c.want {
			t.Errorf("Bump(%v, %s) = %s, want %s", c.tags, c.level, got, c.want)
		}
	}
}

func TestPromote(t *testing.T) {
	got, err := Promote(vs("v0.1.0", "v0.2.0-rc.1", "v0.2.0-rc.2"))
	if err != nil || got.String() != "0.2.0" {
		t.Errorf("Promote = %v, %v; want 0.2.0", got, err)
	}
	// An rc whose stable already exists has nothing to promote.
	if _, err := Promote(vs("v0.2.0", "v0.2.0-rc.1")); !errors.Is(err, errNoRC) {
		t.Errorf("Promote with stable present: err = %v, want errNoRC", err)
	}
	if _, err := Promote(vs("v0.1.0")); !errors.Is(err, errNoRC) {
		t.Errorf("Promote with no rc: err = %v, want errNoRC", err)
	}
	// Betas are not promotable: only a release candidate is.
	if _, err := Promote(vs("v0.1.0", "v0.2.0-beta.1")); !errors.Is(err, errNoRC) {
		t.Errorf("Promote of a beta: err = %v, want errNoRC", err)
	}
}

func TestNotes(t *testing.T) {
	got := Notes(cs(
		"feat(tui): add picker",
		"fix: crash on empty config",
		"perf(budget): cache walk",
		"feat!: drop legacy flag",
		"docs: readme",
		"test: more",
		"ci: pin",
		"chore: deps",
		"refactor: tidy",
		"wip",
	))
	for _, want := range []string{
		"### Breaking changes", "drop legacy flag",
		"### Features", "**tui:** add picker",
		"### Fixes", "crash on empty config",
		"### Performance", "**budget:** cache walk",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("notes missing %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"readme", "more", "pin", "deps", "tidy", "wip"} {
		if strings.Contains(got, bad) {
			t.Errorf("notes must skip %q:\n%s", bad, got)
		}
	}
	if strings.Index(got, "Breaking") > strings.Index(got, "Features") {
		t.Errorf("breaking changes must come first:\n%s", got)
	}
	if !strings.Contains(got, "a123456"[:7]) {
		t.Errorf("notes should carry the short hash:\n%s", got)
	}
	if Notes(cs("docs: x")) != "" {
		t.Error("notes of only skipped commits must be empty")
	}
}

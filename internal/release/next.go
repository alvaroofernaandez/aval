package release

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Commit is one conventional commit. Type is empty when the subject does not
// follow the convention.
type Commit struct {
	Hash     string
	Type     string
	Scope    string
	Subject  string
	Breaking bool
}

var (
	// ErrNothingToRelease means no commit since the last stable release
	// warrants a version bump (only docs, tests, CI or chores).
	ErrNothingToRelease = errors.New("nothing to release: no feat, fix, perf or breaking commit since the last stable tag")

	errBetaAfterRC = errors.New("a beta cannot follow a release candidate of the same version; cut another rc or promote")
	errBadPre      = errors.New("prerelease kind must be rc or beta")
	errNoRC        = errors.New("no release candidate to promote")
	errBadLevel    = errors.New("level must be patch, minor or major")

	subjectRe  = regexp.MustCompile(`^([a-z]+)(?:\(([^)]*)\))?(!)?: (.+)$`)
	breakingRe = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE:`)
	preRe      = regexp.MustCompile(`^(rc|beta)\.(\d+)$`)
)

// ParseCommit reads a commit's subject and body.
func ParseCommit(hash, subject, body string) Commit {
	c := Commit{Hash: hash, Subject: subject}
	m := subjectRe.FindStringSubmatch(subject)
	if m == nil {
		return c
	}
	c.Type, c.Scope, c.Subject = m[1], m[2], m[4]
	c.Breaking = m[3] == "!" || breakingRe.MatchString(body)
	return c
}

func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// Notes renders grouped markdown release notes. docs, test, ci, chore and the
// like are left out: they do not change what a user gets.
func Notes(commits []Commit) string {
	sections := []struct {
		title string
		match func(Commit) bool
	}{
		{"Breaking changes", func(c Commit) bool { return c.Breaking }},
		{"Features", func(c Commit) bool { return c.Type == "feat" && !c.Breaking }},
		{"Fixes", func(c Commit) bool { return c.Type == "fix" && !c.Breaking }},
		{"Performance", func(c Commit) bool { return c.Type == "perf" && !c.Breaking }},
	}
	var b strings.Builder
	for _, s := range sections {
		var lines []string
		for _, c := range commits {
			if c.Type == "" || !s.match(c) {
				continue
			}
			scope := ""
			if c.Scope != "" {
				scope = "**" + c.Scope + ":** "
			}
			lines = append(lines, fmt.Sprintf("- %s%s (%s)", scope, c.Subject, shortHash(c.Hash)))
		}
		if len(lines) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "### %s\n\n%s\n", s.title, strings.Join(lines, "\n"))
	}
	return b.String()
}

// LastStable is the highest stable tag, or 0.0.0 when there is none.
func LastStable(tags []Version) Version {
	var best Version
	for _, t := range tags {
		if !t.IsPre() && t.Compare(best) > 0 {
			best = t
		}
	}
	return best
}

func bumped(v Version, level string) (Version, error) {
	switch level {
	case "major":
		return Version{Major: v.Major + 1}, nil
	case "minor":
		return Version{Major: v.Major, Minor: v.Minor + 1}, nil
	case "patch":
		return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}, nil
	}
	return Version{}, errBadLevel
}

// levelFor is the bump the commits call for. Before 1.0.0 a breaking change
// only moves the minor: the API is not promised stable yet.
func levelFor(last Version, commits []Commit) string {
	level := ""
	for _, c := range commits {
		switch {
		case c.Breaking:
			if last.Major == 0 {
				return "minor"
			}
			return "major"
		case c.Type == "feat":
			level = "minor"
		case (c.Type == "fix" || c.Type == "perf") && level == "":
			level = "patch"
		}
	}
	return level
}

func kindRank(kind string) int {
	if kind == "rc" {
		return 2
	}
	return 1
}

// Next computes the next version from every v* tag and the commits since the
// last stable tag. pre is "" (stable), "rc" or "beta".
func Next(tags []Version, commits []Commit, pre string) (Version, error) {
	if pre != "" && pre != "rc" && pre != "beta" {
		return Version{}, errBadPre
	}
	last := LastStable(tags)
	level := levelFor(last, commits)
	if level == "" {
		return Version{}, ErrNothingToRelease
	}
	target, _ := bumped(last, level)
	if pre == "" {
		return target, nil
	}

	// Highest existing prerelease of this target, if any.
	var kind string
	var num int
	for _, t := range tags {
		m := preRe.FindStringSubmatch(t.Pre)
		if m == nil || t.Major != target.Major || t.Minor != target.Minor || t.Patch != target.Patch {
			continue
		}
		n, _ := strconv.Atoi(m[2])
		if kindRank(m[1]) > kindRank(kind) || (m[1] == kind && n > num) {
			kind, num = m[1], n
		}
	}
	switch {
	case kind == "":
		num = 1
	case kind == pre:
		num++
	case kindRank(pre) < kindRank(kind):
		return Version{}, errBetaAfterRC
	default:
		num = 1
	}
	target.Pre = fmt.Sprintf("%s.%d", pre, num)
	return target, nil
}

// Bump returns the stable version for an explicit level (patch, minor, major).
func Bump(tags []Version, level string) (Version, error) {
	return bumped(LastStable(tags), level)
}

// Promote returns the stable version of the latest release candidate whose
// stable release does not exist yet.
func Promote(tags []Version) (Version, error) {
	var best Version
	found := false
	for _, t := range tags {
		if !t.IsPre() {
			continue
		}
		base := Version{Major: t.Major, Minor: t.Minor, Patch: t.Patch}
		if hasStable(tags, base) {
			continue
		}
		if !found || t.Compare(best) > 0 {
			best, found = t, true
		}
	}
	if !found || !strings.HasPrefix(best.Pre, "rc.") {
		return Version{}, errNoRC
	}
	return Version{Major: best.Major, Minor: best.Minor, Patch: best.Patch}, nil
}

func hasStable(tags []Version, v Version) bool {
	for _, t := range tags {
		if t == v {
			return true
		}
	}
	return false
}

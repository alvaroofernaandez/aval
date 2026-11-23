package tests

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Every release from v0.2.0 on has hand-written notes in English and Spanish,
// and its CHANGELOG section links both.

var changelogRelease = regexp.MustCompile(`(?m)^## v(\d+)\.(\d+)\.(\d+)(?:-[0-9A-Za-z.]+)? `)

// releaseNoteProblems returns one line per CHANGELOG release section (v0.2.0
// and later) that lacks a link to, or the file of, either language's notes.
func releaseNoteProblems(changelog string, fileExists func(rel string) bool) []string {
	var out []string
	idx := changelogRelease.FindAllStringSubmatchIndex(changelog, -1)
	for n, m := range idx {
		major, _ := strconv.Atoi(changelog[m[2]:m[3]])
		minor, _ := strconv.Atoi(changelog[m[4]:m[5]])
		patch, _ := strconv.Atoi(changelog[m[6]:m[7]])
		if major == 0 && minor < 2 {
			continue
		}
		end := len(changelog)
		if n+1 < len(idx) {
			end = idx[n+1][0]
		}
		heading := strings.SplitN(changelog[m[0]:], "\n", 2)[0]
		section := changelog[m[0]:end]
		base := fmt.Sprintf("v%d.%d.%d", major, minor, patch)
		for _, rel := range []string{"docs/releases/" + base + ".md", "docs/releases/" + base + ".es.md"} {
			if !strings.Contains(section, "("+rel+")") {
				out = append(out, heading+": does not link "+rel)
			}
			if !fileExists(rel) {
				out = append(out, heading+": "+rel+" does not exist")
			}
		}
	}
	return out
}

func TestChangelogReleasesLinkBilingualNotes(t *testing.T) {
	for _, problem := range releaseNoteProblems(read(t, "CHANGELOG.md"), func(rel string) bool { return exists(p(rel)) }) {
		t.Error(problem)
	}
}

func TestReleaseNoteProblemsCatchesMissingNotes(t *testing.T) {
	log := "# Changelog\n\n## v0.2.0-rc.1 (2026-11-23)\n\n[Release notes](docs/releases/v0.2.0.md)\n\n## v0.3.0 (2027-01-01)\n\n" +
		"[a](docs/releases/v0.3.0.md) [b](docs/releases/v0.3.0.es.md)\n\n## v0.1.0 (2026-01-01)\n\nold\n"
	got := releaseNoteProblems(log, func(rel string) bool { return rel != "docs/releases/v0.2.0.es.md" })
	want := []string{
		"## v0.2.0-rc.1 (2026-11-23): does not link docs/releases/v0.2.0.es.md",
		"## v0.2.0-rc.1 (2026-11-23): docs/releases/v0.2.0.es.md does not exist",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems:\n got %q\nwant %q", got, want)
	}
}

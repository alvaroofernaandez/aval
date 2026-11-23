package tests

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// The hub is the front door, so the docs that tell people how to start must
// describe it, and must not claim that a bare `aval` always prints the usage.

var hubEntries = []string{"Install", "Update", "Sync", "Doctor", "Backups", "Uninstall", "Settings", "Dev sandbox", "About"}

func TestDocsDescribeTheHub(t *testing.T) {
	golden := read(t, "internal/ui/testdata/view_hub_unicode_80x24.golden")
	var menuLine string
	for _, l := range strings.Split(golden, "\n") {
		if strings.Contains(l, "Set up  › ⇣ Install") {
			menuLine = strings.TrimRight(l, " ")
		}
	}
	if menuLine == "" {
		t.Fatal("golden has no menu line")
	}
	for _, doc := range []string{"README.md", "docs/guide.md", "npm/cli/README.md"} {
		t.Run(doc, func(t *testing.T) {
			text := read(t, doc)
			mustContain(t, text, "hub", doc+" names the hub")
			mustContain(t, text, menuLine, doc+" shows the hub excerpt from the golden")
			for _, e := range hubEntries {
				if !regexp.MustCompile("(?m)^- \\*\\*" + regexp.QuoteMeta(e) + "\\*\\*").MatchString(text) {
					t.Errorf("%s: Views list has no line for %q", doc, e)
				}
			}
			for _, l := range strings.Split(text, "\n") {
				if strings.Contains(l, "prints the usage") &&
					!regexp.MustCompile(`(?i)without a terminal|no terminal|not a terminal|pipe`).MatchString(l) {
					t.Errorf("%s: unqualified usage claim: %q", doc, l)
				}
				if strings.ContainsAny(l, "▗▄▜█") && utf8.RuneCountInString(l) > 80 {
					t.Errorf("%s: excerpt line wider than 80 columns: %q", doc, l)
				}
			}
		})
	}
	mustContain(t, read(t, "README.md"), "docs/assets/hub.gif", "README shows the hub demo")
	mustContain(t, read(t, "cmd/aval/main.go"), "opens the hub", "main.go header describes the hub")
	for _, f := range []string{"docs/hub-demo.tape", "docs/assets/hub.gif"} {
		if !exists(p(f)) {
			t.Errorf("%s is missing", f)
		}
	}
}

func TestChangelogHasHubInItsRelease(t *testing.T) {
	text := read(t, "CHANGELOG.md")
	i := strings.Index(text, "## v0.2.0-rc.1 (")
	if i < 0 {
		t.Fatal("CHANGELOG has no v0.2.0-rc.1 section")
	}
	end := strings.Index(text[i+3:], "\n## ")
	section := text[i:]
	if end >= 0 {
		section = text[i : i+3+end]
	}
	for _, w := range []string{"hub", "rate-guard", "theme", "quit"} {
		mustContain(t, strings.ToLower(section), w, "v0.2.0-rc.1 mentions "+w)
	}
}

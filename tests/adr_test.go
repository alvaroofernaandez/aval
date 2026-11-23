package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The ADR rules live in docs/adr/README.md. These tests keep the records
// uniform so that "never rewrite, amend with a date" stays checkable.

var (
	adrFileRe    = regexp.MustCompile(`^(\d{4})-[a-z0-9]+(-[a-z0-9]+)*\.md$`)
	adrStatusRe  = regexp.MustCompile(`(?m)^Status: \S`)
	adrDateRe    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\b`)
	adrLinkRe    = regexp.MustCompile(`\]\(([^)#\s]+)(#[^)]*)?\)`)
	adrRequired  = []string{"Context", "Decision", "Rejected alternatives", "Consequences"}
	adrSectionRe = regexp.MustCompile(`(?m)^## (.+?)\s*$`)
)

func adrDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repo, "docs", "adr")
}

// adrFiles returns the NNNN-kebab.md file names, sorted, and fails on any
// other markdown file in the directory (except README.md).
func adrFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(adrDir(t))
	if err != nil {
		t.Fatalf("docs/adr is missing: %v", err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if n == "README.md" || e.IsDir() {
			continue
		}
		if !adrFileRe.MatchString(n) {
			t.Errorf("docs/adr/%s does not match NNNN-kebab-case.md", n)
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatal("docs/adr has no ADR files")
	}
	return names
}

func adrRead(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(adrDir(t), name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestADRHaveTheTemplateSections(t *testing.T) {
	for _, name := range adrFiles(t) {
		body := adrRead(t, name)
		if !adrStatusRe.MatchString(body) {
			t.Errorf("%s: no `Status: <value>` line", name)
		}
		have := map[string]bool{}
		for _, m := range adrSectionRe.FindAllStringSubmatch(body, -1) {
			have[m[1]] = true
		}
		for _, s := range adrRequired {
			if !have[s] {
				t.Errorf("%s: missing the `## %s` section", name, s)
			}
		}
	}
}

func TestADRNumberingIsContiguous(t *testing.T) {
	for i, name := range adrFiles(t) {
		n, _ := strconv.Atoi(adrFileRe.FindStringSubmatch(name)[1])
		if n != i+1 {
			t.Errorf("%s: expected number %04d (numbering starts at 0001 and has no gaps)", name, i+1)
		}
	}
}

func TestADRIndexListsEveryADRAndLinksResolve(t *testing.T) {
	readme := adrRead(t, "README.md")
	for _, name := range adrFiles(t) {
		if !strings.Contains(readme, "]("+name+")") {
			t.Errorf("README.md index does not link %s", name)
		}
	}
	for _, m := range adrLinkRe.FindAllStringSubmatch(readme, -1) {
		target := m[1]
		if strings.Contains(target, "://") {
			continue
		}
		if _, err := os.Stat(filepath.Join(adrDir(t), target)); err != nil {
			t.Errorf("README.md links %s, which does not exist", target)
		}
	}
}

func TestADRAmendmentsAreDated(t *testing.T) {
	for _, name := range adrFiles(t) {
		body := adrRead(t, name)
		idx := strings.Index(body, "\n## Amendments")
		if idx < 0 {
			continue
		}
		section := body[idx+len("\n## Amendments"):]
		if next := adrSectionRe.FindStringIndex(section); next != nil {
			section = section[:next[0]]
		}
		entries := 0
		for _, line := range strings.Split(section, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "### ") {
				continue
			}
			entries++
			if !adrDateRe.MatchString(strings.TrimPrefix(line, "### ")) {
				t.Errorf("%s: amendment heading %q does not start with YYYY-MM-DD", name, line)
			}
		}
		if entries == 0 {
			t.Errorf("%s: an Amendments section needs at least one `### YYYY-MM-DD` entry", name)
		}
	}
}

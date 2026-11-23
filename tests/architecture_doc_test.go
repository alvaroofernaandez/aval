package tests

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// ARCHITECTURE.md is the one page a newcomer reads first. These tests keep it
// one page and keep its package map true: every Go package has a row, and no
// row names a package that is gone. They walk the tree with fs.WalkDir, not
// go list, so they stay fast.

const archDoc = "ARCHITECTURE.md"

// archDocMaxLines keeps the page a page.
const archDocMaxLines = 150

// archDocSections are the fixed headings, in order.
var archDocSections = []string{"## Package map", "## Data flow", "## Where a change goes"}

// goPackageDirs lists every directory under cmd/, internal/ and tools/ that
// holds a non-test .go file.
func goPackageDirs(t *testing.T, fsys fs.FS) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, top := range []string{"cmd", "internal", "tools"} {
		if _, err := fs.Stat(fsys, top); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		err := fs.WalkDir(fsys, top, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
				return fs.SkipDir
			}
			if !d.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
				seen[path.Dir(p)] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// archMapRows returns the package paths in the first column of the package
// map table.
func archMapRows(doc string) []string {
	var out []string
	in := false
	for _, ln := range strings.Split(doc, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "## ") {
			in = t == archDocSections[0]
			continue
		}
		if !in || !strings.HasPrefix(t, "| `") {
			continue
		}
		cell := strings.TrimSpace(strings.SplitN(strings.Trim(t, "|"), "|", 2)[0])
		out = append(out, strings.Trim(cell, "`"))
	}
	return out
}

// archDocProblems compares the page with the packages on disk.
func archDocProblems(pkgs []string, doc string) []string {
	var out []string
	lines := strings.Count(doc, "\n")
	if lines > archDocMaxLines {
		out = append(out, "the page has "+strconv.Itoa(lines)+" lines; keep it under "+strconv.Itoa(archDocMaxLines))
	}
	last := -1
	for _, h := range archDocSections {
		i := strings.Index(doc, "\n"+h+"\n")
		if i < 0 {
			out = append(out, "missing section "+h)
			continue
		}
		if i < last {
			out = append(out, "section "+h+" is out of order")
		}
		last = i
	}
	rows := map[string]bool{}
	for _, r := range archMapRows(doc) {
		rows[r] = true
	}
	exists := map[string]bool{}
	for _, p := range pkgs {
		exists[p] = true
		if !rows[p] {
			out = append(out, p+" is a package with no row in the package map")
		}
	}
	for _, r := range archMapRows(doc) {
		if !exists[r] {
			out = append(out, r+" has a row in the package map but is not a package; delete the row")
		}
	}
	return out
}

func TestArchitectureDocProblemsCatchesDrift(t *testing.T) {
	fsys := fstest.MapFS{
		"cmd/aval/main.go":            {Data: []byte("package main")},
		"internal/core/plan/plan.go":  {Data: []byte("package plan")},
		"internal/new/new.go":         {Data: []byte("package new")},
		"internal/only/only_test.go":  {Data: []byte("package only")},
		"internal/x/testdata/fake.go": {Data: []byte("package fake")},
	}
	doc := strings.Join([]string{
		"# Architecture", "", "## Package map", "",
		"| Package | Does |", "| :-- | :-- |",
		"| `cmd/aval` | the binary |",
		"| `internal/core/plan` | decides |",
		"| `internal/gone` | removed |",
		"", "## Data flow", "", "## Where a change goes", "",
	}, "\n")
	got := strings.Join(archDocProblems(goPackageDirs(t, fsys), doc), "\n")
	for _, want := range []string{"internal/new is a package with no row", "internal/gone has a row"} {
		if !strings.Contains(got, want) {
			t.Errorf("problems do not mention %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"internal/only", "testdata", "missing section", "lines"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("problems mention %q, which is fine:\n%s", unwanted, got)
		}
	}
	long := doc + strings.Repeat("\n", archDocMaxLines)
	if !strings.Contains(strings.Join(archDocProblems(nil, long), "\n"), "lines") {
		t.Error("a page over the line limit must be a problem")
	}
	if !strings.Contains(strings.Join(archDocProblems(nil, "# A\n"), "\n"), "missing section ## Data flow") {
		t.Error("a missing section must be a problem")
	}
}

func TestArchitectureDocMatchesTheRepo(t *testing.T) {
	doc, err := os.ReadFile(p(archDoc))
	if err != nil {
		t.Fatalf("%s: %v", archDoc, err)
	}
	for _, problem := range archDocProblems(goPackageDirs(t, os.DirFS(repo)), string(doc)) {
		t.Errorf("%s: %s", archDoc, problem)
	}
}

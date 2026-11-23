package tests

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

// Dependency budget: every direct dependency has a row with its reason in
// docs/dependencies.md, and no row outlives its dependency. Admitting a
// dependency is a decision, so it is made in the same PR, in writing.
//
// Only the standard library: go.mod is read line by line and imports come from
// go/parser, since golang.org/x/mod would itself be a new dependency.

const budgetDoc = "docs/dependencies.md"

// budgetSources maps a section heading of the budget to the manifest it covers.
var budgetSources = map[string]string{
	"Go module":                          "go.mod",
	"npm package (npm/cli)":              "npm/cli/package.json",
	"OpenCode plugins (config/opencode)": "config/opencode/package.json",
}

type goRequire struct {
	path, version string
	indirect      bool
}

// goRequires reads the require entries of a go.mod.
func goRequires(src string) []goRequire {
	var out []goRequire
	block := false
	for _, ln := range strings.Split(src, "\n") {
		t := strings.TrimSpace(ln)
		switch {
		case t == "require (":
			block = true
			continue
		case block && t == ")":
			block = false
			continue
		case strings.HasPrefix(t, "require "):
			t = strings.TrimSpace(strings.TrimPrefix(t, "require "))
		case !block:
			continue
		}
		f := strings.Fields(t)
		if len(f) < 2 {
			continue
		}
		out = append(out, goRequire{path: f[0], version: f[1], indirect: strings.Contains(t, "// indirect")})
	}
	return out
}

// goImports returns every import path of the module's .go files, tests
// included: go mod tidy counts a test import as direct too.
func goImports(t *testing.T, fsys fs.FS) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	fset := token.NewFileSet()
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if p != "." && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "testdata") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		src, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, p, src, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			out[strings.Trim(imp.Path.Value, `"`)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// directGoModules maps each import to the longest required module that
// contains it.
func directGoModules(reqs []goRequire, imports map[string]bool) map[string]bool {
	out := map[string]bool{}
	for imp := range imports {
		best := ""
		for _, r := range reqs {
			if (imp == r.path || strings.HasPrefix(imp, r.path+"/")) && len(r.path) > len(best) {
				best = r.path
			}
		}
		if best != "" {
			out[best] = true
		}
	}
	return out
}

// npmDependencies lists the dependency names of a package.json.
func npmDependencies(src []byte) (map[string]bool, error) {
	var pkg map[string]json.RawMessage
	if err := json.Unmarshal(src, &pkg); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, key := range []string{"dependencies", "devDependencies", "peerDependencies", "optionalDependencies"} {
		raw, ok := pkg[key]
		if !ok {
			continue
		}
		var deps map[string]string
		if err := json.Unmarshal(raw, &deps); err != nil {
			return nil, err
		}
		for name := range deps {
			out[name] = true
		}
	}
	return out, nil
}

type budgetRow struct{ name, reason string }

// parseBudget reads the tables of the budget doc, keyed by section heading.
// A row is "| `name` | version | reason | rejected alternative |".
func parseBudget(doc string) map[string][]budgetRow {
	out := map[string][]budgetRow{}
	section := ""
	for _, ln := range strings.Split(doc, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "## ") {
			section = strings.TrimPrefix(t, "## ")
			continue
		}
		if section == "" || !strings.HasPrefix(t, "| `") {
			continue
		}
		cells := strings.Split(strings.Trim(t, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		row := budgetRow{name: strings.Trim(cells[0], "`")}
		if len(cells) > 2 {
			row.reason = cells[2]
		}
		out[section] = append(out[section], row)
	}
	return out
}

// budgetProblems compares the budget with the real dependencies, per section.
func budgetProblems(actual map[string]map[string]bool, budget map[string][]budgetRow) []string {
	var out []string
	sections := make([]string, 0, len(actual))
	for s := range actual {
		sections = append(sections, s)
	}
	sort.Strings(sections)
	for _, s := range sections {
		listed := map[string]bool{}
		for _, r := range budget[s] {
			listed[r.name] = true
			if !actual[s][r.name] {
				out = append(out, s+": "+r.name+" has a row but is no longer a dependency; delete the row")
			}
			if strings.TrimSpace(r.reason) == "" {
				out = append(out, s+": "+r.name+" has no reason")
			}
		}
		var missing []string
		for name := range actual[s] {
			if !listed[name] {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		for _, name := range missing {
			out = append(out, s+": "+name+" is a direct dependency with no row; add it with its reason in the same PR")
		}
	}
	return out
}

// repoDependencies reads the direct dependencies of every manifest.
func repoDependencies(t *testing.T, fsys fs.FS) map[string]map[string]bool {
	t.Helper()
	actual := map[string]map[string]bool{}
	for section, file := range budgetSources {
		src, err := fs.ReadFile(fsys, file)
		if err != nil {
			t.Fatal(err)
		}
		if file == "go.mod" {
			actual[section] = directGoModules(goRequires(string(src)), goImports(t, fsys))
			continue
		}
		deps, err := npmDependencies(src)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		actual[section] = deps
	}
	return actual
}

func TestDependencyGoModMarksNoDirectImportIndirect(t *testing.T) {
	root := os.DirFS(repo)
	src, err := fs.ReadFile(root, "go.mod")
	if err != nil {
		t.Fatal(err)
	}
	reqs := goRequires(string(src))
	direct := directGoModules(reqs, goImports(t, root))
	for _, r := range reqs {
		if r.indirect && direct[r.path] {
			t.Errorf("go.mod marks %s // indirect, but the code imports it: run go mod tidy", r.path)
		}
	}
}

func TestDependencyBudgetProblemsCatchesDrift(t *testing.T) {
	fsys := fstest.MapFS{
		"go.mod":                       {Data: []byte("module x\n\nrequire (\n\tex.com/a v1.0.0\n\tex.com/b v1.0.0 // indirect\n)\n")},
		"main.go":                      {Data: []byte("package main\n\nimport _ \"ex.com/a/sub\"\n")},
		"npm/cli/package.json":         {Data: []byte(`{"name":"x"}`)},
		"config/opencode/package.json": {Data: []byte(`{"dependencies":{"p":"1","q":"1"}}`)},
	}
	budget := parseBudget(strings.Join([]string{
		"## Go module", "", "| Dependency | Version | Reason | Instead of |", "| :-- | :-- | :-- | :-- |",
		"| `ex.com/a` | v1 | the TUI | none |",
		"## OpenCode plugins (config/opencode)", "",
		"| `p` | 1 |  | none |",
		"| `gone` | 1 | was used | none |",
	}, "\n"))
	got := strings.Join(budgetProblems(repoDependencies(t, fsys), budget), "\n")
	for _, want := range []string{"q is a direct dependency with no row", "gone has a row but is no longer", "p has no reason"} {
		if !strings.Contains(got, want) {
			t.Errorf("problems do not mention %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "ex.com/a") || strings.Contains(got, "ex.com/b") {
		t.Errorf("a listed direct module or an indirect one must not be a problem:\n%s", got)
	}
}

func TestDependencyBudgetMatchesTheRepo(t *testing.T) {
	root := repo
	doc, err := os.ReadFile(filepath.Join(root, budgetDoc))
	if err != nil {
		t.Fatalf("%s: %v", budgetDoc, err)
	}
	for _, p := range budgetProblems(repoDependencies(t, os.DirFS(root)), parseBudget(string(doc))) {
		t.Errorf("%s: %s", budgetDoc, p)
	}
}

package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The catalog rules. Each check runs on the real catalog and on a planted
// broken one, so a check that stops finding anything fails too.

// missingKeys lists "lang: key" for every English key another language lacks,
// and every key a language has that English does not define.
func missingKeys(cats map[Lang]catalog) []string {
	var out []string
	for _, l := range Languages()[1:] {
		for k := range cats[English] {
			if _, ok := cats[l][k]; !ok {
				out = append(out, string(l)+": "+k)
			}
		}
		for k := range cats[l] {
			if _, ok := cats[English][k]; !ok {
				out = append(out, string(l)+": "+k)
			}
		}
	}
	sort.Strings(out)
	return out
}

// placeholders returns the sorted, distinct {names} in m's forms.
func placeholders(m message) []string {
	var names []string
	for _, form := range []string{m.one, m.other} {
		for _, sub := range placeholder.FindAllStringSubmatch(form, -1) {
			if !slices.Contains(names, sub[1]) {
				names = append(names, sub[1])
			}
		}
	}
	sort.Strings(names)
	return names
}

// placeholderMismatches lists "lang: key" where a translation's placeholders
// differ from the English ones.
func placeholderMismatches(cats map[Lang]catalog) []string {
	var out []string
	for _, l := range Languages()[1:] {
		for k, en := range cats[English] {
			tr, ok := cats[l][k]
			if ok && !slices.Equal(placeholders(en), placeholders(tr)) {
				out = append(out, string(l)+": "+k)
			}
		}
	}
	sort.Strings(out)
	return out
}

// pluralGaps lists "lang: key" where a plural key (English sets one) lacks a
// form, or where a language makes a plain English key plural.
func pluralGaps(cats map[Lang]catalog) []string {
	var out []string
	for _, l := range Languages() {
		for k, en := range cats[English] {
			tr, ok := cats[l][k]
			if !ok {
				continue
			}
			plural := en.one != ""
			if plural && (tr.one == "" || tr.other == "") || !plural && tr.one != "" {
				out = append(out, string(l)+": "+k)
			}
		}
	}
	sort.Strings(out)
	return out
}

// lookups returns every string literal passed as the first argument of a .T
// or .N call in the non-test Go files under root, with one position each.
// Keys are literals by rule, so the scan can see them.
func lookups(t *testing.T, root string) map[string]string {
	t.Helper()
	found := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "T" && sel.Sel.Name != "N") {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if key, err := strconv.Unquote(lit.Value); err == nil {
				if _, seen := found[key]; !seen {
					rel, _ := filepath.Rel(root, fset.Position(lit.Pos()).Filename)
					found[key] = filepath.ToSlash(rel) + ":" + strconv.Itoa(fset.Position(lit.Pos()).Line)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// moduleRoot is the directory holding go.mod, above this package.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the i18n package")
		}
		dir = parent
	}
}

var broken = map[Lang]catalog{
	English: {
		"a":      {other: "path {path}"},
		"b":      {other: "{count} things"},
		"plural": {one: "{count} file", other: "{count} files"},
	},
	Spanish: {
		"b":      {other: "{cuenta} cosas"},
		"plural": {other: "{count} archivos"},
		"extra":  {other: "sobra"},
	},
}

func TestEveryKeyExistsInBothLanguages(t *testing.T) {
	if got, want := missingKeys(broken), []string{"es: a", "es: extra"}; !reflect.DeepEqual(got, want) {
		t.Errorf("planted catalog: missingKeys = %q, want %q", got, want)
	}
	if got := missingKeys(catalogs); len(got) > 0 {
		t.Errorf("keys missing from a language:\n  %s", strings.Join(got, "\n  "))
	}
}

func TestPlaceholdersMatchAcrossLanguages(t *testing.T) {
	if got, want := placeholderMismatches(broken), []string{"es: b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("planted catalog: placeholderMismatches = %q, want %q", got, want)
	}
	if got := placeholderMismatches(catalogs); len(got) > 0 {
		t.Errorf("placeholders differ from English:\n  %s", strings.Join(got, "\n  "))
	}
}

func TestPluralKeysHaveBothForms(t *testing.T) {
	if got, want := pluralGaps(broken), []string{"es: plural"}; !reflect.DeepEqual(got, want) {
		t.Errorf("planted catalog: pluralGaps = %q, want %q", got, want)
	}
	if got := pluralGaps(catalogs); len(got) > 0 {
		t.Errorf("plural keys without both forms:\n  %s", strings.Join(got, "\n  "))
	}
}

func TestLookupScanFindsTAndNCalls(t *testing.T) {
	dir := t.TempDir()
	src := `package x

func f(tr interface {
	T(string, map[string]any) string
	N(string, int, map[string]any) string
}) {
	_ = tr.T("scan.plain", nil)
	_ = tr.N("scan.plural", 2, nil)
	_ = tr.T(dynamic, nil)
}

var dynamic = "not.a.literal"
`
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	test := `package x

func g(tr interface{ T(string, map[string]any) string }) { _ = tr.T("scan.test.only", nil) }
`
	if err := os.WriteFile(filepath.Join(dir, "x_test.go"), []byte(test), 0o644); err != nil {
		t.Fatal(err)
	}
	got := lookups(t, dir)
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if want := []string{"scan.plain", "scan.plural"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("lookups = %q, want %q (literals only, test files skipped)", keys, want)
	}
}

// A key no Go code looks up is dead weight a translator still has to keep;
// a key looked up but never defined shows its raw name to the user.
func TestNoUnusedOrUndefinedKeys(t *testing.T) {
	used := lookups(t, moduleRoot(t))
	var unused, undefined []string
	for k := range catalogs[English] {
		if _, ok := used[k]; !ok {
			unused = append(unused, k)
		}
	}
	for k, pos := range used {
		if _, ok := catalogs[English][k]; !ok {
			undefined = append(undefined, pos+": "+k)
		}
	}
	sort.Strings(unused)
	sort.Strings(undefined)
	if len(unused) > 0 {
		t.Errorf("keys defined but never passed to T or N:\n  %s", strings.Join(unused, "\n  "))
	}
	if len(undefined) > 0 {
		t.Errorf("keys looked up but not in the English catalog:\n  %s", strings.Join(undefined, "\n  "))
	}
}

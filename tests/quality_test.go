package tests

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// Size and complexity limits, enforced in go test with a ratchet.
//
// Every non-test, non-generated .go file under cmd/, internal/ and tools/ is
// measured: lines per file, lines per function, and cyclomatic complexity per
// function (1, plus one for each if, for, range, non-default case or select
// clause, && and ||). Code over a limit is allowed only with an entry in
// tests/testdata/quality-allowlist.txt, whose value is a ceiling: the code may
// not grow past it, and when it shrinks the entry must come down with it. A
// new entry is a reviewed change to that file.
//
//	go test ./tests/ -run Quality -v        # also logs the ten worst offenders
//	go test ./tests/ -run TestQualityLimitsHold -update   # rewrites the list from the code

const (
	maxFileLines      = 500
	maxFuncLines      = 60
	maxFuncComplexity = 15
)

const qualityAllowlist = "tests/testdata/quality-allowlist.txt"

// qualityMetric is one measured value. key is "path" for a file and
// "path|symbol" for a function.
type qualityMetric struct {
	kind  string // "file-lines", "func-lines" or "complexity"
	key   string
	value int
}

func (m qualityMetric) id() string { return m.kind + "|" + m.key }

var qualityLimits = map[string]int{
	"file-lines": maxFileLines,
	"func-lines": maxFuncLines,
	"complexity": maxFuncComplexity,
}

// complexity is the cyclomatic complexity of one function body.
func complexity(fn *ast.FuncDecl) int {
	n := 1
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch x := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			n++
		case *ast.CaseClause:
			if x.List != nil {
				n++
			}
		case *ast.CommClause:
			if x.Comm != nil {
				n++
			}
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				n++
			}
		}
		return true
	})
	return n
}

// funcSymbol names a function: Recv.Name for a method, Name otherwise.
func funcSymbol(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	t := fn.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	if g, ok := t.(*ast.IndexExpr); ok {
		t = g.X
	}
	if g, ok := t.(*ast.IndexListExpr); ok {
		t = g.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// measureQuality measures every Go file of the tree.
func measureQuality(t *testing.T, fsys fs.FS) []qualityMetric {
	t.Helper()
	var out []qualityMetric
	fset := token.NewFileSet()
	for _, top := range []string{"cmd", "internal", "tools"} {
		if _, err := fs.Stat(fsys, top); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		err := fs.WalkDir(fsys, top, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			src, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			f, err := parser.ParseFile(fset, p, src, parser.ParseComments)
			if err != nil {
				return err
			}
			if ast.IsGenerated(f) {
				return nil
			}
			lines := bytes.Count(src, []byte("\n"))
			if len(src) > 0 && src[len(src)-1] != '\n' {
				lines++
			}
			out = append(out, qualityMetric{"file-lines", p, lines})
			seen := map[string]int{}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				sym := funcSymbol(fn)
				if seen[sym]++; seen[sym] > 1 {
					sym += "#" + strconv.Itoa(seen[sym])
				}
				key := p + "|" + sym
				n := fset.Position(fn.End()).Line - fset.Position(fn.Pos()).Line + 1
				out = append(out, qualityMetric{"func-lines", key, n}, qualityMetric{"complexity", key, complexity(fn)})
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// parseAllowlist reads "kind|key|ceiling" lines; # starts a comment.
func parseAllowlist(src string) (map[string]int, error) {
	out := map[string]int{}
	for i, ln := range strings.Split(src, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		cut := strings.LastIndex(ln, "|")
		n, err := strconv.Atoi(ln[cut+1:])
		if cut < 0 || err != nil {
			return nil, fmt.Errorf("line %d: want kind|path[|symbol]|ceiling, got %q", i+1, ln)
		}
		out[ln[:cut]] = n
	}
	return out, nil
}

// ratchet compares measurements with the limits and the allow-list.
func ratchet(metrics []qualityMetric, allow map[string]int) []string {
	var out []string
	measured := map[string]bool{}
	for _, m := range metrics {
		id, limit := m.id(), qualityLimits[m.kind]
		measured[id] = true
		ceiling, listed := allow[id]
		switch {
		case !listed && m.value > limit:
			out = append(out, fmt.Sprintf("%s is %d, over the limit of %d: split it, or add %q to %s in a reviewed change",
				id, m.value, limit, id+"|"+strconv.Itoa(m.value), qualityAllowlist))
		case listed && m.value > ceiling:
			out = append(out, fmt.Sprintf("%s is %d, past its ceiling of %d: an exception may not grow", id, m.value, ceiling))
		case listed && m.value <= limit:
			out = append(out, fmt.Sprintf("%s is within the limit now (%d <= %d): delete its entry from %s", id, m.value, limit, qualityAllowlist))
		case listed && m.value < ceiling:
			out = append(out, fmt.Sprintf("%s dropped to %d: lower its ceiling from %d in %s", id, m.value, ceiling, qualityAllowlist))
		}
	}
	for id := range allow {
		if !measured[id] {
			out = append(out, fmt.Sprintf("%s no longer exists: delete its entry from %s", id, qualityAllowlist))
		}
	}
	sort.Strings(out)
	return out
}

// renderAllowlist writes the allow-list for the current offenders.
func renderAllowlist(metrics []qualityMetric) string {
	var lines []string
	for _, m := range metrics {
		if m.value > qualityLimits[m.kind] {
			lines = append(lines, m.id()+"|"+strconv.Itoa(m.value))
		}
	}
	sort.Strings(lines)
	return "# Size and complexity exceptions: kind|path[|symbol]|ceiling. See tests/quality_test.go.\n" +
		"# A ceiling may only come down. Delete an entry once its code is within the limit.\n" +
		strings.Join(lines, "\n") + "\n"
}

func TestQualityCyclomaticComplexityOfAKnownFunction(t *testing.T) {
	src := `package x
func f(a, b int, s []int) int {
	if a > 0 && b > 0 { // if, &&
		return 1
	}
	for _, v := range s { // range
		if v == 0 || v == 1 { // if, ||
			continue
		}
	}
	switch a {
	case 1: // case
	case 2:  // case
	default: // not counted
	}
	return 0
}`
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := complexity(f.Decls[0].(*ast.FuncDecl)); got != 8 {
		t.Errorf("complexity = %d, want 8 (1 + if, &&, range, if, ||, case, case)", got)
	}
}

func TestQualityRatchetNamesEveryDrift(t *testing.T) {
	metrics := []qualityMetric{
		{"file-lines", "a.go", maxFileLines + 10},  // new offender
		{"func-lines", "b.go|F", maxFuncLines + 5}, // grew past its ceiling
		{"complexity", "c.go|G", maxFuncComplexity + 1},
		{"complexity", "d.go|H", 3}, // fixed, entry still listed
		{"func-lines", "e.go|K", maxFuncLines + 2},
	}
	allow := map[string]int{
		"func-lines|b.go|F":  maxFuncLines + 1,
		"complexity|c.go|G":  maxFuncComplexity + 4, // must come down
		"complexity|d.go|H":  maxFuncComplexity + 2,
		"func-lines|e.go|K":  maxFuncLines + 2, // exact: fine
		"file-lines|gone.go": maxFileLines + 1,
	}
	got := strings.Join(ratchet(metrics, allow), "\n")
	for _, want := range []string{
		"file-lines|a.go is " + strconv.Itoa(maxFileLines+10),
		"func-lines|b.go|F is " + strconv.Itoa(maxFuncLines+5),
		"complexity|c.go|G dropped",
		"complexity|d.go|H is within",
		"file-lines|gone.go no longer exists",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ratchet does not report %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "e.go|K") {
		t.Errorf("an offender at its exact ceiling is fine:\n%s", got)
	}
}

func TestQualityMeasuresFilesAndSkipsTestsAndGenerated(t *testing.T) {
	fsys := fstest.MapFS{
		"internal/a/a.go":      {Data: []byte("package a\n\nfunc (s *S) M() {\n}\n\nfunc init() {}\nfunc init() {}\n")},
		"internal/a/a_test.go": {Data: []byte("package a\nfunc T() {}\n")},
		"internal/a/gen.go":    {Data: []byte("// Code generated by x. DO NOT EDIT.\n\npackage a\nfunc G() {}\n")},
	}
	got := map[string]int{}
	for _, m := range measureQuality(t, fsys) {
		got[m.id()] = m.value
	}
	want := map[string]int{
		"file-lines|internal/a/a.go": 7, "func-lines|internal/a/a.go|S.M": 2, "complexity|internal/a/a.go|S.M": 1,
		"func-lines|internal/a/a.go|init": 1, "func-lines|internal/a/a.go|init#2": 1,
		"complexity|internal/a/a.go|init": 1, "complexity|internal/a/a.go|init#2": 1,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %d, want %d", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("measured %d values, want %d (tests and generated files are skipped): %v", len(got), len(want), got)
	}
}

func TestQualityLimitsHold(t *testing.T) {
	metrics := measureQuality(t, os.DirFS(repo))
	if *archUpdate {
		write(t, p(qualityAllowlist), renderAllowlist(metrics))
		return
	}
	allow, err := parseAllowlist(read(t, qualityAllowlist))
	if err != nil {
		t.Fatalf("%s: %v", qualityAllowlist, err)
	}
	for _, problem := range ratchet(metrics, allow) {
		t.Error(problem)
	}
	if testing.Verbose() {
		for _, kind := range []string{"file-lines", "func-lines", "complexity"} {
			var worst []qualityMetric
			for _, m := range metrics {
				if m.kind == kind {
					worst = append(worst, m)
				}
			}
			sort.Slice(worst, func(i, j int) bool { return worst[i].value > worst[j].value })
			for i := 0; i < len(worst) && i < 10; i++ {
				t.Logf("%s #%d: %s = %d", kind, i+1, worst[i].key, worst[i].value)
			}
		}
	}
}

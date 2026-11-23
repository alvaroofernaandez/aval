package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Test hygiene: scratch space goes through t.TempDir() and waiting goes through
// a deadline-bound poll or an injected clock, never a hand-made temp dir or a
// fixed sleep. A fixed sleep is either too short (flaky) or too long (slow), and
// a hand-made temp dir leaks when the test dies.
//
// A rule is an AST match on a call, so comments and strings never trigger it.

// hygieneRule names one forbidden call. inTestMain says whether the call is
// still allowed inside TestMain, where there is no *testing.T to ask for a dir.
type hygieneRule struct {
	pkg, fn    string
	advice     string
	inTestMain bool
}

var hygieneRules = []hygieneRule{
	{"os", "MkdirTemp", "use t.TempDir()", true},
	{"ioutil", "TempDir", "use t.TempDir()", true},
	{"time", "Sleep", "poll with a deadline, set mtimes with os.Chtimes, or inject a clock", false},
}

// hygieneExempt maps a repo-relative file to the reason it may still offend.
// An entry that no longer offends fails the test: delete it.
var hygieneExempt = map[string]string{
	"internal/term/tmux/e2e/helpers_test.go": "the poll tick of waitUntil, waitFor and stable: the one deadline-bound " +
		"polling primitive every real-tmux wait goes through, since a real tmux server offers no event to wait on",
}

type hygieneHit struct {
	file string
	line int
	call string
	msg  string
}

// hygieneHits parses every _test.go under root and returns each offence.
func hygieneHits(t *testing.T, root string) []hygieneHit {
	t.Helper()
	var hits []hygieneHit
	skip := map[string]bool{".git": true, "node_modules": true, "vendor": true, ".worktrees": true}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Errorf("%s: cannot parse: %v", path, perr)
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			isMain := ok && fd.Recv == nil && fd.Name.Name == "TestMain"
			ast.Inspect(decl, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				id, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				for _, r := range hygieneRules {
					if id.Name == r.pkg && sel.Sel.Name == r.fn && !(isMain && r.inTestMain) {
						hits = append(hits, hygieneHit{rel, fset.Position(call.Pos()).Line, r.pkg + "." + r.fn, r.advice})
					}
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func TestHygieneNoHandMadeTempDirsOrFixedSleeps(t *testing.T) {
	hits := hygieneHits(t, repo)
	offending := map[string]bool{}
	for _, h := range hits {
		offending[h.file] = true
		if _, ok := hygieneExempt[h.file]; ok {
			continue
		}
		t.Errorf("%s:%d: %s: %s (or exempt the file in hygieneExempt with a reason)", h.file, h.line, h.call, h.msg)
	}
	files := make([]string, 0, len(hygieneExempt))
	for f := range hygieneExempt {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		if strings.TrimSpace(hygieneExempt[f]) == "" {
			t.Errorf("%s: exemption has no reason", f)
		}
		if !offending[f] {
			t.Errorf("%s: stale exemption, the file no longer offends; remove it from hygieneExempt", f)
		}
	}
}

func TestHygieneRulesCatchWhatTheyClaim(t *testing.T) {
	dir := t.TempDir()
	write(t, dir+"/a_test.go", `package a
import ("os"; "time"; "io/ioutil")
func TestMain(m interface{}) { os.MkdirTemp("", "x") }
func TestA() {
	os.MkdirTemp("", "x")
	ioutil.TempDir("", "x")
	time.Sleep(1)
	_ = "time.Sleep(1) in a string is fine" // time.Sleep(1) in a comment too
}
`)
	var got []string
	for _, h := range hygieneHits(t, dir) {
		got = append(got, h.call)
	}
	eq(t, "TestMain may MkdirTemp; everything else is caught", "os.MkdirTemp,ioutil.TempDir,time.Sleep", strings.Join(got, ","))
}

// Errors are asserted as values: errors.Is for a sentinel, errors.As for a
// typed error. Comparing err.Error() text is a change detector: it fails on a
// reworded message and passes on the wrong error with the right words. The text
// a person reads is pinned once, in tests/messages_test.go.
//
// An offence is an X.Error() call compared with == or !=, switched on, passed to
// a text matcher (strings.Contains and kin, a regexp Match, eq, mustContain) or
// stored on its own in a variable, which only happens to compare it later.
// Building a diagnostic ("<" + err.Error() + ">", t.Errorf("%v", err)) is fine.

// errTextPins is the one file allowed to pin user-facing text.
const errTextPins = "tests/messages_test.go"

// errTextExempt maps a repo-relative file to the reason it may still compare
// error text. An entry that no longer offends fails the test: delete it.
var errTextExempt = map[string]string{}

// errTextMatchers are the calls whose arguments are compared as text.
var errTextMatchers = map[string]bool{
	"Contains": true, "ContainsAny": true, "HasPrefix": true, "HasSuffix": true, "EqualFold": true,
	"Index": true, "Count": true, "Compare": true, "Match": true, "MatchString": true,
	"eq": true, "mustContain": true, "mustHave": true, "mustNotHave": true,
}

// isErrorText reports whether n is an X.Error() call.
func isErrorText(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Error"
}

// errorTextIn returns the X.Error() calls anywhere inside n.
func errorTextIn(n ast.Node) []ast.Node {
	var out []ast.Node
	ast.Inspect(n, func(c ast.Node) bool {
		if isErrorText(c) {
			out = append(out, c)
		}
		return true
	})
	return out
}

// errTextHits parses every _test.go under root and returns each comparison of
// error text, one per X.Error() call.
func errTextHits(t *testing.T, root string) []hygieneHit {
	t.Helper()
	var hits []hygieneHit
	skip := map[string]bool{".git": true, "node_modules": true, "vendor": true, ".worktrees": true}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel == errTextPins {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Errorf("%s: cannot parse: %v", path, perr)
			return nil
		}
		seen := map[token.Pos]bool{}
		flag := func(nodes []ast.Node, how string) {
			for _, c := range nodes {
				if !seen[c.Pos()] {
					seen[c.Pos()] = true
					hits = append(hits, hygieneHit{rel, fset.Position(c.Pos()).Line, how, "assert with errors.Is or errors.As; pin user-facing text in " + errTextPins})
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BinaryExpr:
				if x.Op == token.EQL || x.Op == token.NEQ {
					flag(errorTextIn(x), "err.Error() "+x.Op.String())
				}
			case *ast.SwitchStmt:
				if x.Tag != nil {
					flag(errorTextIn(x.Tag), "switch err.Error()")
				}
			case *ast.CallExpr:
				name := ""
				switch fn := x.Fun.(type) {
				case *ast.SelectorExpr:
					name = fn.Sel.Name
				case *ast.Ident:
					name = fn.Name
				}
				if errTextMatchers[name] {
					for _, a := range x.Args {
						flag(errorTextIn(a), name+"(err.Error())")
					}
				}
			case *ast.AssignStmt:
				for _, r := range x.Rhs {
					if isErrorText(r) {
						flag([]ast.Node{r}, "v := err.Error()")
					}
				}
			case *ast.ValueSpec:
				for _, v := range x.Values {
					if isErrorText(v) {
						flag([]ast.Node{v}, "var v = err.Error()")
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].file != hits[j].file {
			return hits[i].file < hits[j].file
		}
		return hits[i].line < hits[j].line
	})
	return hits
}

func TestHygieneNoErrorTextComparisons(t *testing.T) {
	offending := map[string]bool{}
	for _, h := range errTextHits(t, repo) {
		offending[h.file] = true
		if _, ok := errTextExempt[h.file]; ok {
			continue
		}
		t.Errorf("%s:%d: %s: %s (or exempt the file in errTextExempt with a reason)", h.file, h.line, h.call, h.msg)
	}
	files := make([]string, 0, len(errTextExempt))
	for f := range errTextExempt {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		if strings.TrimSpace(errTextExempt[f]) == "" {
			t.Errorf("%s: exemption has no reason", f)
		}
		if !offending[f] {
			t.Errorf("%s: stale exemption, the file no longer offends; remove it from errTextExempt", f)
		}
	}
}

func TestHygieneErrorTextRuleCatchesWhatItClaims(t *testing.T) {
	dir := t.TempDir()
	write(t, dir+"/a_test.go", `package a
import ("errors"; "regexp"; "strings"; "testing")
func TestA(t *testing.T) {
	err := errors.New("x")
	_ = err.Error() == "x"
	_ = strings.Contains(err.Error(), "x")
	_ = regexp.MustCompile("x").MatchString(err.Error())
	_ = strings.Contains(out+err.Error(), "x")
	msg := err.Error()
	switch err.Error() {
	}
	eq(t, "msg", "x", err.Error())
	_ = "<" + err.Error() + ">"
	t.Errorf("err = %v", err)
	_ = "err.Error() == x in a string is fine" // err.Error() == "x" in a comment too
	_ = errors.Is(err, err)
	_ = msg
}
`)
	write(t, dir+"/tests/messages_test.go", `package tests
func TestM() { _ = err.Error() == "pinned here, on purpose" }
`)
	var got []string
	for _, h := range errTextHits(t, dir) {
		got = append(got, h.file+":"+strings.Fields(h.call)[0])
	}
	eq(t, "every comparison is caught, nothing else, and messages_test.go is the allowed place",
		"a_test.go:err.Error(),a_test.go:Contains(err.Error()),a_test.go:MatchString(err.Error()),a_test.go:Contains(err.Error()),"+
			"a_test.go:v,a_test.go:switch,a_test.go:eq(err.Error())",
		strings.Join(got, ","))
}

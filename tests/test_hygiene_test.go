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

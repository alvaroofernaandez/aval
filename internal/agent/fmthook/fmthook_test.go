package fmthook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"go/format"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// recorder is a fake Runner that remembers every argv and answers with err.
type recorder struct {
	calls [][]string
	err   error
}

func (r *recorder) run(_ context.Context, _ string, name string, args ...string) error {
	r.calls = append(r.calls, append([]string{filepath.Base(name)}, args...))
	return r.err
}

// repo is a scratch repository: a directory with a .git directory.
func repo(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mkfile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	return root
}

func mkfile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func payload(file string) *bytes.Reader {
	b, _ := json.Marshal(map[string]any{
		"hook_event_name": "PostToolUse", "tool_name": "Edit", "cwd": filepath.Dir(file),
		"tool_input": map[string]string{"file_path": file},
	})
	return bytes.NewReader(b)
}

// fakeOpts finds tools only in onPath, never treats a file as ignored and
// records every run.
func fakeOpts(r *recorder, onPath ...string) (Options, *int) {
	ignoreCalls := 0
	return Options{
		Run: r.run,
		LookPath: func(name string) (string, error) {
			for _, p := range onPath {
				if p == name {
					return "/usr/bin/" + name, nil
				}
			}
			return "", exec.ErrNotFound
		},
		Ignored: func(string, string) bool { ignoreCalls++; return false },
		Timeout: time.Second,
		GOOS:    "linux",
	}, &ignoreCalls
}

const uglyGo = "package x\nfunc  F( ) {\nreturn\n}\n"

func TestRunFormatsAGoFileLikeGofmt(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not installed")
	}
	root := repo(t)
	file := filepath.Join(root, "x.go")
	mkfile(t, file, uglyGo)
	want, err := format.Source([]byte(uglyGo))
	if err != nil {
		t.Fatal(err)
	}
	var warn bytes.Buffer
	Run(payload(file), &warn, DefaultOptions())
	got, _ := os.ReadFile(file)
	if string(got) != string(want) {
		t.Errorf("the file is not gofmt'd:\n got %q\nwant %q (warn %q)", got, want, warn.String())
	}
}

func TestRunPicksBiomeThenPrettier(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  string
	}{
		{"biome with its binary", []string{"biome.json", "node_modules/.bin/biome", ".prettierrc", "node_modules/.bin/prettier"}, "biome format --write"},
		{"prettier with its binary", []string{".prettierrc", "node_modules/.bin/prettier"}, "prettier --write"},
		{"biome config without its binary", []string{"biome.json"}, ""},
		{"prettier config without its binary", []string{".prettierrc.json"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := repo(t)
			for _, f := range c.files {
				mkfile(t, filepath.Join(root, filepath.FromSlash(f)), "{}")
			}
			file := filepath.Join(root, "src", "app.tsx")
			mkfile(t, file, "export const a=1\n")
			r := &recorder{}
			o, _ := fakeOpts(r)
			Run(payload(file), &bytes.Buffer{}, o)
			got := ""
			if len(r.calls) == 1 {
				got = strings.Join(r.calls[0][:len(r.calls[0])-1], " ")
				if last := r.calls[0][len(r.calls[0])-1]; last != file {
					t.Errorf("formatted %s, want %s", last, file)
				}
			} else if len(r.calls) > 1 {
				t.Fatalf("ran %d formatters, want at most one: %v", len(r.calls), r.calls)
			}
			if got != c.want {
				t.Errorf("ran %q, want %q", got, c.want)
			}
		})
	}
}

func TestRunPythonNeedsRuffAndAProjectFile(t *testing.T) {
	cases := []struct {
		name    string
		project string
		ruff    bool
		ran     bool
	}{
		{"ruff and pyproject.toml", "pyproject.toml", true, true},
		{"ruff and ruff.toml", "ruff.toml", true, true},
		{"ruff without a project file", "", true, false},
		{"a project file without ruff", "pyproject.toml", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := repo(t)
			if c.project != "" {
				mkfile(t, filepath.Join(root, c.project), "")
			}
			file := filepath.Join(root, "pkg", "m.py")
			mkfile(t, file, "x=1\n")
			r := &recorder{}
			var onPath []string
			if c.ruff {
				onPath = []string{"ruff"}
			}
			o, _ := fakeOpts(r, onPath...)
			Run(payload(file), &bytes.Buffer{}, o)
			if ran := len(r.calls) == 1 && r.calls[0][0] == "ruff" && r.calls[0][1] == "format"; ran != c.ran {
				t.Errorf("ruff ran: %v, want %v (%v)", ran, c.ran, r.calls)
			}
		})
	}
}

func treeHash(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		h.Write([]byte(rel))
		h.Write(b)
		return nil
	})
	return string(h.Sum(nil))
}

func TestRunTouchesOnlyTheEditedFile(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not installed")
	}
	root := repo(t)
	file := filepath.Join(root, "a.go")
	mkfile(t, file, uglyGo)
	mkfile(t, filepath.Join(root, "b.go"), uglyGo)
	mkfile(t, filepath.Join(root, "sub", "c.go"), uglyGo)
	Run(payload(file), &bytes.Buffer{}, DefaultOptions())
	mkfile(t, file, uglyGo) // restore the edited file: the rest must be untouched
	before := treeHash(t, root)
	Run(payload(filepath.Join(root, "nothing.txt")), &bytes.Buffer{}, DefaultOptions())
	if got, _ := os.ReadFile(filepath.Join(root, "b.go")); string(got) != uglyGo {
		t.Error("a file that was not edited was formatted")
	}
	if after := treeHash(t, root); after != before {
		t.Error("the tree changed outside the edited file")
	}
}

func TestRunWithoutAFormatterIsSilentAndStartsNothing(t *testing.T) {
	root := repo(t)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{
		filepath.Join(root, "notes.txt"),
		filepath.Join(root, "data.unknownext"),
		filepath.Join(root, "Makefile"),
		filepath.Join(outside, "x.go"),
		filepath.Join(outside, "app.tsx"),
	} {
		mkfile(t, file, "x\n")
		r := &recorder{}
		o, ignoreCalls := fakeOpts(r, "gofmt", "ruff")
		var warn bytes.Buffer
		Run(payload(file), &warn, o)
		if len(r.calls) != 0 || *ignoreCalls != 0 || warn.Len() != 0 {
			t.Errorf("%s: runs %v, ignore checks %d, warn %q; want nothing at all", file, r.calls, *ignoreCalls, warn.String())
		}
	}
	for _, in := range []string{"", "not json", `{"tool_input":7}`, `{"tool_input":{"file_path":""}}`} {
		r := &recorder{}
		o, ignoreCalls := fakeOpts(r, "gofmt")
		var warn bytes.Buffer
		Run(strings.NewReader(in), &warn, o)
		if len(r.calls) != 0 || *ignoreCalls != 0 || warn.Len() != 0 {
			t.Errorf("payload %q: runs %v, ignore checks %d, warn %q", in, r.calls, *ignoreCalls, warn.String())
		}
	}
}

func TestRunFormatterFailureWarnsInOneLine(t *testing.T) {
	for _, fail := range []error{errors.New("exit status 2\nsecond line"), context.DeadlineExceeded} {
		root := repo(t)
		file := filepath.Join(root, "x.go")
		mkfile(t, file, uglyGo)
		r := &recorder{err: fail}
		o, _ := fakeOpts(r, "gofmt")
		var warn bytes.Buffer
		Run(payload(file), &warn, o)
		if lines := strings.Count(warn.String(), "\n"); lines != 1 || !strings.HasSuffix(warn.String(), "\n") {
			t.Errorf("%v: want exactly one warning line, got %q", fail, warn.String())
		}
		if got, _ := os.ReadFile(file); string(got) != uglyGo {
			t.Errorf("%v: the file changed", fail)
		}
	}
}

func TestRunSkipsBinaryAndIgnoredFiles(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		ignored bool
	}{
		{"a binary file", "package x\x00\x01\x02", false},
		{"an ignored file", uglyGo, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := repo(t)
			file := filepath.Join(root, "gen.go")
			mkfile(t, file, c.body)
			r := &recorder{}
			o, _ := fakeOpts(r, "gofmt")
			o.Ignored = func(string, string) bool { return c.ignored }
			Run(payload(file), &bytes.Buffer{}, o)
			if len(r.calls) != 0 {
				t.Errorf("ran %v on %s", r.calls, c.name)
			}
		})
	}
}

func TestRunUsesTheWindowsShims(t *testing.T) {
	root := repo(t)
	mkfile(t, filepath.Join(root, ".prettierrc"), "{}")
	mkfile(t, filepath.Join(root, "node_modules", ".bin", "prettier.cmd"), "")
	file := filepath.Join(root, "a.css")
	mkfile(t, file, "a{}\n")
	r := &recorder{}
	o, _ := fakeOpts(r)
	o.GOOS = "windows"
	Run(payload(file), &bytes.Buffer{}, o)
	if len(r.calls) != 1 || r.calls[0][0] != "prettier.cmd" {
		t.Errorf("want prettier.cmd on Windows, ran %v", r.calls)
	}
}

func benchRepo(b *testing.B, name, body string) string {
	b.Helper()
	root, err := filepath.EvalSymlinks(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	for p, c := range map[string]string{filepath.Join(root, ".git", "HEAD"): "x", filepath.Join(root, name): body} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	return filepath.Join(root, name)
}

// BenchmarkRunNoFormatter is the hot path: an edit nobody formats.
func BenchmarkRunNoFormatter(b *testing.B) {
	file := benchRepo(b, "notes.txt", "x\n")
	in, _ := json.Marshal(map[string]any{"tool_input": map[string]string{"file_path": file}})
	o := DefaultOptions()
	for b.Loop() {
		Run(bytes.NewReader(in), io.Discard, o)
	}
}

// BenchmarkRunGofmt formats one Go file with the real gofmt and git.
func BenchmarkRunGofmt(b *testing.B) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		b.Skip("gofmt not installed")
	}
	file := benchRepo(b, "x.go", uglyGo)
	in, _ := json.Marshal(map[string]any{"tool_input": map[string]string{"file_path": file}})
	o := DefaultOptions()
	for b.Loop() {
		Run(bytes.NewReader(in), io.Discard, o)
	}
}

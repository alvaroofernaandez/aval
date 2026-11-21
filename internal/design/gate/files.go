package gate

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// walk lists the regular files under the given paths whose extension passes
// ok (a nil ok accepts every file named explicitly). Directories are walked in
// lexical order and node_modules is skipped. A file passed by name is kept
// when its extension passes ok.
func walk(paths []string, ok func(ext string) bool) []string {
	var out []string
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !st.IsDir() {
			if ok == nil || ok(filepath.Ext(p)) {
				out = append(out, p)
			}
			continue
		}
		_ = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if ok == nil || ok(filepath.Ext(path)) {
				out = append(out, path)
			}
			return nil
		})
	}
	return out
}

func extSet(exts ...string) func(string) bool {
	m := map[string]bool{}
	for _, e := range exts {
		m[e] = true
	}
	return func(e string) bool { return m[e] }
}

// missingPaths returns the paths that do not exist.
func missingPaths(paths []string) []string {
	var out []string
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			out = append(out, p)
		}
	}
	return out
}

// readText reads a file that must be valid UTF-8.
func readText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", errNotUTF8
	}
	return string(b), nil
}

type utf8Err struct{}

func (utf8Err) Error() string { return "not valid UTF-8" }

var errNotUTF8 = utf8Err{}

func lines(s string) []string {
	ls := strings.Split(s, "\n")
	for i, l := range ls {
		ls[i] = strings.TrimSuffix(l, "\r")
	}
	return ls
}

func isWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// prevRune is the rune before byte offset i, or 0 at the start.
func prevRune(s string, i int) rune {
	if i == 0 {
		return 0
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return r
}

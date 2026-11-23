// Package config owns the aval config file: ${XDG_CONFIG_HOME:-~/.config}/aval/config.
//
// The format is `key = value` lines with `#` comments. Reads are tolerant
// (unknown keys and bad UI values never fail a command); writes are strict and
// preserve comments, unknown keys and line order.
//
// Precedence for every key: flag > env > file > default.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Themes lists every accepted value of the theme key. The UI theme registry
// is checked against it.
var Themes = []string{
	"aval", "aval-dark", "aval-light", "terminal", "mono",
	"catppuccin", "catppuccin-mocha", "catppuccin-latte", "catppuccin-frappe", "catppuccin-macchiato",
	"tokyo-night", "tokyo-night-storm", "tokyo-night-day",
	"gruvbox", "gruvbox-dark", "gruvbox-light",
	"dracula", "dracula-alucard", "nord",
	"kanagawa", "kanagawa-wave", "kanagawa-dragon", "kanagawa-lotus",
	"rose-pine", "rose-pine-moon", "rose-pine-dawn",
	"solarized", "solarized-dark", "solarized-light",
	"one-dark", "one-light",
	"everforest", "everforest-dark", "everforest-light",
}

var keys = []string{"repo", "theme", "background", "glyphs", "motion", "update_check"}

var defaults = map[string]string{
	"repo":         "",
	"theme":        "aval",
	"background":   "auto",
	"glyphs":       "auto",
	"motion":       "auto",
	"update_check": "on",
}

var enums = map[string][]string{
	"background":   {"auto", "dark", "light"},
	"glyphs":       {"auto", "unicode", "ascii"},
	"motion":       {"auto", "off"},
	"update_check": {"on", "off"},
}

var envNames = map[string]string{
	"repo":       "AVAL_REPO",
	"theme":      "AVAL_THEME",
	"background": "AVAL_BACKGROUND",
	"glyphs":     "AVAL_GLYPHS",
	"motion":     "AVAL_MOTION",
}

// noUpdateCheckEnv is the legacy switch: any non-empty value means update_check=off.
const noUpdateCheckEnv = "AVAL_NO_UPDATE_CHECK"

const header = "# Written by aval. Override with flags or AVAL_* environment variables."

// Keys returns the known keys in display order.
func Keys() []string { return slices.Clone(keys) }

// Warning reports a bad value found while loading.
type Warning struct {
	Line    int
	Message string
}

func (w Warning) String() string { return w.Message }

// Source says where a resolved value came from.
type Source struct {
	kind string // flag, env, file, default
	Line int
}

func (s Source) String() string {
	if s.kind == "file" {
		return fmt.Sprintf("file:%d", s.Line)
	}
	if s.kind == "" {
		return "default"
	}
	return s.kind
}

type entry struct {
	value string
	line  int
}

// File is a parsed config file.
type File struct {
	path    string
	vals    map[string]entry
	unknown []string
}

// Path is where the config lives: ${XDG_CONFIG_HOME:-~/.config}/aval/config,
// or %APPDATA%\aval\config on Windows.
func Path(getenv func(string) string, home, goos string) string {
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "aval", "config")
	}
	if goos == "windows" {
		if a := getenv("APPDATA"); a != "" {
			return filepath.Join(a, "aval", "config")
		}
	}
	return filepath.Join(home, ".config", "aval", "config")
}

// Load parses the file. A missing file is an empty config, not an error.
// Invalid UI values yield warnings and are not stored, so defaults apply.
func Load(path string) (*File, []Warning, error) {
	f := &File{path: path, vals: map[string]entry{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var warns []Warning
	for i, text := range splitLines(string(b)) {
		k, v, ok := keyValue(text)
		if !ok {
			continue
		}
		if !slices.Contains(keys, k) {
			f.unknown = append(f.unknown, k)
			continue
		}
		if _, dup := f.vals[k]; dup {
			continue
		}
		if msg := validate(k, v); msg != "" {
			warns = append(warns, Warning{Line: i + 1, Message: fmt.Sprintf("config: %s:%d: %s", path, i+1, msg)})
			continue
		}
		f.vals[k] = entry{v, i + 1}
	}
	return f, warns, nil
}

// Get returns the file's valid value for key.
func (f *File) Get(key string) (string, bool) {
	e, ok := f.vals[key]
	return e.value, ok
}

// Unknown returns the keys the file holds that aval does not know.
func (f *File) Unknown() []string { return slices.Clone(f.unknown) }

// Resolve applies flag > env > file > default. Invalid flag or env values are
// skipped rather than trusted.
func (f *File) Resolve(key string, flags map[string]string, getenv func(string) string) (string, Source) {
	if v := flags[key]; v != "" && validate(key, v) == "" {
		return v, Source{kind: "flag"}
	}
	if key == "update_check" {
		if getenv(noUpdateCheckEnv) != "" {
			return "off", Source{kind: "env"}
		}
	} else if name := envNames[key]; name != "" {
		if v := getenv(name); v != "" && validate(key, v) == "" {
			return v, Source{kind: "env"}
		}
	}
	if e, ok := f.vals[key]; ok {
		return e.value, Source{kind: "file", Line: e.line}
	}
	return defaults[key], Source{kind: "default"}
}

// Set writes key = value atomically. It validates first and leaves the file
// untouched on error. Comments, unknown keys and order are preserved; an
// existing line is replaced in place, a new key is appended.
func Set(path, key, value string) error {
	if !slices.Contains(keys, key) {
		return fmt.Errorf("config: unknown key %q. Valid: %s", key, strings.Join(keys, ", "))
	}
	if msg := validate(key, value); msg != "" {
		return fmt.Errorf("config: %s: %s", path, msg)
	}
	var lines []string
	if b, err := os.ReadFile(path); err == nil {
		lines = splitLines(string(b))
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	out := make([]string, 0, len(lines)+2)
	done := false
	for _, l := range lines {
		if k, _, ok := keyValue(l); ok && k == key {
			if !done {
				out = append(out, key+" = "+value)
				done = true
			}
			continue
		}
		out = append(out, l)
	}
	if !done {
		if len(out) == 0 {
			out = append(out, header)
		}
		out = append(out, key+" = "+value)
	}
	return writeAtomic(path, strings.Join(out, "\n")+"\n")
}

func writeAtomic(path, text string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.WriteString(text)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr, os.Chmod(name, 0o644)); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func splitLines(text string) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func keyValue(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	k, v, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(k), strings.TrimSpace(v), true
}

// validate returns an error message ("" when valid). repo is never validated here.
func validate(key, value string) string {
	switch key {
	case "repo":
		return ""
	case "theme":
		if slices.Contains(Themes, value) {
			return ""
		}
		msg := fmt.Sprintf("theme = %q is not a theme. Valid: %s.", value, strings.Join(Themes, ", "))
		if s := closest(value, Themes); s != "" {
			msg += " Did you mean " + s + "?"
		}
		return msg
	}
	valid := enums[key]
	if slices.Contains(valid, value) {
		return ""
	}
	return fmt.Sprintf("%s = %q is not a valid %s. Valid: %s.", key, value, key, strings.Join(valid, ", "))
}

// closest returns the candidate nearest to in, or "" when none is close. A
// candidate also matches through its prefix, so "tokio" finds tokyo-night.
func closest(in string, cands []string) string {
	best, bestD := "", 3
	for _, c := range cands {
		d := lev(in, c)
		if len(c) > len(in) {
			d = min(d, lev(in, c[:len(in)]))
		}
		if d < bestD && d < len(in) {
			best, bestD = c, d
		}
	}
	return best
}

func lev(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

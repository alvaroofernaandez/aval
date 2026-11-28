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
	"regexp"
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

var keys = []string{"repo", "theme", "background", "glyphs", "motion", "update_check", "language"}

var defaults = map[string]string{
	"repo":         "",
	"theme":        "aval",
	"background":   "auto",
	"glyphs":       "auto",
	"motion":       "auto",
	"update_check": "on",
	"language":     "auto",
}

var enums = map[string][]string{
	"background":    {"auto", "dark", "light"},
	"glyphs":        {"auto", "unicode", "ascii"},
	"motion":        {"auto", "off"},
	"update_check":  {"on", "off"},
	"language":      {"auto", "en", "es"},
	"usage.billing": {"api", "subscription"},
}

var envNames = map[string]string{
	"repo":       "AVAL_REPO",
	"theme":      "AVAL_THEME",
	"background": "AVAL_BACKGROUND",
	"glyphs":     "AVAL_GLYPHS",
	"motion":     "AVAL_MOTION",
	"language":   "AVAL_LANG",
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
	// Invalid is the bad value as data, so a front end can render the
	// warning in the user's language.
	Invalid *InvalidValueError
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
	path      string
	vals      map[string]entry
	unknown   []string
	unknownAt []UnknownKey
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
		if !slices.Contains(keys, k) && !usageKey(k) {
			f.unknown = append(f.unknown, k)
			f.unknownAt = append(f.unknownAt, UnknownKey{Key: k, Line: i + 1})
			continue
		}
		if _, dup := f.vals[k]; dup {
			continue
		}
		if bad := invalid(k, v); bad != nil {
			warns = append(warns, Warning{Line: i + 1, Message: fmt.Sprintf("config: %s:%d: %s", path, i+1, bad.msg), Invalid: bad})
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
	if !slices.Contains(keys, key) && !usageKey(key) {
		return fmt.Errorf("config: unknown key %q. Valid: %s", key, strings.Join(keys, ", "))
	}
	if bad := invalid(key, value); bad != nil {
		return fmt.Errorf("config: %s: %w", path, bad)
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
	if bad := invalid(key, value); bad != nil {
		return bad.msg
	}
	return ""
}

// invalid returns the error for a bad value, or nil when value is valid for
// key. theme and language get a suggestion when one is close; the other enums
// are short enough to read at a glance.
func invalid(key, value string) *InvalidValueError {
	if key == "repo" {
		return nil
	}
	if strings.HasPrefix(key, "usage.budget.") {
		if budgetValue.MatchString(value) {
			return nil
		}
		e := &InvalidValueError{Key: key, Value: value}
		e.msg = fmt.Sprintf("%s = %q is not a budget. Use a token count, a share like 50%% or an amount like $4.5.", key, value)
		return e
	}
	valid, noun := enums[key], "a valid "+key
	if key == "theme" {
		valid, noun = Themes, "a theme"
	}
	if slices.Contains(valid, value) {
		return nil
	}
	e := &InvalidValueError{Key: key, Value: value, Valid: slices.Clone(valid)}
	if key == "theme" || key == "language" {
		e.Suggestion = closest(value, valid)
	}
	e.msg = fmt.Sprintf("%s = %q is not %s. Valid: %s.", key, value, noun, strings.Join(valid, ", "))
	if e.Suggestion != "" {
		e.msg += " Did you mean " + e.Suggestion + "?"
	}
	return e
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

// Validate returns an error message for an invalid value, or "" when valid.
func Validate(key, value string) string { return validate(key, value) }

// UnknownKey is a key aval does not know, with its 1-based line.
type UnknownKey struct {
	Key  string
	Line int
}

// UnknownKeys returns the unknown keys with their lines.
func (f *File) UnknownKeys() []UnknownKey { return slices.Clone(f.unknownAt) }

// EnvName is the environment variable that overrides key, or "".
func EnvName(key string) string {
	if key == "update_check" {
		return noUpdateCheckEnv
	}
	return envNames[key]
}

// InvalidValueError is a value Set refuses for Key. Its message says which
// values are valid, with a suggestion when one is close.
type InvalidValueError struct {
	Key, Value string
	// Valid lists the accepted values; Suggestion is the closest one, or "".
	Valid      []string
	Suggestion string
	msg        string
}

func (e *InvalidValueError) Error() string { return e.msg }

// Prefixed returns the file's valid values for the open key family that starts
// with prefix (the usage.* keys).
func (f *File) Prefixed(prefix string) map[string]string {
	out := map[string]string{}
	for k, e := range f.vals {
		if strings.HasPrefix(k, prefix) {
			out[k] = e.value
		}
	}
	return out
}

// Known reports whether key is a setting aval has: one of Keys() or a usage.*
// key.
func Known(key string) bool { return slices.Contains(keys, key) || usageKey(key) }

// usageKey reports whether key belongs to the open usage family: usage.billing
// and usage.budget.<consumer>.<window>. It is not in Keys(), which is the list
// the settings view and `aval config list` show.
func usageKey(key string) bool {
	if key == "usage.billing" {
		return true
	}
	rest, ok := strings.CutPrefix(key, "usage.budget.")
	if !ok {
		return false
	}
	consumer, window, ok := strings.Cut(rest, ".")
	return ok && slices.Contains(budgetConsumers, consumer) && slices.Contains(budgetWindows, window)
}

var (
	budgetConsumers = []string{"interactive", "daemon", "headless"}
	budgetWindows   = []string{"5h", "7d"}
	// A budget is a token count, a share of the window's tokens ("50%") or an
	// amount in dollars ("$4.5").
	budgetValue = regexp.MustCompile(`^(\d+|\d+(\.\d+)?%|\$\d+(\.\d+)?)$`)
)

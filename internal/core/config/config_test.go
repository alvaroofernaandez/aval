package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "aval", "config")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	f, warns, err := Load(filepath.Join(t.TempDir(), "absent"))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if v, ok := f.Get("repo"); ok || v != "" {
		t.Errorf("Get(repo) = %q, %v", v, ok)
	}
}

func TestRoundTripOnlyRepo(t *testing.T) {
	p := write(t, "repo = /x\n")
	f, warns, err := Load(p)
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if v, _ := f.Get("repo"); v != "/x" {
		t.Errorf("repo = %q", v)
	}
	if err := Set(p, "repo", "/x"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, p); got != "repo = /x\n" {
		t.Errorf("file = %q", got)
	}
}

func TestSetPreservesCommentsUnknownKeysAndOrder(t *testing.T) {
	p := write(t, "# mine\nfoo = bar\nrepo = /x\n\n# tail\ncolour = red\n")
	if err := Set(p, "theme", "nord"); err != nil {
		t.Fatal(err)
	}
	if err := Set(p, "repo", "/y"); err != nil {
		t.Fatal(err)
	}
	want := "# mine\nfoo = bar\nrepo = /y\n\n# tail\ncolour = red\ntheme = nord\n"
	if got := read(t, p); got != want {
		t.Errorf("file = %q\nwant   %q", got, want)
	}
	f, _, _ := Load(p)
	if u := strings.Join(f.Unknown(), ","); u != "foo,colour" {
		t.Errorf("Unknown = %q", u)
	}
}

func TestSetCreatesFileWithHeader(t *testing.T) {
	p := filepath.Join(t.TempDir(), "deep", "aval", "config")
	if err := Set(p, "repo", "/x"); err != nil {
		t.Fatal(err)
	}
	got := read(t, p)
	if !strings.HasPrefix(got, "# ") || !strings.HasSuffix(got, "repo = /x\n") {
		t.Errorf("file = %q", got)
	}
}

func TestSetDropsDuplicateKeyLines(t *testing.T) {
	p := write(t, "repo = /a\nx = 1\nrepo = /b\n")
	if err := Set(p, "repo", "/c"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, p); got != "repo = /c\nx = 1\n" {
		t.Errorf("file = %q", got)
	}
}

func TestInvalidValueMessages(t *testing.T) {
	list := strings.Join(Themes, ", ")
	cases := []struct {
		line, want string
	}{
		{`theme = tokio`, `theme = "tokio" is not a theme. Valid: ` + list + `. Did you mean tokyo-night?`},
		{`theme = zzzzzzzzzz`, `theme = "zzzzzzzzzz" is not a theme. Valid: ` + list + `.`},
		{`background = dim`, `background = "dim" is not a valid background. Valid: auto, dark, light.`},
		{`glyphs = emoji`, `glyphs = "emoji" is not a valid glyphs. Valid: auto, unicode, ascii.`},
		{`motion = on`, `motion = "on" is not a valid motion. Valid: auto, off.`},
		{`update_check = maybe`, `update_check = "maybe" is not a valid update_check. Valid: on, off.`},
		{`language = esp`, `language = "esp" is not a valid language. Valid: auto, en, es. Did you mean es?`},
		{`language = fr`, `language = "fr" is not a valid language. Valid: auto, en, es.`},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			p := write(t, "# c\n"+c.line+"\n")
			f, warns, err := Load(p)
			if err != nil {
				t.Fatal(err)
			}
			if len(warns) != 1 {
				t.Fatalf("warns = %v", warns)
			}
			want := "config: " + p + ":2: " + c.want
			if warns[0].Message != want {
				t.Errorf("message = %q\nwant      %q", warns[0].Message, want)
			}
			if warns[0].Line != 2 {
				t.Errorf("line = %d", warns[0].Line)
			}
			key, _, _ := strings.Cut(c.line, " ")
			if _, ok := f.Get(key); ok {
				t.Errorf("invalid %s must not be stored", key)
			}
			v, src := f.Resolve(key, nil, env(nil))
			if src.String() != "default" || v != defaults[key] {
				t.Errorf("Resolve = %q, %s", v, src)
			}
		})
	}
}

func TestSetInvalidLeavesFileUntouched(t *testing.T) {
	orig := "# keep\nrepo = /x\n"
	p := write(t, orig)
	err := Set(p, "theme", "tokio")
	var ie *InvalidValueError
	if !errors.As(err, &ie) || ie.Key != "theme" || ie.Value != "tokio" {
		t.Fatalf("err = %v, want an *InvalidValueError for theme = tokio", err)
	}
	if got := read(t, p); got != orig {
		t.Errorf("file changed: %q", got)
	}
	if err := Set(p, "nope", "1"); err == nil {
		t.Error("unknown key accepted by Set")
	}
}

func TestBadRepoPathIsNotValidated(t *testing.T) {
	p := write(t, "repo = /does/not/exist\n")
	_, warns, err := Load(p)
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
}

func TestResolveSources(t *testing.T) {
	p := write(t, "# c\ntheme = nord\nmotion = off\n")
	f, _, _ := Load(p)
	e := env(map[string]string{"AVAL_THEME": "dracula", "AVAL_GLYPHS": "ascii"})
	cases := []struct {
		key   string
		flags map[string]string
		want  string
		src   string
	}{
		{"theme", map[string]string{"theme": "mono"}, "mono", "flag"},
		{"theme", nil, "dracula", "env"},
		{"glyphs", nil, "ascii", "env"},
		{"motion", nil, "off", "file:3"},
		{"background", nil, "auto", "default"},
		{"update_check", nil, "on", "default"},
		{"repo", nil, "", "default"},
		{"theme", map[string]string{"theme": "bogus"}, "dracula", "env"},
	}
	for _, c := range cases {
		v, src := f.Resolve(c.key, c.flags, e)
		if v != c.want || src.String() != c.src {
			t.Errorf("%s: got %q (%s), want %q (%s)", c.key, v, src, c.want, c.src)
		}
	}
	if v, s := f.Resolve("theme", nil, env(nil)); v != "nord" || s.String() != "file:2" {
		t.Errorf("file theme: %q %s", v, s)
	}
}

func TestResolveRepoEnv(t *testing.T) {
	f, _, _ := Load(write(t, "repo = /x\n"))
	if v, s := f.Resolve("repo", nil, env(map[string]string{"AVAL_REPO": "/e"})); v != "/e" || s.String() != "env" {
		t.Errorf("got %q %s", v, s)
	}
	if v, s := f.Resolve("repo", nil, env(nil)); v != "/x" || s.String() != "file:1" {
		t.Errorf("got %q %s", v, s)
	}
}

func TestNoUpdateCheckEnvMapsToOff(t *testing.T) {
	f, _, _ := Load(write(t, "update_check = on\n"))
	v, s := f.Resolve("update_check", nil, env(map[string]string{"AVAL_NO_UPDATE_CHECK": "1"}))
	if v != "off" || s.String() != "env" {
		t.Errorf("got %q %s", v, s)
	}
	v, s = f.Resolve("update_check", nil, env(map[string]string{"AVAL_NO_UPDATE_CHECK": ""}))
	if v != "on" || s.String() != "file:1" {
		t.Errorf("empty env: got %q %s", v, s)
	}
}

func TestKeysAndThemes(t *testing.T) {
	if got := strings.Join(Keys(), ","); got != "repo,theme,background,glyphs,motion,update_check,language" {
		t.Errorf("Keys = %s", got)
	}
	if len(Themes) != 34 || Themes[0] != "aval" {
		t.Errorf("Themes = %v", Themes)
	}
}

func TestPath(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		goos string
		want string
	}{
		{"xdg", map[string]string{"XDG_CONFIG_HOME": "/xdg"}, "linux", filepath.Join("/xdg", "aval", "config")},
		{"appdata", map[string]string{"APPDATA": "/ad"}, "windows", filepath.Join("/ad", "aval", "config")},
		{"appdata ignored off windows", map[string]string{"APPDATA": "/ad"}, "linux", filepath.Join("/h", ".config", "aval", "config")},
		{"xdg beats appdata", map[string]string{"XDG_CONFIG_HOME": "/xdg", "APPDATA": "/ad"}, "windows", filepath.Join("/xdg", "aval", "config")},
		{"home", nil, "darwin", filepath.Join("/h", ".config", "aval", "config")},
	}
	for _, c := range cases {
		if got := Path(env(c.env), "/h", c.goos); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestValidateIsExportedForFlags(t *testing.T) {
	if msg := Validate("theme", "nord"); msg != "" {
		t.Errorf("nord is valid, got %q", msg)
	}
	msg := Validate("theme", "tokio-night")
	if !strings.Contains(msg, "Did you mean tokyo-night?") {
		t.Errorf("no suggestion in %q", msg)
	}
	if Validate("motion", "sometimes") == "" {
		t.Error("motion=sometimes must be invalid")
	}
}

func TestUnknownKeysCarryTheirLine(t *testing.T) {
	f, _, _ := Load(write(t, "# c\ntheme = nord\nfoo = 1\n\ncolour = red\n"))
	got := f.UnknownKeys()
	if len(got) != 2 || got[0].Key != "foo" || got[0].Line != 3 || got[1].Key != "colour" || got[1].Line != 5 {
		t.Errorf("UnknownKeys() = %+v", got)
	}
}

func TestEnvNameNamesTheOverride(t *testing.T) {
	for key, want := range map[string]string{
		"theme": "AVAL_THEME", "repo": "AVAL_REPO", "glyphs": "AVAL_GLYPHS",
		"background": "AVAL_BACKGROUND", "motion": "AVAL_MOTION",
		"update_check": "AVAL_NO_UPDATE_CHECK", "language": "AVAL_LANG", "nope": "",
	} {
		if got := EnvName(key); got != want {
			t.Errorf("EnvName(%q) = %q, want %q", key, got, want)
		}
	}
}

// A bad value comes back as data as well as text, so a front end can render
// it in the user's language: the key, the value, what is valid and the
// closest valid value.
func TestInvalidValueCarriesItsData(t *testing.T) {
	_, warns, err := Load(write(t, "language = esp\n"))
	if err != nil || len(warns) != 1 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	ie := warns[0].Invalid
	if ie == nil {
		t.Fatal("Warning.Invalid is nil")
	}
	if ie.Key != "language" || ie.Value != "esp" || strings.Join(ie.Valid, ",") != "auto,en,es" || ie.Suggestion != "es" {
		t.Errorf("Invalid = %+v", *ie)
	}
	err = Set(write(t, ""), "language", "esp")
	var se *InvalidValueError
	if !errors.As(err, &se) || se.Suggestion != "es" || strings.Join(se.Valid, ",") != "auto,en,es" {
		t.Errorf("Set error = %v", err)
	}
}

// language follows the same precedence as every key: AVAL_LANG beats the
// file, and auto is the default (the caller then detects the locale).
func TestResolveLanguage(t *testing.T) {
	f, _, _ := Load(write(t, "language = es\n"))
	if v, s := f.Resolve("language", nil, env(nil)); v != "es" || s.String() != "file:1" {
		t.Errorf("file: %q %s", v, s)
	}
	if v, s := f.Resolve("language", nil, env(map[string]string{"AVAL_LANG": "en"})); v != "en" || s.String() != "env" {
		t.Errorf("env: %q %s", v, s)
	}
	if v, s := f.Resolve("language", map[string]string{"language": "auto"}, env(map[string]string{"AVAL_LANG": "en"})); v != "auto" || s.String() != "flag" {
		t.Errorf("flag: %q %s", v, s)
	}
	empty, _, _ := Load(write(t, ""))
	if v, s := empty.Resolve("language", nil, env(nil)); v != "auto" || s.String() != "default" {
		t.Errorf("default: %q %s", v, s)
	}
}

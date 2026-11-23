package config

import (
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
	if err == nil || !strings.Contains(err.Error(), `theme = "tokio" is not a theme`) || !strings.Contains(err.Error(), "Did you mean tokyo-night?") {
		t.Fatalf("err = %v", err)
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
	if got := strings.Join(Keys(), ","); got != "repo,theme,background,glyphs,motion,update_check" {
		t.Errorf("Keys = %s", got)
	}
	if len(Themes) != 31 || Themes[0] != "aval" {
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

package repoloc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// repo makes a directory that looks like an aval checkout.
func repo(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "install.manifest"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return d
}

func config(t *testing.T, repo string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "aval", "config")
	if err := Save(p, repo); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolvePrecedence(t *testing.T) {
	flag, env, cfg, wd, exe := repo(t), repo(t), repo(t), repo(t), repo(t)
	cfgFile := config(t, cfg)
	empty := t.TempDir()

	cases := []struct {
		name string
		s    Sources
		want string
	}{
		{"flag wins", Sources{Flag: flag, Env: env, ConfigFile: cfgFile, Wd: wd, Exe: exe}, flag},
		{"env beats config", Sources{Env: env, ConfigFile: cfgFile, Wd: wd, Exe: exe}, env},
		{"config beats cwd", Sources{ConfigFile: cfgFile, Wd: wd, Exe: exe}, cfg},
		{"cwd beats executable", Sources{Wd: wd, Exe: exe}, wd},
		{"executable last", Sources{Wd: empty, Exe: exe}, exe},
		{"missing config file is skipped", Sources{ConfigFile: filepath.Join(empty, "nope"), Wd: wd}, wd},
		{"cwd walks up", Sources{Wd: mkdir(t, filepath.Join(wd, "a", "b"))}, wd},
		{"executable's parent", Sources{Wd: empty, Exe: mkdir(t, filepath.Join(exe, "bin"))}, exe},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Resolve(c.s)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// An explicit source is a statement of intent: when it points at the wrong
// place, falling through to a guess would install from the wrong checkout.
func TestResolveExplicitSourceThatPointsNowhereFails(t *testing.T) {
	wd := repo(t)
	empty := t.TempDir()

	cfg := config(t, empty)
	cases := []struct {
		name, source string
		s            Sources
	}{
		{"flag", "--repo", Sources{Flag: empty, Wd: wd}},
		{"env", "$AVAL_REPO", Sources{Env: empty, Wd: wd}},
		{"config", "repo in config " + cfg, Sources{ConfigFile: cfg, Wd: wd}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Resolve(c.s)
			var se *sourceError
			if !errors.As(err, &se) {
				t.Fatalf("err = %v, want a *sourceError", err)
			}
			if se.Source != c.source || se.Path != empty {
				t.Errorf("source %q path %q, want %q and %q", se.Source, se.Path, c.source, empty)
			}
		})
	}
}

func TestConfigPath(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	home := filepath.FromSlash("/home/u")

	cases := []struct {
		name string
		env  map[string]string
		goos string
		want string
	}{
		{"xdg", map[string]string{"XDG_CONFIG_HOME": "/x"}, "darwin", filepath.Join("/x", "aval", "config")},
		{"default on macOS", nil, "darwin", filepath.Join(home, ".config", "aval", "config")},
		{"default on linux", nil, "linux", filepath.Join(home, ".config", "aval", "config")},
		{"windows", map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`}, "windows", filepath.Join(`C:\Users\u\AppData\Roaming`, "aval", "config")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ConfigPath(env(c.env), home, c.goos); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "config")
	r := repo(t)
	if err := Save(p, r); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != r {
		t.Errorf("Load = %q, want %q", got, r)
	}

	// A second save replaces the value instead of appending a duplicate.
	r2 := repo(t)
	if err := Save(p, r2); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(p); got != r2 {
		t.Errorf("after resave Load = %q, want %q", got, r2)
	}
}

func TestLoadIgnoresCommentsAndOtherKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config")
	text := "# aval\n\nother = 1\n  repo = /some/where  \n"
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/some/where" {
		t.Errorf("Load = %q", got)
	}
}

func TestLoadMissingFileIsEmptyNotError(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "absent"))
	if err != nil || got != "" {
		t.Errorf("Load = %q, %v; want empty, nil", got, err)
	}
}

func mkdir(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveNothingFoundIsErrNotFound(t *testing.T) {
	empty := t.TempDir()
	_, err := Resolve(Sources{Wd: empty, Exe: empty})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestResolveExplicitSourceIsNotErrNotFound(t *testing.T) {
	_, err := Resolve(Sources{Flag: t.TempDir()})
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("an explicit source that points nowhere must fail loudly, not as not-found: %v", err)
	}
}

func TestCheckAcceptsACheckoutAndItsSubfolders(t *testing.T) {
	r := repo(t)
	sub := filepath.Join(r, "config")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{r, sub} {
		got, err := Check(p)
		if err != nil || got != r {
			t.Errorf("Check(%s) = %q, %v; want %q", p, got, err, r)
		}
	}
}

func TestCheckSaysWhatIsWrongAndHowToFixIt(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, path string
		want       error
	}{
		{"empty", "", errNoPath},
		{"missing", filepath.Join(dir, "nope"), errPathMissing},
		{"file", file, errNotADir},
		{"no manifest", dir, errNotACheckout},
	}
	for _, c := range cases {
		if _, err := Check(c.path); !errors.Is(err, c.want) {
			t.Errorf("%s: Check(%q) = %v, want %v", c.name, c.path, err, c.want)
		}
	}
}

func TestFindWalksUpFromInsideACheckout(t *testing.T) {
	r := repo(t)
	sub := filepath.Join(r, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Find(sub); got != r {
		t.Errorf("Find(%s) = %q, want %q", sub, got, r)
	}
	if got := Find(t.TempDir()); got != "" {
		t.Errorf("Find outside a checkout = %q, want empty", got)
	}
}

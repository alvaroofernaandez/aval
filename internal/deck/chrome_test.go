package deck

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func env(goos string, vars map[string]string, present ...string) Env {
	set := map[string]bool{}
	for _, p := range present {
		set[p] = true
	}
	return Env{
		Getenv: func(k string) string { return vars[k] },
		GOOS:   goos,
		Exists: func(p string) bool { return set[p] },
		LookPath: func(name string) (string, error) {
			if set["PATH:"+name] {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
	}
}

func TestFindBrowser(t *testing.T) {
	chromeMac := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	braveMac := "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
	cases := []struct {
		name string
		e    Env
		want string
	}{
		{"AVAL_CHROME wins", env("darwin", map[string]string{"AVAL_CHROME": "/opt/my-chrome"}, "/opt/my-chrome", chromeMac), "/opt/my-chrome"},
		{"chrome before brave on macOS", env("darwin", nil, chromeMac, braveMac), chromeMac},
		{"brave when chrome is missing", env("darwin", nil, braveMac), braveMac},
		{"user Applications folder", env("darwin", map[string]string{"HOME": "/Users/u"}, "/Users/u"+braveMac), "/Users/u" + braveMac},
		{"linux PATH", env("linux", nil, "PATH:chromium"), "/usr/bin/chromium"},
		// Paths follow the target OS, not the OS running the test.
		{"windows program files", env("windows", map[string]string{"ProgramFiles": `C:\PF`}, `C:\PF\Google\Chrome\Application\chrome.exe`), `C:\PF\Google\Chrome\Application\chrome.exe`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := FindBrowser(c.e)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestFindBrowserNothingExplainsTheFix(t *testing.T) {
	_, err := FindBrowser(env("darwin", nil))
	if err == nil || !strings.Contains(err.Error(), "AVAL_CHROME") {
		t.Fatalf("want an error naming AVAL_CHROME, got %v", err)
	}
	_, err = FindBrowser(env("darwin", map[string]string{"AVAL_CHROME": "/nope"}))
	if err == nil || !strings.Contains(err.Error(), "/nope") {
		t.Fatalf("a broken AVAL_CHROME must be reported, got %v", err)
	}
}

func TestParseOverflow(t *testing.T) {
	cases := []struct {
		dom   string
		want  string
		known bool
	}{
		{`<html lang="es" data-overflow="">`, "", true},
		{`<html lang="es" data-overflow="3,7">`, "3,7", true},
		{`<html lang="es" data-overflow="pending">`, "", false},
		{`<html lang="es">`, "", false},
	}
	for _, c := range cases {
		got, known := ParseOverflow(c.dom)
		if strings.Join(got, ",") != c.want || known != c.known {
			t.Errorf("%s: got %v %v, want %q %v", c.dom, got, known, c.want, c.known)
		}
	}
}

// fakeBrowser answers --dump-dom with the given overflow and writes a PDF on --print-to-pdf.
func fakeBrowser(t *testing.T, overflow string, calls *[][]string) Runner {
	return func(name string, args ...string) ([]byte, error) {
		*calls = append(*calls, append([]string{name}, args...))
		for _, a := range args {
			if a == "--dump-dom" {
				return []byte(`<html data-overflow="` + overflow + `"><body></body></html>`), nil
			}
			if p, ok := strings.CutPrefix(a, "--print-to-pdf="); ok {
				return nil, os.WriteFile(p, []byte("%PDF-1.7 fake"), 0o644)
			}
		}
		return nil, errors.New("unexpected call")
	}
}

func TestBuildWritesThePDF(t *testing.T) {
	var calls [][]string
	out := filepath.Join(t.TempDir(), "plan.pdf")
	err := Build(valid(), out, BuildOptions{Browser: "/b/chrome", Run: fakeBrowser(t, "", &calls), TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil || !strings.HasPrefix(string(b), "%PDF") {
		t.Fatalf("no PDF written: %v %q", err, b)
	}
	if len(calls) != 2 {
		t.Fatalf("want a check run and a print run, got %d calls", len(calls))
	}
	joined := strings.Join(calls[1], " ")
	for _, want := range []string{"--headless", "--no-pdf-header-footer", "file://"} {
		if !strings.Contains(joined, want) {
			t.Errorf("print argv lacks %q: %s", want, joined)
		}
	}
	// A fresh --user-data-dir makes Chromium hang ~30 s on first launch.
	if strings.Contains(joined, "--user-data-dir") {
		t.Error("print argv must not pass --user-data-dir")
	}
	check := strings.Join(calls[0], " ")
	if !strings.Contains(check, "--dump-dom") || !strings.Contains(check, "--virtual-time-budget=") {
		t.Errorf("the overflow check must dump the DOM after scripts ran: %s", check)
	}
	// --window-size makes headless Brave hang on --dump-dom; slides are fixed-size anyway.
	if strings.Contains(check, "--window-size") {
		t.Error("the overflow check must not pass --window-size")
	}
}

func TestBuildStopsOnOverflow(t *testing.T) {
	var calls [][]string
	out := filepath.Join(t.TempDir(), "plan.pdf")
	err := Build(valid(), out, BuildOptions{Browser: "/b/chrome", Run: fakeBrowser(t, "2,3", &calls), TempDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "slides 2, 3") {
		t.Fatalf("want an overflow error naming slides 2, 3, got %v", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("no PDF may be written when a slide overflows")
	}
}

func TestBuildRejectsAnInvalidDeck(t *testing.T) {
	var calls [][]string
	d := valid()
	d.Title = ""
	if err := Build(d, filepath.Join(t.TempDir(), "x.pdf"), BuildOptions{Browser: "b", Run: fakeBrowser(t, "", &calls), TempDir: t.TempDir()}); err == nil {
		t.Fatal("want a validation error")
	}
	if len(calls) != 0 {
		t.Error("the browser must not start for an invalid spec")
	}
}

// A Windows path needs the third slash: file:///C:/..., never file://C:/...
func TestFileURL(t *testing.T) {
	cases := map[string]string{
		"/tmp/aval-deck-1/deck.html": "file:///tmp/aval-deck-1/deck.html",
		"C:/Users/x/deck.html":       "file:///C:/Users/x/deck.html",
		"/tmp/a b/deck.html":         "file:///tmp/a%20b/deck.html",
	}
	for in, want := range cases {
		if got := fileURL(in); got != want {
			t.Errorf("fileURL(%q) = %q, want %q", in, got, want)
		}
	}
}

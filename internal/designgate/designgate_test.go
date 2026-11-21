package designgate

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRatio(t *testing.T) {
	tests := []struct {
		fg, bg string
		want   float64
	}{
		{"#767676", "#ffffff", 4.54},
		{"#fff", "#000", 21.00},
		{"000000", "ffffff", 21.00},
		{"#777777", "#ffffff", 4.48},
		{"#1d1d1f", "#ffffff", 16.83},
	}
	for _, tc := range tests {
		got, err := Ratio(tc.fg, tc.bg)
		if err != nil {
			t.Fatalf("%s on %s: %v", tc.fg, tc.bg, err)
		}
		if math.Abs(got-tc.want) > 0.01 {
			t.Errorf("%s on %s = %.3f, want %.2f", tc.fg, tc.bg, got, tc.want)
		}
	}
	for _, bad := range []string{"#12", "zzzzzz", "", "#12345"} {
		if _, err := Ratio(bad, "#fff"); err == nil {
			t.Errorf("Ratio(%q) accepted a bad hex", bad)
		}
	}
}

func TestContrastExit(t *testing.T) {
	tests := []struct {
		name         string
		fg, bg       string
		large        bool
		code         int
		wantContains []string
	}{
		{"passes AA", "#767676", "#fff", false, 0, []string{"4.54:1", "Normal text  AA  (4.5:1): PASS", "AAA (7.0:1): FAIL"}},
		{"fails AA", "#777777", "#fff", false, 1, []string{"4.48:1", "Normal text  AA  (4.5:1): FAIL"}},
		{"large text lowers the bar", "#777777", "#fff", true, 0, []string{"Large text   AA  (3.0:1): PASS"}},
		{"bad hex is a usage error", "nope", "#fff", false, 2, []string{"ERROR"}},
	}
	for _, tc := range tests {
		var b bytes.Buffer
		if got := Contrast(&b, tc.fg, tc.bg, tc.large); got != tc.code {
			t.Errorf("%s: exit %d, want %d\n%s", tc.name, got, tc.code, b.String())
		}
		for _, s := range tc.wantContains {
			if !strings.Contains(b.String(), s) {
				t.Errorf("%s: output lacks %q:\n%s", tc.name, s, b.String())
			}
		}
	}
}

func TestTokens(t *testing.T) {
	dir := t.TempDir()
	ok := write(t, dir, "ok.json", `{"color":{"a":{"$value":"#fff"},"b":{"$value":"{color.a}"}}}`)
	badRef := write(t, dir, "ref.json", `{"x":{"$value":"{nope.here}"}}`)
	badJSON := write(t, dir, "bad.json", `{`)
	suffix := write(t, dir, "motion.json", `{"duration":{"fast":{"$value":"100ms"}}}`)
	cross := write(t, dir, "use.json", `{"t":{"$value":"{motion.duration.fast}"}}`)

	tests := []struct {
		name  string
		paths []string
		code  int
		want  string
	}{
		{"resolved alias", []string{ok}, 0, "OK"},
		{"unresolved alias fails", []string{badRef}, 1, "unresolved"},
		{"invalid json fails", []string{badJSON}, 1, "invalid JSON"},
		{"file-prefixed cross ref resolves", []string{suffix, cross}, 0, "OK"},
		{"missing path fails", []string{filepath.Join(dir, "missing.json")}, 1, "not found"},
		{"directory is globbed", []string{dir}, 1, "FAIL"},
		{"no paths is an error", nil, 1, "ERROR"},
	}
	for _, tc := range tests {
		var b bytes.Buffer
		if got := Tokens(&b, tc.paths); got != tc.code {
			t.Errorf("%s: exit %d, want %d\n%s", tc.name, got, tc.code, b.String())
		}
		if !strings.Contains(b.String(), tc.want) {
			t.Errorf("%s: output lacks %q:\n%s", tc.name, tc.want, b.String())
		}
	}
}

const goodColors = `{
 "base":{"white":{"$value":"#ffffff"},"ink":{"$value":"#1d1d1f"},"grey":{"$value":"#767676"},"blue":{"$value":"#0055cc"}},
 "semantic":{
  "text":{"primary":{"$value":"{base.ink}"},"secondary":{"$value":"{base.grey}"},"link":{"$value":"{base.blue}"},"on-action":{"$value":"{base.white}"}},
  "surface":{"page":{"$value":"{base.white}"},"card":{"$value":"{base.white}"}},
  "action":{"primary":{"$value":"{base.blue}"}},
  "border":{"strong":{"$value":"#767676"}}
 },
 "dark":{"text":{"primary":{"$value":"#ffffff"},"link":{"$value":"#66aaff"}},"surface":{"page":{"$value":"#000000"},"card":{"$value":"#111111"}}}
}`

func TestPairs(t *testing.T) {
	dir := t.TempDir()
	good := write(t, dir, "good.json", goodColors)
	weak := write(t, dir, "weak.json", strings.Replace(goodColors, `"secondary":{"$value":"{base.grey}"}`, `"secondary":{"$value":"#bbbbbb"}`, 1))
	empty := write(t, dir, "empty.json", `{}`)

	var b bytes.Buffer
	if got := Pairs(&b, good, false); got != 0 {
		t.Fatalf("good set: exit %d\n%s", got, b.String())
	}
	for _, s := range []string{"=== LIGHT (required) ===", "=== DARK (required) ===", "OK:"} {
		if !strings.Contains(b.String(), s) {
			t.Errorf("good set output lacks %q:\n%s", s, b.String())
		}
	}

	b.Reset()
	if got := Pairs(&b, weak, false); got != 1 || !strings.Contains(b.String(), "secondary text on page") {
		t.Errorf("weak set: exit %d, want 1\n%s", got, b.String())
	}

	b.Reset()
	if got := Pairs(&b, empty, false); got != 1 || !strings.Contains(b.String(), "token missing") {
		t.Errorf("empty set must fail on missing tokens: exit %d\n%s", got, b.String())
	}

	b.Reset()
	Pairs(&b, good, true)
	if !strings.Contains(b.String(), "AAA(7:1)") {
		t.Errorf("--aaa not reported:\n%s", b.String())
	}

	b.Reset()
	if got := Pairs(&b, filepath.Join(dir, "none.json"), false); got != 1 {
		t.Errorf("missing file: exit %d", got)
	}
}

func hitKinds(line string, tw, css bool) []string {
	var out []string
	for _, h := range LintLine(line, tw, css) {
		out = append(out, h.Kind)
	}
	return out
}

func TestLintLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		tw   bool
		css  bool
		want string
	}{
		{"hex", "color: #ff0000;", true, true, "hex"},
		{"short hex", "color: #f00;", true, true, "hex"},
		{"px", "margin: 13px;", true, true, "px"},
		{"1px ok", "border: 1px solid;", true, true, ""},
		{"0px ok", "margin: 0px;", true, true, ""},
		{"time", "transition: opacity 200ms;", true, true, "time"},
		{"var exempts the reference only", "padding: var(--space-2) 13px;", true, true, "px"},
		{"pure var ok", "padding: var(--space-2) var(--space-3);", true, true, ""},
		{"nested var with fallback", "gap: var(--a, var(--b, 12px));", true, true, ""},
		{"token def exempt", "--space-2: 8px;", true, true, ""},
		{"allow comment", "margin: 13px; /* ds-allow-hardcode */", true, true, ""},
		{"media condition skips px", "@media (min-width: 768px) {", true, true, ""},
		{"tailwind palette", `className="bg-gray-500 p-2"`, true, true, "tailwind-palette"},
		{"tailwind off", `className="bg-gray-500"`, false, true, ""},
		{"ring-offset palette", `class="ring-offset-blue-500"`, true, true, "tailwind-palette"},
		{"semantic tailwind ok", `className="bg-primary"`, true, true, ""},
		{"font family literal", "font-family: Arial, sans-serif;", true, true, "font-family"},
		{"font family via var", "font-family: var(--font-body);", true, true, ""},
		{"prose hash is not a colour in markup", "<p>issue #412 fixed</p>", true, false, ""},
		{"markup hex in value position", `<path fill="#ff0000"/>`, true, false, "hex"},
		{"markup durations are content", "<td>8m 04s</td>", true, false, ""},
		{"comment line skipped", "// margin: 13px", true, true, ""},
		{"digit-suffixed ident is not px", "grid x12px", true, true, ""},
		{"theme() exempts", "margin: theme(spacing.4) 13px;", true, true, ""},
	}
	for _, tc := range tests {
		got := strings.Join(hitKinds(tc.line, tc.tw, tc.css), ",")
		if got != tc.want {
			t.Errorf("%s: %q -> [%s], want [%s]", tc.name, tc.line, got, tc.want)
		}
	}
}

func TestHardcodes(t *testing.T) {
	dir := t.TempDir()
	clean := write(t, dir, "clean.css", "a { color: var(--c); }\n")
	dirty := write(t, dir, "dirty.css", "a { color: #ff0000; }\n/* multi\n line 388px comment\n*/\nb { margin: 13px; }\n")
	region := write(t, dir, "region.css", "/* ds-allow-hardcode:start */\na { color: #f00; }\n/* ds-allow-hardcode:end */\n")
	write(t, dir, "node_modules/x.css", "a { color: #f00; }\n")

	tests := []struct {
		name  string
		paths []string
		code  int
		want  string
	}{
		{"clean", []string{clean}, 0, "OK: no hardcoded values found."},
		{"dirty", []string{dirty}, 1, "dirty.css:1: hardcoded hex '#ff0000'"},
		{"multi-line comment is prose", []string{dirty}, 1, "FAIL: 2 hardcoded value(s)"},
		{"allow region", []string{region}, 0, "OK"},
		{"directory skips node_modules", []string{dir}, 1, "Scanned 3 of 3"},
		{"missing path", []string{filepath.Join(dir, "nope")}, 1, "path(s) not found"},
		{"nothing lintable", []string{write(t, dir, "x.txt", "")}, 1, "no lintable"},
	}
	for _, tc := range tests {
		var b bytes.Buffer
		if got := Hardcodes(&b, tc.paths, HardcodeOpts{Tailwind: true}); got != tc.code {
			t.Errorf("%s: exit %d, want %d\n%s", tc.name, got, tc.code, b.String())
		}
		if !strings.Contains(b.String(), tc.want) {
			t.Errorf("%s: output lacks %q:\n%s", tc.name, tc.want, b.String())
		}
	}

	var b bytes.Buffer
	if got := Hardcodes(&b, []string{dirty}, HardcodeOpts{Exts: map[string]bool{".tsx": true}}); got != 1 {
		t.Errorf("--ext filter should leave nothing lintable, exit %d", got)
	}
}

func TestHardcodesHTMLScope(t *testing.T) {
	dir := t.TempDir()
	page := write(t, dir, "p.html", "<td>13px wait 8m 04s</td>\n<style>\na { margin: 13px; }\n</style>\n")
	var b bytes.Buffer
	if got := Hardcodes(&b, []string{page}, HardcodeOpts{Tailwind: true}); got != 1 {
		t.Fatalf("exit %d\n%s", got, b.String())
	}
	if !strings.Contains(b.String(), "p.html:3:") || strings.Contains(b.String(), "p.html:1:") {
		t.Errorf("only the <style> line may be flagged:\n%s", b.String())
	}
}

func TestThemeRefs(t *testing.T) {
	dir := t.TempDir()
	theme := write(t, dir, "theme.css", ":root{--color-a:#fff;--space:4px}\n")
	extra := write(t, dir, "extra.css", ":root{--color-b:#000}\n")
	good := write(t, dir, "src/good.css", "a{color:var(--color-a);background:var(--color-b);margin:var(--local)}\n:root{--local:1}\n")
	fallback := write(t, dir, "src/fb.css", "a{color:var(--nope, red)}\n")
	bad := write(t, dir, "src/bad.css", "a{color:var(--nope)}\n")

	tests := []struct {
		name   string
		themes []string
		paths  []string
		code   int
		want   string
	}{
		{"all resolve", []string{theme, extra}, []string{good}, 0, "OK"},
		{"fallback is not a failure", []string{theme}, []string{fallback}, 0, "carry a fallback"},
		{"undefined fails", []string{theme}, []string{bad}, 1, "var(--nope) resolves to nothing"},
		{"no theme", nil, []string{good}, 1, "ERROR"},
		{"theme without vars", []string{write(t, dir, "empty.css", "a{}")}, []string{good}, 1, "no CSS variables"},
		{"missing path", []string{theme}, []string{filepath.Join(dir, "x")}, 1, "not found"},
	}
	for _, tc := range tests {
		var b bytes.Buffer
		if got := ThemeRefs(&b, tc.themes, tc.paths); got != tc.code {
			t.Errorf("%s: exit %d, want %d\n%s", tc.name, got, tc.code, b.String())
		}
		if !strings.Contains(b.String(), tc.want) {
			t.Errorf("%s: output lacks %q:\n%s", tc.name, tc.want, b.String())
		}
	}
}

func TestComponent(t *testing.T) {
	root := t.TempDir()
	write(t, root, "components/button.md", "# Button\nAnatomy\nVariants\nStates\nTokens: {semantic.x}\nAccessibility: aria\n")
	write(t, root, "components/icon-system.md", "Tokens and a11y only\n")

	var b bytes.Buffer
	if got := Component(&b, root); got != 0 {
		t.Fatalf("complete specs: exit %d\n%s", got, b.String())
	}

	write(t, root, "components/card.md", "# Card\nAnatomy only\n")
	b.Reset()
	if got := Component(&b, root); got != 1 || !strings.Contains(b.String(), "missing: variants, states, token mapping, accessibility") {
		t.Errorf("incomplete spec: exit %d\n%s", got, b.String())
	}

	b.Reset()
	if got := Component(&b, t.TempDir()); got != 1 || !strings.Contains(b.String(), "not found") {
		t.Errorf("no components dir: exit %d\n%s", got, b.String())
	}
}

func TestEmoji(t *testing.T) {
	dir := t.TempDir()
	clean := write(t, dir, "ok.md", "plain text → arrows are fine\n")
	dirty := write(t, dir, "bad.md", "done \U0001F680\nsecond \u2713\n")
	ui := write(t, dir, "x.tsx", "<p>a \u2014 b</p>\n")
	md := write(t, dir, "dash.md", "a \u2014 b is fine in docs\n")

	tests := []struct {
		name  string
		paths []string
		code  int
		want  string
	}{
		{"clean", []string{clean}, 0, "OK"},
		{"emoji and dingbat", []string{dirty}, 1, "FAIL: 2 emoji"},
		{"dash in UI file", []string{ui}, 1, "em/en-dash"},
		{"dash in docs ok", []string{md}, 0, "OK"},
		{"missing path", []string{filepath.Join(dir, "gone")}, 1, "not found"},
		{"no paths", nil, 1, "ERROR"},
		{"dir with nothing scannable", []string{t.TempDir()}, 1, "no scannable"},
	}
	for _, tc := range tests {
		var b bytes.Buffer
		if got := Emoji(&b, tc.paths); got != tc.code {
			t.Errorf("%s: exit %d, want %d\n%s", tc.name, got, tc.code, b.String())
		}
		if !strings.Contains(b.String(), tc.want) {
			t.Errorf("%s: output lacks %q:\n%s", tc.name, tc.want, b.String())
		}
	}
}

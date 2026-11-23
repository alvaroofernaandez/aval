package ui

import (
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// renderer is a deterministic renderer: no terminal is queried.
func renderer(p termenv.Profile) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(p)
	return r
}

func capsFor(p termenv.Profile, dark bool) caps.Caps {
	return caps.Caps{Profile: p, Unicode: true, Dark: dark, Width: 80, Height: 24}
}

func mustTheme(t *testing.T, name string, c caps.Caps, r *lipgloss.Renderer) Theme {
	t.Helper()
	th, err := NewTheme(name, c, r)
	if err != nil {
		t.Fatalf("NewTheme(%q): %v", name, err)
	}
	return th
}

func TestAvalPaletteMatchesDesignContract(t *testing.T) {
	// Pinned against .agents/DESIGN.md section 10. A hex edited by hand fails
	// here before it ships.
	cases := map[string]Palette{
		"aval-dark": {
			Background: "#000000",
			Text:       "#ffffff", Strong: "#ffffff", Key: "#ffffff",
			Muted: "#a3a3a3", Rule: "#2e2e2e",
			OK: "#98BB6C", Warn: "#E6C384", Fail: "#E46876",
		},
		"aval-light": {
			Background: "#ffffff",
			Text:       "#000000", Strong: "#000000", Key: "#000000",
			Muted: "#525252", Rule: "#d4d4d4",
			OK: "#5a7339", Warn: "#77713f", Fail: "#C34043",
		},
	}
	for name, want := range cases {
		got, err := LookupPalette(name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", name, err)
		}
		want.Name = name
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", name, got, want)
		}
	}
}

func TestBareNameFollowsBackground(t *testing.T) {
	for _, c := range []struct {
		dark bool
		want string
	}{{true, "aval-dark"}, {false, "aval-light"}} {
		p, err := LookupPalette("aval", c.dark)
		if err != nil {
			t.Fatal(err)
		}
		if p.Name != c.want {
			t.Errorf("aval with dark=%v resolved to %q, want %q", c.dark, p.Name, c.want)
		}
	}
	// A pinned variant ignores the background.
	if p, _ := LookupPalette("aval-light", true); p.Name != "aval-light" {
		t.Errorf("pinned aval-light resolved to %q", p.Name)
	}
}

func TestAvalTokensReachAAContrast(t *testing.T) {
	// Every token that carries meaning reaches 4.5:1 on its own background.
	// rule is decorative and carries nothing, so it is exempt.
	for _, name := range []string{"aval-dark", "aval-light"} {
		p, err := LookupPalette(name, true)
		if err != nil {
			t.Fatalf("LookupPalette(%q): %v", name, err)
		}
		for token, fg := range p.meaningful() {
			r, err := gate.Ratio(fg, p.Background)
			if err != nil {
				t.Fatalf("%s.%s: %v", name, token, err)
			}
			if r < 4.5 {
				t.Errorf("%s.%s %s on %s = %.2f:1, want >= 4.5", name, token, fg, p.Background, r)
			}
		}
	}
}

func TestTerminalThemeSetsNoHexColour(t *testing.T) {
	// terminal inherits the user's own palette: ANSI 16 only.
	p, err := LookupPalette("terminal", true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "terminal" {
		t.Fatalf("terminal resolved to %q", p.Name)
	}
	for token, v := range p.all() {
		if strings.HasPrefix(v, "#") {
			t.Errorf("terminal.%s = %s, want an ANSI index or the default", token, v)
		}
	}
	if p.OK != "2" || p.Warn != "3" || p.Fail != "1" {
		t.Errorf("terminal state colours = %q/%q/%q, want ANSI 2/3/1", p.OK, p.Warn, p.Fail)
	}
}

func TestMonoThemeSetsNoColour(t *testing.T) {
	p, err := LookupPalette("mono", false)
	if err != nil {
		t.Fatal(err)
	}
	for token, v := range p.all() {
		if v != "" {
			t.Errorf("mono.%s = %q, want no colour", token, v)
		}
	}
}

// colourSGR matches any foreground or background colour sequence.
var colourSGR = regexp.MustCompile(`\x1b\[[0-9;]*(3[0-9]|4[0-9]|9[0-7]|10[0-7]|38;|48;)`)

func renderAll(th Theme) string {
	var b strings.Builder
	for _, s := range th.styles() {
		b.WriteString(s.Render("x"))
	}
	b.WriteString(th.ProgressBar(3, 10, 10))
	return b.String()
}

func TestAsciiProfileEmitsNoColour(t *testing.T) {
	// NO_COLOR wins over every theme, even when the renderer could do colour.
	r := renderer(termenv.TrueColor)
	for _, name := range []string{"aval", "terminal", "mono"} {
		th := mustTheme(t, name, capsFor(termenv.Ascii, true), r)
		out := renderAll(th)
		if strings.Contains(out, "\x1b[3") || strings.Contains(out, "38;") || strings.Contains(out, "48;") || colourSGR.MatchString(out) {
			t.Errorf("%s under Ascii emitted colour: %q", name, out)
		}
		if !strings.Contains(th.Strong.Render("x"), "\x1b[1") {
			t.Errorf("%s under Ascii lost bold on strong: %q", name, th.Strong.Render("x"))
		}
		if !strings.Contains(th.Cursor.Render("x"), "7m") {
			t.Errorf("%s under Ascii lost reverse on cursor: %q", name, th.Cursor.Render("x"))
		}
	}
}

func TestAvalEmitsItsHexUnderTrueColor(t *testing.T) {
	// The positive control for the Ascii test: colour does reach the output
	// when the terminal can show it.
	th := mustTheme(t, "aval", capsFor(termenv.TrueColor, true), renderer(termenv.TrueColor))
	// #98BB6C = 152;187;108
	if got := th.OK.Render("x"); !strings.Contains(got, "38;2;152;187;108") {
		t.Errorf("aval dark ok = %q, want the springGreen truecolor sequence", got)
	}
}

func TestMutedIsNeverFaint(t *testing.T) {
	// DESIGN.md: muted is never SGR dim or bright black.
	for _, name := range []string{"aval", "terminal", "mono"} {
		th := mustTheme(t, name, capsFor(termenv.ANSI, true), renderer(termenv.ANSI))
		if th.Muted.GetFaint() {
			t.Errorf("%s muted is faint", name)
		}
		if fg, ok := th.Muted.GetForeground().(lipgloss.Color); ok && (fg == "8" || fg == "0") {
			t.Errorf("%s muted is bright black", name)
		}
	}
}

func TestNoStylePaintsABackground(t *testing.T) {
	// The terminal belongs to the user: the cursor row is reverse video, and
	// no style paints a canvas.
	for _, name := range []string{"aval", "terminal", "mono"} {
		th := mustTheme(t, name, capsFor(termenv.TrueColor, true), renderer(termenv.TrueColor))
		for i, s := range th.styles() {
			if bg := s.GetBackground(); bg != (lipgloss.NoColor{}) {
				t.Errorf("%s style %d sets background %v", name, i, bg)
			}
		}
		if !th.Cursor.GetReverse() || !th.Selected.GetReverse() {
			t.Errorf("%s cursor row must be reverse video", name)
		}
	}
}

func TestUnknownThemeSuggestsClosestName(t *testing.T) {
	for in, want := range map[string]string{
		"avl":        `"aval"`,
		"termnal":    `"terminal"`,
		"mon":        `"mono"`,
		"aval-drak":  `"aval-dark"`,
		"aval-lihgt": `"aval-light"`,
	} {
		_, err := NewTheme(in, capsFor(termenv.ANSI, true), renderer(termenv.ANSI))
		if err == nil {
			t.Errorf("NewTheme(%q) = nil error, want unknown theme", in)
			continue
		}
		if !strings.Contains(err.Error(), "did you mean "+want) {
			t.Errorf("NewTheme(%q) error = %q, want a suggestion of %s", in, err, want)
		}
	}
}

func TestThemeNamesListEveryVariant(t *testing.T) {
	got := strings.Join(ThemeNames(), ",")
	for _, n := range []string{"aval", "aval-dark", "aval-light", "terminal", "mono"} {
		if !strings.Contains(","+got+",", ","+n+",") {
			t.Errorf("ThemeNames() = %s, missing %s", got, n)
		}
	}
}

func TestProgressBarFillsProportionally(t *testing.T) {
	th := DefaultTheme()
	cases := []struct {
		done, total, width int
		wantFilled         int
	}{
		{0, 10, 10, 0},
		{5, 10, 10, 5},
		{10, 10, 10, 10},
		{3, 4, 8, 6},
	}
	for _, c := range cases {
		bar := th.ProgressBar(c.done, c.total, c.width)
		if got := strings.Count(bar, th.Glyphs.BarFull); got != c.wantFilled {
			t.Errorf("ProgressBar(%d,%d,%d) filled %d, want %d", c.done, c.total, c.width, got, c.wantFilled)
		}
	}
}

func TestProgressBarNeverExceedsItsWidth(t *testing.T) {
	// done > total happens transiently if an action reports twice; the bar must
	// not spill past its box and corrupt the frame.
	th := DefaultTheme()
	bar := th.ProgressBar(20, 10, 10)
	if got := strings.Count(bar, th.Glyphs.BarFull); got != 10 {
		t.Errorf("overfilled bar has %d cells, want 10", got)
	}
}

func TestProgressBarHandlesDegenerateInput(t *testing.T) {
	// A zero-width terminal or an empty plan must render nothing, not panic.
	th := DefaultTheme()
	if got := th.ProgressBar(0, 0, 10); got != "" {
		t.Errorf("zero total = %q, want empty", got)
	}
	if got := th.ProgressBar(1, 10, 0); got != "" {
		t.Errorf("zero width = %q, want empty", got)
	}
}

func TestStateColoursOnlyTheGlyph(t *testing.T) {
	th := mustTheme(t, "aval", capsFor(termenv.TrueColor, true), renderer(termenv.TrueColor))
	got := th.State(StateOK, "linked")
	ok := "38;2;152;187;108" // #98BB6C
	i := strings.Index(got, ok)
	if i < 0 || i > strings.Index(got, th.Glyphs.OK) {
		t.Fatalf("State = %q, want the ok sequence before the glyph", got)
	}
	word := got[strings.Index(got, th.Glyphs.OK)+len(th.Glyphs.OK):]
	if strings.Contains(word, ok) {
		t.Errorf("word carries the ok colour: %q", word)
	}
	if want := th.Text.Render("linked"); !strings.HasSuffix(got, want) {
		t.Errorf("State = %q, want the word wrapped as %q", got, want)
	}
}

func TestStateIsPlainUnderAscii(t *testing.T) {
	for _, uni := range []bool{true, false} {
		c := capsFor(termenv.Ascii, true)
		c.Unicode = uni
		th := mustTheme(t, "aval", c, renderer(termenv.TrueColor))
		for k, g := range map[StateKind]string{StateOK: th.Glyphs.OK, StateWarn: th.Glyphs.Warn, StateFail: th.Glyphs.Fail} {
			if got, want := th.State(k, "word"), g+" word"; got != want {
				t.Errorf("State(%d) = %q, want %q", k, got, want)
			}
		}
	}
}

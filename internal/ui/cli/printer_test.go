package cli

import (
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/ui"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

func theme(t *testing.T, c caps.Caps) ui.Theme {
	t.Helper()
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(c.Profile)
	th, err := ui.NewTheme("aval", c, r)
	if err != nil {
		t.Fatal(err)
	}
	return th
}

func TestPlainPrinterReturnsTheWordUntouched(t *testing.T) {
	p := Plain()
	for name, got := range map[string]string{
		"ok": p.OK("linked"), "warn": p.Warn("skipped"), "fail": p.Fail("FAILED: x"),
		"muted": p.Muted("m"), "strong": p.Strong("s"),
	} {
		want := map[string]string{"ok": "linked", "warn": "skipped", "fail": "FAILED: x", "muted": "m", "strong": "s"}[name]
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestPrinterUnicodeColourPrefixesGlyphAndKeepsWord(t *testing.T) {
	c := caps.Caps{Profile: termenv.TrueColor, Unicode: true, Dark: true, Width: 80, Height: 24}
	p := New(c, theme(t, c))
	for _, tc := range []struct{ got, want string }{
		{p.OK("linked"), "√ linked"},
		{p.Warn("skipped"), "! skipped"},
		{p.Fail("FAILED"), "× FAILED"},
	} {
		if plain := ansi.ReplaceAllString(tc.got, ""); plain != tc.want {
			t.Errorf("plain text = %q, want %q", plain, tc.want)
		}
		if !ansi.MatchString(tc.got) {
			t.Errorf("%q carries no colour", tc.got)
		}
	}
}

func TestPrinterASCIITierUsesASCIIGlyphs(t *testing.T) {
	c := caps.Caps{Profile: termenv.ANSI, Unicode: false, Dark: true}
	p := New(c, theme(t, c))
	for _, tc := range []struct{ got, want string }{
		{p.OK("linked"), "ok linked"},
		{p.Warn("skipped"), "! skipped"},
		{p.Fail("FAILED"), "x FAILED"},
	} {
		if plain := ansi.ReplaceAllString(tc.got, ""); plain != tc.want {
			t.Errorf("plain text = %q, want %q", plain, tc.want)
		}
	}
}

func TestPrinterWithoutColourEmitsNoEscapes(t *testing.T) {
	c := caps.Caps{Profile: termenv.Ascii, Unicode: true, Dark: true}
	p := New(c, theme(t, c))
	if got := p.OK("linked"); got != "√ linked" {
		t.Errorf("OK = %q", got)
	}
	if got := p.Muted("m"); got != "m" {
		t.Errorf("Muted = %q", got)
	}
}

func TestWidthIgnoresEscapes(t *testing.T) {
	c := caps.Caps{Profile: termenv.TrueColor, Unicode: true, Dark: true}
	p := New(c, theme(t, c))
	if w := p.Width(p.OK("OK")); w != 4 {
		t.Errorf("Width = %d", w)
	}
	if w := Plain().Width("abc"); w != 3 {
		t.Errorf("plain Width = %d", w)
	}
}

func TestPrinterColoursOnlyTheGlyph(t *testing.T) {
	c := caps.Caps{Profile: termenv.TrueColor, Unicode: true, Dark: true, Width: 80, Height: 24}
	th := theme(t, c)
	p := New(c, th)
	got := p.OK("linked")
	if want := th.Text.Render("linked"); !strings.HasSuffix(got, want) {
		t.Errorf("OK = %q, want the word in the text colour %q", got, want)
	}
	if strings.Contains(got[strings.Index(got, "√")+len("√"):], "38;2;152;187;108") {
		t.Errorf("OK colours the word: %q", got)
	}
}

func TestPrinterCarriesItsLanguage(t *testing.T) {
	if got := Plain().Lang(); got != i18n.English {
		t.Errorf("Plain() speaks %q, want en", got)
	}
	p := Plain()
	es := p.WithLang(i18n.Spanish)
	if es.Lang() != i18n.Spanish || p.Lang() != i18n.English {
		t.Errorf("WithLang: copy speaks %q, original %q", es.Lang(), p.Lang())
	}
}

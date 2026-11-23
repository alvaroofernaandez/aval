package deck

import (
	"bytes"
	"encoding/base64"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

// fontBudget is the measured total of the two upstream variable fonts
// (google/fonts, unmodified); D6 lets it rise to that total.
const fontBudget = 680_000

func embeddedFonts(t *testing.T) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	_ = fs.WalkDir(fontFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && path.Ext(p) == ".ttf" {
			b, _ := fs.ReadFile(fontFS, p)
			out[path.Base(p)] = b
		}
		return nil
	})
	return out
}

func TestRenderHTMLHasNoExternalURL(t *testing.T) {
	html, err := Render(everyLayout())
	if err != nil {
		t.Fatal(err)
	}
	for _, scheme := range []string{"http://", "https://"} {
		if i := strings.Index(html, scheme); i >= 0 {
			t.Errorf("the rendered deck reaches the network: %q", html[i:min(len(html), i+80)])
		}
	}
}

var fontFaceRe = regexp.MustCompile(`@font-face\s*\{[^}]*font-family:\s*"([^"]+)";[^}]*font-weight:\s*(\d+) (\d+);[^}]*src:\s*url\(data:font/ttf;base64,([A-Za-z0-9+/=]+)\)`)

func TestRenderInlinesTheBrandFontFaces(t *testing.T) {
	html, err := Render(everyLayout())
	if err != nil {
		t.Fatal(err)
	}
	fonts := embeddedFonts(t)
	faces := fontFaceRe.FindAllStringSubmatch(html, -1)
	if len(faces) != 2 {
		t.Fatalf("got %d @font-face rules with an inline TrueType source, want 2", len(faces))
	}
	need := map[string][]int{"Space Grotesk": {600, 700}, "IBM Plex Sans": {400, 600}}
	for _, f := range faces {
		data, err := base64.StdEncoding.DecodeString(f[4])
		if err != nil {
			t.Fatalf("%s: %v", f[1], err)
		}
		match := false
		for _, b := range fonts {
			match = match || bytes.Equal(b, data)
		}
		if !match {
			t.Errorf("%s: the inline source is not one of the embedded files", f[1])
		}
		lo, hi := atoi(f[2]), atoi(f[3])
		for _, w := range need[f[1]] {
			if w < lo || w > hi {
				t.Errorf("%s: weight %d is outside the declared range %d-%d", f[1], w, lo, hi)
			}
		}
		delete(need, f[1])
	}
	for fam := range need {
		t.Errorf("no @font-face for %s", fam)
	}
}

func TestEmbeddedFontsAreTrueTypeWithTheirLicences(t *testing.T) {
	fonts := embeddedFonts(t)
	if len(fonts) != 2 {
		t.Fatalf("got %d embedded font files, want 2", len(fonts))
	}
	for name, b := range fonts {
		if !bytes.HasPrefix(b, []byte{0, 1, 0, 0}) {
			t.Errorf("%s is not a TrueType font", name)
		}
	}
	for _, lic := range []string{"assets/fonts/OFL-spacegrotesk.txt", "assets/fonts/OFL-ibmplexsans.txt"} {
		b, err := fs.ReadFile(fontFS, lic)
		if err != nil || !bytes.Contains(b, []byte("SIL OPEN FONT LICENSE")) {
			t.Errorf("%s must hold the SIL Open Font License text", lic)
		}
	}
}

func TestEmbeddedFontsStayUnderBudget(t *testing.T) {
	total := 0
	for _, b := range embeddedFonts(t) {
		total += len(b)
	}
	if total > fontBudget {
		t.Errorf("embedded fonts total %d bytes, budget %d", total, fontBudget)
	}
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

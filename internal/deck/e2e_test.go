package deck

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBuildRealBrowser prints every layout with the machine's browser. Opt-in
// (AVAL_DECK_E2E=1): it needs a Chromium-based browser, and no network: the
// brand fonts are embedded. AVAL_DECK_E2E_OUT keeps the PDF for a visual check.
// With pdffonts (poppler) on PATH it also checks both brand fonts are in the PDF.
func TestBuildRealBrowser(t *testing.T) {
	if os.Getenv("AVAL_DECK_E2E") == "" {
		t.Skip("set AVAL_DECK_E2E=1 to print with a real browser")
	}
	browser, err := FindBrowser(OSEnv(runtime.GOOS))
	if err != nil {
		t.Skip(err)
	}
	out := os.Getenv("AVAL_DECK_E2E_OUT")
	if out == "" {
		out = filepath.Join(t.TempDir(), "every-layout.pdf")
	}
	if err := Build(everyLayout(), out, BuildOptions{Browser: browser}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	pdffonts, err := exec.LookPath("pdffonts")
	if err != nil {
		t.Log("pdffonts not installed: the fonts in the PDF are not checked")
		return
	}
	listed, err := exec.Command(pdffonts, out).Output()
	if err != nil {
		t.Fatalf("pdffonts: %v", err)
	}
	for _, family := range []string{"SpaceGrotesk", "IBMPlexSans"} {
		if !strings.Contains(strings.ReplaceAll(string(listed), " ", ""), family) {
			t.Errorf("the PDF does not embed %s:\n%s", family, listed)
		}
	}
}

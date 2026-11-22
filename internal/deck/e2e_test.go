package deck

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestBuildRealBrowser prints every layout with the machine's browser. Opt-in
// (AVAL_DECK_E2E=1): it needs a Chromium-based browser and, for the brand fonts,
// network. AVAL_DECK_E2E_OUT keeps the PDF for a visual check.
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
}

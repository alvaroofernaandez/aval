package main

import (
	"io"
	"os"
	"runtime"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"

	"github.com/alvaroofernaandez/aval/internal/ui"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
	"github.com/alvaroofernaandez/aval/internal/ui/cli"
)

// newPrinterFor styles status cells only when w is a terminal. Anything else
// (a pipe, a file, a test buffer) gets the plain printer, so scripts keep
// byte-stable output. On a terminal it follows the resolved UI settings.
func newPrinterFor(w io.Writer, s uiSettings) *cli.Printer {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(f.Fd()) {
		return cli.Plain()
	}
	width, height, _ := term.GetSize(f.Fd())
	c := caps.Detect(s.capsInputs(os.Getenv, runtime.GOOS, true, width, height))
	r := lipgloss.NewRenderer(f)
	r.SetColorProfile(c.Profile)
	th, err := ui.NewTheme(themeOrDefault(s.Theme), c, r)
	if err != nil {
		return cli.Plain()
	}
	return cli.New(c, th)
}

// themeOrDefault maps the zero value of uiSettings.Theme to the default theme.
func themeOrDefault(name string) string {
	if name == "" {
		return ui.DefaultThemeName
	}
	return name
}

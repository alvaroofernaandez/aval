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

// newPrinter styles status cells only when w is a terminal. Anything else
// (a pipe, a file, a test buffer) gets the plain printer, so scripts keep
// byte-stable output.
func newPrinter(w io.Writer) *cli.Printer {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(f.Fd()) {
		return cli.Plain()
	}
	width, height, _ := term.GetSize(f.Fd())
	c := caps.Detect(caps.Inputs{
		Getenv: os.Getenv, GOOS: runtime.GOOS, IsTTY: true,
		Width: width, Height: height, Glyphs: "auto", Motion: "auto", Background: "auto",
	})
	r := lipgloss.NewRenderer(f)
	r.SetColorProfile(c.Profile)
	th, err := ui.NewTheme("aval", c, r)
	if err != nil {
		return cli.Plain()
	}
	return cli.New(c, th)
}

package deck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Runner starts a program and returns its standard output.
type Runner func(name string, args ...string) ([]byte, error)

// browserTimeout bounds one browser run; a deck prints in about a second.
const browserTimeout = 90 * time.Second

// ExecRunner runs the real program. Chromium leaves helper processes (crash
// handler) holding stdout and stderr open after the browser exits, so without
// WaitDelay the read never ends; the context bounds a browser that hangs.
func ExecRunner(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), browserTimeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = &stderr
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil // the browser exited; only its helpers kept the pipes open
	}
	if ctx.Err() != nil {
		return out, fmt.Errorf("%s did not finish within %s", filepath.Base(name), browserTimeout)
	}
	if err != nil {
		return out, fmt.Errorf("%s: %w: %s", filepath.Base(name), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Env is what browser lookup reads from the machine, injectable for tests.
type Env struct {
	Getenv   func(string) string
	GOOS     string
	Exists   func(string) bool
	LookPath func(string) (string, error)
}

// OSEnv is the real machine.
func OSEnv(goos string) Env {
	return Env{
		Getenv: os.Getenv,
		GOOS:   goos,
		Exists: func(p string) bool {
			st, err := os.Stat(p)
			return err == nil && !st.IsDir()
		},
		LookPath: exec.LookPath,
	}
}

var macApps = []string{
	"Google Chrome.app/Contents/MacOS/Google Chrome",
	"Chromium.app/Contents/MacOS/Chromium",
	"Brave Browser.app/Contents/MacOS/Brave Browser",
	"Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
}

var unixNames = []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "brave-browser", "microsoft-edge"}

// FindBrowser returns a Chromium-based browser able to print PDFs headless:
// $AVAL_CHROME, then Chrome, Chromium, Brave and Edge in their usual places.
// aval ships no browser and needs no Node: any of these is enough.
func FindBrowser(e Env) (string, error) {
	if p := e.Getenv("AVAL_CHROME"); p != "" {
		if e.Exists(p) {
			return p, nil
		}
		return "", fmt.Errorf("AVAL_CHROME points at %s, which does not exist", p)
	}
	var tried []string
	try := func(p string) bool {
		tried = append(tried, p)
		return e.Exists(p)
	}
	switch e.GOOS {
	case "darwin":
		dirs := []string{"/Applications"}
		if h := e.Getenv("HOME"); h != "" {
			dirs = append(dirs, filepath.Join(h, "Applications"))
		}
		for _, d := range dirs {
			for _, a := range macApps {
				if p := filepath.Join(d, a); try(p) {
					return p, nil
				}
			}
		}
	case "windows":
		for _, base := range []string{e.Getenv("ProgramFiles"), e.Getenv("ProgramFiles(x86)"), e.Getenv("LOCALAPPDATA")} {
			if base == "" {
				continue
			}
			for _, rel := range [][]string{
				{"Google", "Chrome", "Application", "chrome.exe"},
				{"Microsoft", "Edge", "Application", "msedge.exe"},
				{"BraveSoftware", "Brave-Browser", "Application", "brave.exe"},
			} {
				if p := filepath.Join(append([]string{base}, rel...)...); try(p) {
					return p, nil
				}
			}
		}
	}
	for _, n := range unixNames {
		tried = append(tried, n+" on PATH")
		if p, err := e.LookPath(n); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no Chromium-based browser found to print the PDF (tried %d places); install Chrome, Brave, Edge or Chromium, or set AVAL_CHROME to its executable", len(tried))
}

var overflowAttr = regexp.MustCompile(`data-overflow="([^"]*)"`)

// ParseOverflow reads the slide numbers the page marked as overflowing. known is
// false when the check never finished (no attribute, or still "pending").
func ParseOverflow(dom string) (slides []string, known bool) {
	m := overflowAttr.FindStringSubmatch(dom)
	if m == nil || m[1] == "pending" {
		return nil, false
	}
	if m[1] == "" {
		return nil, true
	}
	return strings.Split(m[1], ","), true
}

// BuildOptions configures Build.
type BuildOptions struct {
	Browser string // executable; FindBrowser's result
	Run     Runner // nil means ExecRunner
	TempDir string // where the HTML and the browser profile live; "" means os.TempDir
}

// Build validates the deck, checks in the browser that no slide overflows its
// margins, and prints the PDF to pdf. Nothing is written to pdf unless every
// step passed.
func Build(d Deck, pdf string, o BuildOptions) error {
	if err := d.Validate(); err != nil {
		return err
	}
	html, err := Render(d)
	if err != nil {
		return err
	}
	run := o.Run
	if run == nil {
		run = ExecRunner
	}
	work, err := os.MkdirTemp(o.TempDir, "aval-deck-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	page := filepath.Join(work, "deck.html")
	if err := os.WriteFile(page, []byte(html), 0o644); err != nil {
		return err
	}
	pageURL := fileURL(page)
	// No --user-data-dir: headless=new already runs on a throwaway profile, and a
	// fresh --user-data-dir makes Chrome, Chrome for Testing and Brave hang on their
	// first launch for ~30 s (measured on macOS, 2026-11-22).
	common := []string{
		"--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--hide-scrollbars", "--virtual-time-budget=5000",
	}

	// No --window-size: with it, headless Brave never returns from --dump-dom
	// (measured on Brave, macOS). Slides have a fixed 1920px width and the
	// check measures relative to each slide, so the viewport does not matter.
	dom, err := run(o.Browser, append(append([]string{}, common...), "--dump-dom", pageURL)...)
	if err != nil {
		return fmt.Errorf("checking the layout: %w", err)
	}
	bad, known := ParseOverflow(string(dom))
	if !known {
		return errors.New("the layout check did not finish: the browser returned no overflow report")
	}
	if len(bad) > 0 {
		return fmt.Errorf("slides %s overflow their margins: shorten the text or split the slide", strings.Join(bad, ", "))
	}

	tmpPDF := filepath.Join(work, "deck.pdf")
	if _, err := run(o.Browser, append(append([]string{}, common...), "--no-pdf-header-footer", "--print-to-pdf="+tmpPDF, pageURL)...); err != nil {
		return fmt.Errorf("printing the PDF: %w", err)
	}
	b, err := os.ReadFile(tmpPDF)
	if err != nil || !bytes.HasPrefix(b, []byte("%PDF")) {
		return errors.New("the browser did not produce a PDF")
	}
	if err := os.MkdirAll(filepath.Dir(pdf), 0o755); err != nil {
		return err
	}
	return os.WriteFile(pdf, b, 0o644)
}

// fileURL turns an absolute path into a file URL; a Windows drive path gets
// the leading slash file URLs need (file:///C:/...).
func fileURL(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

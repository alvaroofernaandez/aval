package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/deck"
)

const deckUsage = `usage:
  aval deck build <spec.json> -o <out.pdf> [--plan <odd/tasks/plan.md>] [--open]
  aval deck check <spec.json>     validate the spec (layouts, required fields, text limits)
  aval deck example               print a complete spec that uses every layout

layouts:
`

// deckDeps is what the command needs from the machine, injectable for tests.
type deckDeps struct {
	browser func() (string, error)
	run     deck.Runner
	open    func(path string) error
}

func realDeckDeps() deckDeps {
	return deckDeps{
		browser: func() (string, error) { return deck.FindBrowser(deck.OSEnv(runtime.GOOS)) },
		run:     deck.ExecRunner,
		open:    openFile,
	}
}

func usageErr() error { return errors.New(deckUsage + deck.Help()) }

func runDeck(args []string, out io.Writer, d deckDeps) error {
	if len(args) == 0 {
		return usageErr()
	}
	switch args[0] {
	case "example":
		fmt.Fprint(out, deck.Example())
		return nil
	case "check":
		if len(args) != 2 {
			return usageErr()
		}
		if _, err := readSpec(args[1]); err != nil {
			return err
		}
		fmt.Fprintln(out, "deck: spec is valid")
		return nil
	case "build":
		return deckBuild(args[1:], out, d)
	}
	return usageErr()
}

func readSpec(path string) (deck.Deck, error) {
	f, err := os.Open(path)
	if err != nil {
		return deck.Deck{}, err
	}
	defer f.Close()
	return deck.Parse(f)
}

func deckBuild(args []string, out io.Writer, d deckDeps) error {
	var spec, pdf string
	open := false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-o" || a == "--out":
			if i+1 >= len(args) {
				return usageErr()
			}
			i++
			pdf = args[i]
		case a == "--open":
			open = true
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %s\n%s", a, deckUsage)
		case spec == "":
			spec = a
		default:
			return usageErr()
		}
	}
	if spec == "" {
		return usageErr()
	}
	if pdf == "" {
		return errors.New("aval deck build needs -o <out.pdf>: where the deck is saved")
	}
	if !strings.EqualFold(filepath.Ext(pdf), ".pdf") {
		return fmt.Errorf("the output must be a .pdf file, got %s", pdf)
	}
	pdf, err := filepath.Abs(pdf)
	if err != nil {
		return err
	}

	dk, err := readSpec(spec)
	if err != nil {
		return err
	}
	browser, err := d.browser()
	if err != nil {
		return err
	}
	if err := deck.Build(dk, pdf, deck.BuildOptions{Browser: browser, Run: d.run}); err != nil {
		return err
	}

	// Keep the spec beside the PDF: it is the editable source of the deck.
	keep := strings.TrimSuffix(pdf, filepath.Ext(pdf)) + ".deck.json"
	if src, _ := filepath.Abs(spec); src != keep {
		b, err := os.ReadFile(spec)
		if err != nil {
			return err
		}
		if err := os.WriteFile(keep, b, 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "deck: %d slides → %s\n      source: %s\n", len(dk.Slides), pdf, keep)

	if open {
		if err := d.open(pdf); err != nil {
			fmt.Fprintf(out, "deck: could not open the PDF: %v\n", err)
		}
	}
	return nil
}

// openFile shows a file with the system's default application.
func openFile(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

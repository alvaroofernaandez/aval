package ui

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// goldenTheme builds the deterministic theme every golden uses: a renderer on
// io.Discard with the Ascii colour profile, so no colour sequence is emitted
// and output is identical on every OS. unicode picks the glyph tier.
func goldenTheme(t *testing.T, unicode bool) Theme {
	t.Helper()
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.Ascii)
	th, err := NewTheme(DefaultThemeName, caps.Caps{Profile: termenv.Ascii, Unicode: unicode, Dark: true}, r)
	if err != nil {
		t.Fatal(err)
	}
	return th
}

// assertGolden compares got with testdata/<name>.golden. With -update it
// rewrites the file instead. CRLF is normalised on both sides.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	norm := func(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
	path := filepath.Join("testdata", name+".golden")
	got = norm(got)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update): %v", path, err)
	}
	if norm(string(want)) != got {
		t.Errorf("view differs from %s (run with -update to accept)\n--- want\n%s\n--- got\n%s", path, want, got)
	}
}

// renderAt drives m with a window size and returns its View.
func renderAt(m Model, width int) string {
	return renderAtSize(m, width, 40)
}

// renderAtSize drives m with a full window size and returns its View.
func renderAtSize(m Model, width, height int) string {
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Model).View()
}

// screenFixture builds a model for one screen with deliberately long labels,
// details, reasons and errors, so width bugs surface. Nothing touches disk.
func screenFixture(t *testing.T, screen Screen, unicode bool) Model {
	t.Helper()
	long := "~/.config/some/very/long/destination/path/that/keeps/going/config.toml"
	acts := func(labels ...string) []plan.Action {
		var as []plan.Action
		for _, l := range labels {
			as = append(as, plan.Action{Label: l})
		}
		return as
	}
	p := &plan.Plan{Groups: []plan.Group{
		{ID: "term", Label: "Terminal and shell environment", Detail: "tmux, zsh and the prompt, wired for this machine", Selected: true, Actions: acts("~/.tmux.conf", "~/.zshrc")},
		{ID: "edit", Label: "Editor", Detail: "LazyVim with the aval colour scheme", Selected: false, Actions: acts("~/.config/nvim")},
	}}
	r := install.New(install.Options{Home: "/home/u", BackupRoot: "/home/u/.aval-backup/20260101-000000", Symlinks: true})
	m := NewModel(p, r, Options{
		Version: "0.2.0", Platform: "macOS (arm64)", Symlinks: true,
		// The path does not exist: settings open on the defaults.
		Settings: SettingsEnv{
			Path: "/home/u/.config/aval/config", Home: "/home/u",
			Getenv: func(k string) string {
				if k == "AVAL_REPO" {
					return "/home/u/.aval"
				}
				return ""
			},
		},
	})
	m = m.WithTheme(goldenTheme(t, unicode))
	m.Unavailable = []UnavailableGroup{{Label: "Windows Terminal profile", Reason: "only offered on Windows, this machine runs macOS"}}

	switch screen {
	case ScreenInstall:
		m.screen = ScreenInstall
		m.queue = acts("~/.tmux.conf", long, "~/.zshrc", "~/.config/ghostty", "~/.config/nvim", "~/.gitconfig")
		m = m.record(install.Result{Action: m.queue[0], Status: install.StatusLinked}, nil)
		m = m.record(install.Result{Action: m.queue[1], Status: install.StatusAlready}, nil)
		m = m.record(install.Result{Action: m.queue[2], Status: install.StatusLinked, Backup: "/home/u/.aval-backup/20260101-000000/.zshrc"}, nil)
		m = m.record(install.Result{Action: m.queue[3]}, &install.Error{Action: m.queue[3], Reason: "source missing"})
	case ScreenDone:
		m.screen = ScreenDone
		m.results = []install.Result{
			{Action: plan.Action{Label: "~/.tmux.conf"}, Status: install.StatusLinked},
			{Action: plan.Action{Label: "~/.zshrc"}, Status: install.StatusLinked, Backup: "/home/u/.aval-backup/20260101-000000/.zshrc"},
			{Action: plan.Action{Label: "~/.gitconfig"}, Status: install.StatusAlready},
		}
		m.errs = []error{fmt.Errorf("link %s: permission denied while writing the destination", long)}
	case ScreenSettings:
		m = send(m, "s")
	}
	return m
}

var goldenScreens = []struct {
	name   string
	screen Screen
}{
	{"select", ScreenSelect},
	{"install", ScreenInstall},
	{"done", ScreenDone},
	{"settings", ScreenSettings},
}

func TestViewsNeverExceedTheWidth(t *testing.T) {
	for _, unicode := range []bool{true, false} {
		for _, s := range goldenScreens {
			m := screenFixture(t, s.screen, unicode)
			for w := 20; w <= 120; w++ {
				for i, line := range strings.Split(renderAt(m, w), "\n") {
					if got := lipgloss.Width(line); got > w {
						t.Errorf("%s unicode=%v width %d: line %d is %d wide: %q", s.name, unicode, w, i, got, line)
						break
					}
				}
			}
		}
	}
}

func TestGoldenViews(t *testing.T) {
	for _, tier := range []struct {
		name    string
		unicode bool
	}{{"unicode", true}, {"ascii", false}} {
		for _, s := range goldenScreens {
			for _, w := range []int{36, 80, 120} {
				name := fmt.Sprintf("%s_%s_%d", s.name, tier.name, w)
				t.Run(name, func(t *testing.T) {
					assertGolden(t, name, renderAt(screenFixture(t, s.screen, tier.unicode), w))
				})
			}
		}
	}
}

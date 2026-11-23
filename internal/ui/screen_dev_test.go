package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/devmode"
)

const noBackupLine = "There is no backup: a sandbox is disposable. Your real home is not touched."

// devFake records what the dev view asks of its services.
type devFake struct {
	cmds    []*exec.Cmd
	calls   [][2]string
	removed []string
	rmErr   error
}

func (f *devFake) services() Services {
	return Services{
		tr:    testTr,
		Home:  "/home/u",
		Repo:  "/home/u/.aval",
		Build: BuildInfo{Version: "0.3.0"},
		Sandboxes: func() ([]devmode.Info, error) {
			all := []devmode.Info{
				{Name: "default", Root: "/home/u/.cache/aval/dev/default", Ready: true},
				{Name: "release", Root: "/home/u/.cache/aval/dev/release"},
			}
			var out []devmode.Info
			for _, i := range all {
				gone := false
				for _, r := range f.removed {
					gone = gone || r == i.Name
				}
				if !gone {
					out = append(out, i)
				}
			}
			return out, nil
		},
		DevCommand: func(sub, name string) *exec.Cmd {
			f.calls = append(f.calls, [2]string{sub, name})
			args := []string{"dev"}
			if sub != "" {
				args = append(args, sub)
			}
			c := exec.Command("aval", append(args, "--name", name)...)
			f.cmds = append(f.cmds, c)
			return c
		},
		RemoveSandbox: func(name string) error {
			if f.rmErr != nil {
				return f.rmErr
			}
			f.removed = append(f.removed, name)
			return nil
		},
	}
}

func devApp(t *testing.T, unicode bool, svc Services) App {
	t.Helper()
	c := chrome{tr: testTr, theme: goldenTheme(t, unicode), version: svc.Build.Version, platform: "macOS (arm64)", width: 120, height: 30}
	a := newApp(c, svc, newDev(svc))
	for _, m := range drainCmd(a.Init()) {
		a, _ = routerStep(a, m)
	}
	return a
}

func devPress(t *testing.T, a App, keys ...string) App {
	t.Helper()
	for _, k := range keys {
		msg := key(k)
		switch k {
		case "esc":
			msg = escKey()
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		}
		a, _ = routerStep(a, msg)
	}
	return a
}

func devTyped(t *testing.T, a App, s string) App {
	t.Helper()
	for _, r := range s {
		a = devPress(t, a, string(r))
	}
	return a
}

func devFlat(s string) string { return strings.Join(strings.Fields(s), " ") }

func TestDevEnterReturnsExecProcessForTheSandboxShell(t *testing.T) {
	f := &devFake{}
	a := devApp(t, true, f.services())
	_, cmd := a.Update(key("enter"))
	if cmd == nil {
		t.Fatal("enter returned no command")
	}
	if got := fmt.Sprintf("%T", cmd()); got != "tea.execMsg" {
		t.Errorf("enter produced %s, want tea.execMsg from tea.ExecProcess", got)
	}
	if len(f.calls) != 1 || f.calls[0] != [2]string{"", "default"} {
		t.Fatalf("DevCommand calls = %v, want [{ default}]", f.calls)
	}
	if got := strings.Join(f.cmds[0].Args, " "); got != "aval dev --name default" {
		t.Errorf("command = %q, want `aval dev --name default`", got)
	}
}

func TestDevInvalidNameStaysInTheFieldWithTheValidatorMessage(t *testing.T) {
	f := &devFake{}
	a := devTyped(t, devPress(t, devApp(t, true, f.services()), "n"), "-x")
	a = devPress(t, a, "enter")
	if !a.typing() {
		t.Fatal("the field closed on an invalid name")
	}
	if len(f.calls) != 0 {
		t.Errorf("an invalid name reached DevCommand: %v", f.calls)
	}
	out := devFlat(a.View())
	want := devFlat(devmode.ValidName("-x").Error())
	if !strings.Contains(out, want) {
		t.Errorf("view lacks the validator message %q:\n%s", want, a.View())
	}
	if !strings.Contains(out, "-x") {
		t.Errorf("the typed name left the field:\n%s", a.View())
	}
}

func TestDevValidNameHandsOverToTheShell(t *testing.T) {
	f := &devFake{}
	a := devTyped(t, devPress(t, devApp(t, true, f.services()), "n"), "fresh")
	_, cmd := a.Update(key("enter"))
	if cmd == nil || fmt.Sprintf("%T", cmd()) != "tea.execMsg" {
		t.Fatalf("enter on a valid name did not return ExecProcess")
	}
	if len(f.calls) != 1 || f.calls[0] != [2]string{"", "fresh"} {
		t.Errorf("DevCommand calls = %v, want [{ fresh}]", f.calls)
	}
}

func TestDevTextFieldSwallowsQuitAndHelp(t *testing.T) {
	a := devPress(t, devApp(t, true, (&devFake{}).services()), "n")
	for _, k := range []string{"q", "?"} {
		var quit bool
		a, quit = routerStep(a, key(k))
		if quit || a.Quitting() {
			t.Fatalf("%q quit the program from inside the field", k)
		}
		if a.help {
			t.Fatalf("%q opened help from inside the field", k)
		}
	}
	if !strings.Contains(a.View(), "q?") {
		t.Errorf("q and ? were not typed into the field:\n%s", a.View())
	}
	a = devPress(t, a, "esc")
	if a.typing() || a.depth() != 1 {
		t.Errorf("esc should close the field and stay on the view (typing=%v depth=%d)", a.typing(), a.depth())
	}
}

func TestDevRemoveConfirmsOnCancelAndSaysThereIsNoBackup(t *testing.T) {
	f := &devFake{}
	a := devPress(t, devApp(t, true, f.services()), "d")
	out := a.View()
	if !strings.Contains(out, "› Cancel") {
		t.Errorf("the confirm does not open on Cancel:\n%s", out)
	}
	if !strings.Contains(out, noBackupLine) {
		t.Errorf("the confirm lacks the safety line:\n%s", out)
	}
	a = devPress(t, a, "enter")
	if len(f.removed) != 0 {
		t.Fatalf("enter on Cancel removed %v", f.removed)
	}
	a = devPress(t, a, "d", "up", "enter")
	if len(f.removed) != 1 || f.removed[0] != "default" {
		t.Fatalf("removed = %v, want [default]", f.removed)
	}
	if strings.Contains(a.View(), "default") && strings.Contains(a.View(), "ready") {
		t.Errorf("the removed sandbox is still listed:\n%s", a.View())
	}
}

func TestDevRemoveFailureIsShown(t *testing.T) {
	f := &devFake{rmErr: errors.New("permission denied")}
	a := devPress(t, devApp(t, true, f.services()), "d", "up", "enter")
	if out := a.View(); !strings.Contains(out, "permission denied") {
		t.Errorf("a failed removal says nothing:\n%s", out)
	}
}

func TestDevResetConfirmsThenExecsReset(t *testing.T) {
	f := &devFake{}
	a := devPress(t, devApp(t, true, f.services()), "r")
	if out := a.View(); !strings.Contains(out, noBackupLine) || !strings.Contains(out, "› Cancel") {
		t.Errorf("reset confirm is not destructive:\n%s", out)
	}
	a = devPress(t, a, "enter")
	if len(f.calls) != 0 {
		t.Fatalf("Cancel ran %v", f.calls)
	}
	a = devPress(t, a, "r", "up")
	_, cmd := a.Update(key("enter"))
	if cmd == nil || fmt.Sprintf("%T", cmd()) != "tea.execMsg" {
		t.Fatal("confirming reset did not return ExecProcess")
	}
	if len(f.calls) != 1 || f.calls[0] != [2]string{"reset", "default"} {
		t.Errorf("DevCommand calls = %v, want [{reset default}]", f.calls)
	}
}

func TestDevInsideASandboxTheWritingKeysAreAbsent(t *testing.T) {
	f := &devFake{}
	svc := f.services()
	svc.Sandbox = "alpha"
	a := devApp(t, true, svc)
	if out := devFlat(a.View()); !strings.Contains(out, "You are inside sandbox alpha. Exit it to manage sandboxes.") {
		t.Errorf("the read-only notice is missing:\n%s", a.View())
	}
	for _, k := range []string{"enter", "n", "r", "d"} {
		if _, ok := a.keys().lookup(key(k).String()); ok {
			t.Errorf("writing key %q is in the key map inside a sandbox", k)
		}
		next, cmd := a.Update(key(k))
		if cmd != nil || next.(App).typing() || next.(App).View() != a.View() {
			t.Errorf("writing key %q acts inside a sandbox", k)
		}
	}
	if len(f.calls) != 0 || len(f.removed) != 0 {
		t.Errorf("services were called: %v %v", f.calls, f.removed)
	}
	if out := a.View(); !strings.Contains(out, "default") || !strings.Contains(out, "release") {
		t.Errorf("the list is not shown read-only:\n%s", out)
	}
}

func TestDevStatusLineSaysEnterLeavesTheScreen(t *testing.T) {
	a := devApp(t, true, (&devFake{}).services())
	want := "Enter leaves this screen for a shell in the sandbox; exit brings you back"
	if out := a.View(); !strings.Contains(out, want) {
		t.Errorf("status line missing %q:\n%s", want, out)
	}
}

func TestDevListShowsNameStateAndPath(t *testing.T) {
	out := devApp(t, true, (&devFake{}).services()).View()
	for _, want := range []string{"default", "ready", "~/.cache/aval/dev/default", "release", "incomplete", "~/.cache/aval/dev/release"} {
		if !strings.Contains(out, want) {
			t.Errorf("list lacks %q:\n%s", want, out)
		}
	}
}

// probe hosts an App in a real tea.Program: it presses enter at start and
// quits once the dev view has heard how the sandbox shell ended.
type probe struct{ App }

func (p probe) Init() tea.Cmd {
	return tea.Batch(p.App.Init(), func() tea.Msg { return key("enter") })
}

func (p probe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := p.App.Update(msg)
	p.App = next.(App)
	if g, ok := msg.(genMsg); ok {
		if _, done := g.msg.(devExitMsg); done {
			return p, tea.Sequence(cmd, tea.Quit)
		}
	}
	return p, cmd
}

// TestDevNonZeroExitShowsStderrThroughRealExecProcess runs the whole path in
// a real tea.Program: the command's stderr must reach the failure lines, so
// a Stderr set before ExecProcess survives bubbletea 1.3.10 (SetStderr only
// fills a nil one).
func TestDevNonZeroExitShowsStderrThroughRealExecProcess(t *testing.T) {
	f := &devFake{}
	svc := f.services()
	svc.DevCommand = func(sub, name string) *exec.Cmd {
		return exec.Command("sh", "-c", "echo first >&2; echo 'no space left' >&2; exit 3")
	}
	c := chrome{tr: testTr, theme: goldenTheme(t, true), version: "0.3.0", platform: "macOS (arm64)", width: 120, height: 30}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	p := tea.NewProgram(probe{newApp(c, svc, newDev(svc))}, tea.WithContext(ctx), tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program: %v", err)
	}
	out := final.View()
	for _, want := range []string{"aval dev failed", "no space left"} {
		if !strings.Contains(out, want) {
			t.Errorf("failure lacks %q:\n%s", want, out)
		}
	}
}

func TestDevNonZeroExitWithoutStderrFallsBackToTheStatus(t *testing.T) {
	f := &devFake{}
	svc := f.services()
	svc.DevCommand = func(sub, name string) *exec.Cmd { return exec.Command("sh", "-c", "exit 4") }
	c := chrome{tr: testTr, theme: goldenTheme(t, true), version: "0.3.0", platform: "macOS (arm64)", width: 120, height: 30}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	p := tea.NewProgram(probe{newApp(c, svc, newDev(svc))}, tea.WithContext(ctx), tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program: %v", err)
	}
	if out := final.View(); !strings.Contains(out, "aval dev failed") || !strings.Contains(out, "exit status 4") {
		t.Errorf("failure lacks the exit status:\n%s", out)
	}
}

func init() {
	goldenViews = append(goldenViews,
		struct {
			name  string
			build func(t *testing.T, unicode bool) tea.Model
		}{"dev", func(t *testing.T, unicode bool) tea.Model { return devApp(t, unicode, (&devFake{}).services()) }},
		struct {
			name  string
			build func(t *testing.T, unicode bool) tea.Model
		}{"dev_sandboxed", func(t *testing.T, unicode bool) tea.Model {
			svc := (&devFake{}).services()
			svc.Sandbox = "alpha"
			return devApp(t, unicode, svc)
		}},
		struct {
			name  string
			build func(t *testing.T, unicode bool) tea.Model
		}{"dev_remove", func(t *testing.T, unicode bool) tea.Model {
			return devPress(t, devApp(t, unicode, (&devFake{}).services()), "d")
		}},
	)
}

// The open name field lives outside goldenViews: its invariants (every key
// is in the map) do not hold while keys are typed as text, by design.
func TestDevNameFieldGoldens(t *testing.T) {
	for _, tier := range []struct {
		name    string
		unicode bool
	}{{"unicode", true}, {"ascii", false}} {
		for _, w := range []int{36, 80, 120} {
			name := fmt.Sprintf("view_dev_new_invalid_%s_%d", tier.name, w)
			t.Run(name, func(t *testing.T) {
				a := devTyped(t, devPress(t, devApp(t, tier.unicode, (&devFake{}).services()), "n"), "-x")
				assertGolden(t, name, renderModelAt(devPress(t, a, "enter"), w, 40))
			})
		}
	}
}

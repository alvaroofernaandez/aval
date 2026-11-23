package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// fakeCloner records every call; it makes dest a checkout unless err is set.
type fakeCloner struct {
	calls []string
	err   error
}

func (f *fakeCloner) Clone(dest string) error {
	f.calls = append(f.calls, dest)
	if f.err != nil {
		return f.err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, "install.manifest"), []byte("x\n"), 0o644)
}

// checkoutDir makes a directory that repoloc accepts.
func checkoutDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "install.manifest"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return d
}

type welcomeFixture struct {
	m        Model
	cloner   *fakeCloner
	prepared []string
	home     string
}

// newWelcomeFixture builds the welcome screen over a temp home. Prepare
// records the repo and returns a one-group plan.
func newWelcomeFixture(t *testing.T, unicode, motion bool, candidate string) *welcomeFixture {
	t.Helper()
	f := &welcomeFixture{cloner: &fakeCloner{}, home: t.TempDir()}
	env := WelcomeEnv{
		Home:      f.home,
		CloneDest: filepath.Join(f.home, ".aval"),
		CloneURL:  "https://github.com/alvaroofernaandez/aval.git",
		Cloner:    f.cloner,
		Candidate: candidate,
		Prepare: func(repo string) (Prepared, error) {
			f.prepared = append(f.prepared, repo)
			return Prepared{Plan: &plan.Plan{Groups: []plan.Group{
				{ID: "one", Label: "Group One", Selected: true, Actions: []plan.Action{{Label: "~/.a"}}},
			}}, Symlinks: true}, nil
		},
	}
	m := NewWelcome(Options{Version: "0.2.0", Platform: "macOS (arm64)", Caps: caps.Caps{Motion: motion}}, env)
	f.m = m.WithTheme(goldenTheme(t, unicode))
	return f
}

func (f *welcomeFixture) send(msg tea.Msg) tea.Cmd {
	next, cmd := f.m.Update(msg)
	f.m = next.(Model)
	return cmd
}

func (f *welcomeFixture) key(k string) tea.Cmd { return f.send(key(k)) }

func (f *welcomeFixture) typeText(s string) {
	f.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
}

// runCmd executes cmd and every command in a batch, feeding back each
// message that is not a spinner tick. Ticks are returned, not fed.
func (f *welcomeFixture) runCmd(cmd tea.Cmd) (ticks int) {
	if cmd == nil {
		return 0
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			ticks += f.runCmd(c)
		}
	case spinTickMsg:
		return 1
	case nil:
	default:
		f.runCmd(f.send(msg))
	}
	return ticks
}

func (f *welcomeFixture) view() string { return renderAtSize(f.m, 100, 30) }

func TestWelcomeAsksWhereTheRepositoryIsWithTwoChoices(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	if f.m.Screen() != ScreenWelcome {
		t.Fatalf("NewWelcome opened screen %v", f.m.Screen())
	}
	v := f.view()
	for _, want := range []string{
		"Where is the aval repository?",
		"Clone it to ~/.aval", "needs git and a network connection",
		"Use a folder I already have", "type or paste a path",
		"~/.aval-backup",
		"step 1 of 3 · repository",
		"? help",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("welcome does not show %q:\n%s", want, v)
		}
	}
	if n := strings.Count(v, "( )"); n != 2 {
		t.Errorf("welcome shows %d choices, want 2 (Hick):\n%s", n, v)
	}
}

func TestWelcomeOffersTheCheckoutFoundHereFirst(t *testing.T) {
	here := checkoutDir(t)
	f := newWelcomeFixture(t, true, false, here)
	v := f.view()
	if n := strings.Count(v, "( )"); n != 3 {
		t.Errorf("welcome shows %d choices with a checkout here, want 3:\n%s", n, v)
	}
	first := strings.Index(v, "Use the checkout here")
	clone := strings.Index(v, "Clone it to ~/.aval")
	if first < 0 || first > clone {
		t.Errorf("the checkout here is not the first choice:\n%s", v)
	}
	f.key("enter")
	if f.m.Screen() != ScreenSelect || len(f.prepared) != 1 || f.prepared[0] != here {
		t.Errorf("enter on the checkout here: screen %v, prepared %v; want select with %s", f.m.Screen(), f.prepared, here)
	}
	if len(f.cloner.calls) != 0 {
		t.Errorf("choosing the checkout here cloned %v", f.cloner.calls)
	}
}

func TestWelcomeHelpListsItsKeys(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	f.key("?")
	v := f.view()
	if !strings.Contains(v, "Keys") || !strings.Contains(v, "choose") || !strings.Contains(v, "quit") {
		t.Errorf("? on welcome does not list its keys:\n%s", v)
	}
}

func TestWelcomeQuitLeavesNoRepo(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	f.key("q")
	if !f.m.Quitting || f.m.Repo() != "" || f.m.Completed() {
		t.Errorf("q on welcome: quitting=%v repo=%q completed=%v", f.m.Quitting, f.m.Repo(), f.m.Completed())
	}
}

// openPathField moves to "Use a folder I already have" and opens the field.
func (f *welcomeFixture) openPathField() {
	f.key("down")
	f.key("enter")
}

func TestWelcomeInvalidPathShowsTheErrorInline(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	f.openPathField()
	missing := filepath.Join(f.home, "nope")
	f.typeText(missing)
	f.key("enter")
	v := f.view()
	if f.m.Screen() != ScreenWelcome {
		t.Fatalf("an invalid path left the welcome screen for %v", f.m.Screen())
	}
	if !strings.Contains(v, "does not exist") || !strings.Contains(v, "~/nope") {
		t.Errorf("the missing path error is not inline:\n%s", v)
	}
	if len(f.prepared) != 0 {
		t.Errorf("an invalid path reached Prepare: %v", f.prepared)
	}

	// Fix it in place: clear the field and type a folder with no manifest.
	for range []rune(missing) {
		f.send(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	f.typeText(f.home)
	f.key("enter")
	v = f.view()
	if !strings.Contains(v, "not an aval repository") || !strings.Contains(v, "install.manifest") {
		t.Errorf("the no-manifest error is not inline:\n%s", v)
	}
}

func TestWelcomeTypedPathAcceptsLettersBoundToKeys(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	f.openPathField()
	f.typeText("q")
	f.key("j")
	if f.m.Quitting {
		t.Fatal("typing q in the path field quit")
	}
	if v := f.view(); !strings.Contains(v, "qj") {
		t.Errorf("the field does not hold the typed text:\n%s", v)
	}
	f.send(tea.KeyMsg{Type: tea.KeyEsc})
	if v := f.view(); !strings.Contains(v, "Where is the aval repository?") || strings.Contains(v, "qj") {
		t.Errorf("esc did not close the field:\n%s", v)
	}
}

func TestWelcomeValidPathProceedsToSelect(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	repo := checkoutDir(t)
	f.openPathField()
	f.typeText(repo)
	f.key("enter")
	if f.m.Screen() != ScreenSelect || f.m.Repo() != repo {
		t.Fatalf("screen %v repo %q, want select with %s", f.m.Screen(), f.m.Repo(), repo)
	}
	if v := f.view(); !strings.Contains(v, "Group One") {
		t.Errorf("select does not show the prepared plan:\n%s", v)
	}
}

func TestWelcomeTildePathExpandsAgainstHome(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	repo := filepath.Join(f.home, "code", "aval")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "install.manifest"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.openPathField()
	f.typeText("~/code/aval")
	f.key("enter")
	if f.m.Repo() != repo {
		t.Errorf("~/code/aval resolved to %q, want %s", f.m.Repo(), repo)
	}
}

func TestWelcomePrepareErrorStaysInline(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	f.m.welcome.env.Prepare = func(string) (Prepared, error) {
		return Prepared{}, errors.New("nothing in the manifest applies to macOS")
	}
	f.openPathField()
	f.typeText(checkoutDir(t))
	f.key("enter")
	if f.m.Screen() != ScreenWelcome || !strings.Contains(f.view(), "nothing in the manifest applies") {
		t.Errorf("a Prepare error is not shown on welcome:\n%s", f.view())
	}
}

func TestWelcomeCloneCallsTheClonerAndShowsTheSpinner(t *testing.T) {
	f := newWelcomeFixture(t, true, true, "")
	cmd := f.key("enter")
	if f.m.Screen() != ScreenWelcome {
		t.Fatalf("clone left welcome before it finished: %v", f.m.Screen())
	}
	v := f.view()
	if !strings.Contains(v, spinFrames[0]+" Cloning"+UnicodeGlyphs.Ellipsis) {
		t.Errorf("cloning with motion does not show the spinner:\n%s", v)
	}
	// The spinner advances on a tick while the clone is in flight.
	f.send(spinTickMsg{})
	if v := f.view(); !strings.Contains(v, spinFrames[1]+" Cloning") {
		t.Errorf("a tick did not advance the spinner:\n%s", v)
	}
	if ticks := f.runCmd(cmd); ticks == 0 {
		t.Error("cloning with motion scheduled no spinner tick")
	}
	if len(f.cloner.calls) != 1 || f.cloner.calls[0] != filepath.Join(f.home, ".aval") {
		t.Errorf("cloner calls = %v, want one into ~/.aval", f.cloner.calls)
	}
}

func TestWelcomeCloneWithMotionOffShowsStaticText(t *testing.T) {
	f := newWelcomeFixture(t, false, false, "")
	cmd := f.key("enter")
	v := f.view()
	if !strings.Contains(v, "Cloning...") {
		t.Errorf("cloning without motion does not say Cloning...:\n%s", v)
	}
	for _, fr := range spinFrames {
		if strings.Contains(v, fr+" Cloning") {
			t.Errorf("motion off still draws spinner frame %q:\n%s", fr, v)
		}
	}
	if next := f.send(spinTickMsg{}); next != nil || f.view() != v {
		t.Error("a tick changed the static clone text")
	}
	if ticks := f.runCmd(cmd); ticks != 0 {
		t.Errorf("motion off scheduled %d spinner ticks", ticks)
	}
	if len(f.cloner.calls) != 1 {
		t.Errorf("cloner calls = %v, want 1", f.cloner.calls)
	}
}

func TestWelcomeSuccessfulCloneProceedsToSelect(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	f.runCmd(f.key("enter"))
	dest := filepath.Join(f.home, ".aval")
	if f.m.Screen() != ScreenSelect || f.m.Repo() != dest {
		t.Fatalf("after a clone: screen %v repo %q, want select with %s", f.m.Screen(), f.m.Repo(), dest)
	}
	if len(f.prepared) != 1 || f.prepared[0] != dest {
		t.Errorf("prepared %v, want [%s]", f.prepared, dest)
	}
}

func TestWelcomeCloneFailureOffersRetryOrPath(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	f.cloner.err = errors.New("Could not resolve host: github.com")
	f.runCmd(f.key("enter"))
	v := f.view()
	if f.m.Screen() != ScreenWelcome {
		t.Fatalf("a failed clone left welcome for %v", f.m.Screen())
	}
	for _, want := range []string{"Could not resolve host: github.com", "Try the clone again", "Use a folder I already have"} {
		if !strings.Contains(v, want) {
			t.Errorf("a failed clone does not show %q:\n%s", want, v)
		}
	}
	if n := strings.Count(v, "( )"); n != 2 {
		t.Errorf("a failed clone offers %d choices, want retry and path entry:\n%s", n, v)
	}

	// Retry, this time it works.
	f.cloner.err = nil
	f.runCmd(f.key("enter"))
	if len(f.cloner.calls) != 2 || f.m.Screen() != ScreenSelect {
		t.Errorf("retry: %d clone calls, screen %v; want 2 and select", len(f.cloner.calls), f.m.Screen())
	}
}

func TestWelcomeCloneFailureCanSwitchToPathEntry(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	f.cloner.err = errors.New("git is not installed")
	f.runCmd(f.key("enter"))
	f.openPathField()
	repo := checkoutDir(t)
	f.typeText(repo)
	f.key("enter")
	if f.m.Screen() != ScreenSelect || f.m.Repo() != repo {
		t.Errorf("path entry after a failed clone: screen %v repo %q", f.m.Screen(), f.m.Repo())
	}
}

func TestWelcomeCloneUsesAnExistingCheckoutAtTheDestination(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	dest := filepath.Join(f.home, ".aval")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "install.manifest"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.runCmd(f.key("enter"))
	if len(f.cloner.calls) != 0 {
		t.Errorf("an existing checkout at ~/.aval was cloned again: %v", f.cloner.calls)
	}
	if f.m.Screen() != ScreenSelect || f.m.Repo() != dest {
		t.Errorf("screen %v repo %q, want select with %s", f.m.Screen(), f.m.Repo(), dest)
	}
}

func TestWelcomeCloneRefusesANonCheckoutAtTheDestination(t *testing.T) {
	f := newWelcomeFixture(t, true, false, "")
	if err := os.MkdirAll(filepath.Join(f.home, ".aval", "stuff"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.runCmd(f.key("enter"))
	if len(f.cloner.calls) != 0 {
		t.Errorf("cloned over an existing folder: %v", f.cloner.calls)
	}
	v := f.view()
	if f.m.Screen() != ScreenWelcome || !strings.Contains(v, "~/.aval") || !strings.Contains(v, "not an aval repository") {
		t.Errorf("a non-checkout at ~/.aval is not reported inline:\n%s", v)
	}
}

func TestWelcomeInlineErrorsFitEveryWidth(t *testing.T) {
	for _, unicode := range []bool{true, false} {
		f := newWelcomeFixture(t, unicode, false, "")
		f.openPathField()
		f.typeText(filepath.Join(f.home, "a/very/long/path/that/does/not/exist/anywhere/on/this/machine"))
		f.key("enter")
		for w := 20; w <= 120; w++ {
			for h := 8; h <= 30; h += 11 {
				lines := strings.Split(renderAtSize(f.m, w, h), "\n")
				if len(lines) > h {
					t.Errorf("unicode=%v %dx%d: %d lines", unicode, w, h, len(lines))
				}
				for i, line := range lines {
					if lw := lipgloss.Width(line); lw > w {
						t.Errorf("unicode=%v %dx%d: line %d is %d wide: %q", unicode, w, h, i, lw, line)
					}
				}
			}
		}
	}
}

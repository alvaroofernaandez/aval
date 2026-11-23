package main

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/core/platform"
	"github.com/alvaroofernaandez/aval/internal/ui"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
	"github.com/alvaroofernaandez/aval/internal/ui/cli"
)

// headlessFixture builds a plan with one action per outcome: linked, already
// correct, linked over an existing file (backed up), and a missing source.
func headlessFixture(t *testing.T, dryRun bool) (*plan.Plan, *install.Runner, string) {
	t.Helper()
	root := t.TempDir()
	repo, home := filepath.Join(root, "repo"), filepath.Join(root, "home")
	for _, d := range []string{repo, home} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var actions []plan.Action
	for _, n := range []string{"a", "b", "c"} {
		src := filepath.Join(repo, n)
		if err := os.WriteFile(src, []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, plan.Action{Source: src, Dest: filepath.Join(home, n), Label: "~/" + n})
	}
	if err := os.Symlink(actions[1].Source, actions[1].Dest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actions[2].Dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	actions = append(actions, plan.Action{Source: filepath.Join(repo, "gone"), Dest: filepath.Join(home, "gone"), Label: "~/gone"})
	p := &plan.Plan{Groups: []plan.Group{{ID: "g", Selected: true, Actions: actions}}}
	r := install.New(install.Options{Home: home, Symlinks: true, DryRun: dryRun, BackupRoot: filepath.Join(root, "bk")})
	return p, r, root
}

// captureStdout runs fn and returns what it wrote to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = wr
	done := make(chan string)
	go func() { b, _ := io.ReadAll(rd); done <- string(b) }()
	fn()
	wr.Close()
	os.Stdout = old
	return <-done
}

func TestRunHeadlessPlainOutputIsByteStable(t *testing.T) {
	pf := platform.Platform{OS: "darwin", Arch: "arm64"}
	cases := []struct {
		name   string
		dryRun bool
		want   string
	}{
		{"apply", false, "aval " + version + " on macOS (arm64)\n" +
			"symlinked: edits in the repo apply immediately\n\n" +
			"  ~/a                                          linked\n" +
			"  ~/b                                          already installed\n" +
			"  ~/c                                          linked  (backed up)\n" +
			"  ~/gone                                       FAILED: install: source missing: ROOT/repo/gone\n" +
			"\n1 existing file(s) moved to ROOT/bk\n"},
		{"dry run", true, "aval " + version + " on macOS (arm64)\n" +
			"symlinked: edits in the repo apply immediately\n\n" +
			"  ~/a                                          would install\n" +
			"  ~/b                                          already installed\n" +
			"  ~/c                                          would install\n" +
			"  ~/gone                                       FAILED: install: source missing: ROOT/repo/gone\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, r, root := headlessFixture(t, c.dryRun)
			var err error
			out := captureStdout(t, func() { err = runHeadless(p, pf, r, true, c.dryRun, uiSettings{}) })
			if err == nil || err.Error() != "1 action(s) failed" {
				t.Fatalf("err = %v", err)
			}
			got := strings.ReplaceAll(out, root, "ROOT")
			if got != c.want {
				t.Errorf("output changed.\n got: %q\nwant: %q", got, c.want)
			}
		})
	}
}

func TestRunHeadlessToStyledKeepsTheWords(t *testing.T) {
	pf := platform.Platform{OS: "darwin", Arch: "arm64"}
	p, r, root := headlessFixture(t, false)
	c := caps.Caps{Profile: termenv.TrueColor, Unicode: true, Dark: true, Width: 80, Height: 24}
	rn := lipgloss.NewRenderer(io.Discard)
	rn.SetColorProfile(c.Profile)
	th, err := ui.NewTheme("aval", c, rn)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	_ = runHeadlessTo(&b, cli.New(c, th), p, pf, r, true, false)
	out := b.String()
	if !strings.Contains(out, "\x1b[") {
		t.Error("styled output carries no colour")
	}
	stripped := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(out, "")
	for _, want := range []string{"√ linked", "√ already installed", "× FAILED: install: source missing: " + root + "/repo/gone", "(backed up)"} {
		if !strings.Contains(stripped, want) {
			t.Errorf("missing %q in:\n%s", want, stripped)
		}
	}
}

func TestRunHeadlessOutputHasNoEmDash(t *testing.T) {
	pf := platform.Platform{OS: "darwin", Arch: "arm64"}
	for _, dry := range []bool{false, true} {
		_, r, _ := headlessFixture(t, dry)
		var b strings.Builder
		if err := runHeadlessTo(&b, cli.Plain(), &plan.Plan{}, pf, r, true, dry); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(b.String(), "—") {
			t.Errorf("dryRun=%v: em dash in output:\n%s", dry, b.String())
		}
	}
}

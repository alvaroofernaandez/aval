package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alvaroofernaandez/aval/internal/devops/reaper"
	"github.com/alvaroofernaandez/aval/internal/term/macos"
)

const macosUsage = `usage: aval macos <command>
  defaults                          apply the macOS defaults from the habits audit
  screenshot-clipboard install      send every screenshot to the clipboard too (LaunchAgent)
  screenshot-clipboard copy [dir]   copy the newest screenshot to the clipboard, once`

// runMacos is `aval macos ...`. The LaunchAgent it installs calls back into
// `aval macos screenshot-clipboard copy`, so it needs no repository.
func runMacos(args []string, out io.Writer) error {
	run := func(name string, a ...string) error { return exec.Command(name, a...).Run() }
	home, _ := os.UserHomeDir()

	switch {
	case len(args) == 1 && args[0] == "defaults":
		if err := macos.ApplyDefaults(run); err != nil {
			return err
		}
		fmt.Fprintln(out, "macOS defaults applied (log out for key repeat)")
		return nil

	case len(args) >= 2 && args[0] == "screenshot-clipboard" && args[1] == "install":
		err := macos.InstallClipboardAgent(macos.InstallOptions{
			Home: home, Bin: filepath.Join(home, ".local", "bin", "aval"), Run: run, UID: os.Getuid(),
			SkipDefaults: os.Getenv("SKIP_DEFAULTS") == "1", SkipLaunchctl: os.Getenv("SKIP_LAUNCHCTL") == "1",
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "screenshots: %s (also copied to the clipboard)\n", filepath.Join(home, "Pictures", "Screenshots"))
		return nil

	case len(args) >= 2 && args[0] == "screenshot-clipboard" && args[1] == "copy":
		return copyScreenshot(args[2:], home)
	}
	return fmt.Errorf("%s", macosUsage)
}

// copyScreenshot is the action the LaunchAgent runs. CLIP_CMD replaces the
// clipboard writer (tests); SETTLE is the wait in seconds for macOS to finish
// writing the file.
func copyScreenshot(args []string, home string) error {
	dir := filepath.Join(home, "Pictures", "Screenshots")
	if len(args) > 0 {
		dir = args[0]
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	settle := 400 * time.Millisecond
	if v, err := strconv.ParseFloat(os.Getenv("SETTLE"), 64); err == nil {
		settle = time.Duration(v * float64(time.Second))
	}
	run := func(name string, a ...string) error { return exec.Command(name, a...).Run() }
	clip := macos.OsascriptClip(run)
	if c := os.Getenv("CLIP_CMD"); c != "" {
		clip = func(p string) error { return exec.Command(c, p).Run() }
	}
	return macos.CopyNewest(macos.CopyOptions{Dir: dir, StateDir: state, Settle: settle, Clip: clip})
}

// runReap is `aval reap-orphan-tests [--min-age SECONDS] [--dry-run]`, run every
// five minutes by ~/Library/LaunchAgents/dev.reap-orphan-tests.plist.
func runReap(args []string, out io.Writer) error {
	fs := newFlagSet("reap-orphan-tests", out)
	minAge := fs.Int("min-age", 600, "only end orphans at least this many seconds old")
	dry := fs.Bool("dry-run", false, "list what would be ended")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return reaper.Run(reaper.Options{MinAge: *minAge, Dry: *dry, Self: os.Getpid()}, out)
}

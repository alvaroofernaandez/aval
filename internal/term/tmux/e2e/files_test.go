//go:build !windows

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// openSrv starts a session with a ready .env file and returns the server plus
// an open() that runs `aval tmux open-file` against it with the stand-in editor.
func openSrv(t *testing.T) (*srv, func(env []string, files ...string) result) {
	s := newSrv(t)
	writeFile(t, filepath.Join(s.work, ".env"), "PORT=3000\n")
	s.start("")
	open := func(env []string, files ...string) result {
		if env == nil {
			env = s.with("FILE_EDITOR=" + fakeTool("fakeedit"))
		}
		return s.aval(env, append([]string{"tmux", "open-file"}, files...)...)
	}
	return s, open
}

func argsSeen(s *srv) func() string { return func() string { return s.fakeFile("args.txt") } }

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}

// Contract of open-file: opening a file from the sidebar creates a NEW tmux
// window. Whatever is running in the current window (a build, a REPL, an editor
// with unsaved work) is never touched, never has keys sent into it, and never
// loses focus state. That is the point of using a window instead of the work pane.
func TestOpenFile(t *testing.T) {
	t.Run("opens in a new window and leaves the current one untouched", func(t *testing.T) {
		s, open := openSrv(t)
		env := filepath.Join(s.work, ".env")
		before := s.windows()
		beforePane := s.q("list-panes", "-t", "main", "-F", "#{pane_id}")
		open(nil, env)
		waitUntil(settle, "\n", argsSeen(s))

		eq(t, "creates a new window", before+1, s.windows())
		eq(t, "runs the editor with the right argv", env, lastLine(s.fakeFile("args.txt")))
		eq(t, "does not split the original window", 1, countLines(s.q("list-panes", "-t", beforePane, "-F", "#{pane_id}")))
		eq(t, "leaves the original pane alive", beforePane, s.q("list-panes", "-t", beforePane, "-F", "#{pane_id}"))
		eq(t, "sends no keystrokes into the original pane", false, strings.Contains(s.q("capture-pane", "-p", "-t", beforePane), "fakeedit"))
	})

	// The old design refused when the work pane was busy. Opening a window
	// sidesteps that entirely: a running process is simply never involved.
	t.Run("a busy window is irrelevant", func(t *testing.T) {
		s, open := openSrv(t)
		busy := s.q("list-panes", "-t", "main", "-F", "#{pane_id}")
		s.must("send-keys", "-t", busy, "sleep 600", "Enter")
		if _, ok := waitUntil(settle, "sleep", func() string { return s.q("display", "-p", "-t", busy, "#{pane_current_command}") }); !ok {
			t.Fatal("the busy command never started")
		}
		busyBefore := s.q("capture-pane", "-p", "-t", busy)
		open(nil, filepath.Join(s.work, ".env"))
		waitUntil(settle, "\n", argsSeen(s))

		eq(t, "opens even while the current window is busy", 2, s.windows())
		// Anchored to the pane id, not the session: new-window moves focus, so a
		// session target would capture the newly opened window instead.
		eq(t, "the busy process is left undisturbed", busyBefore, s.q("capture-pane", "-p", "-t", busy))
		eq(t, "the busy command is still running", "sleep", s.q("display", "-p", "-t", busy, "#{pane_current_command}"))
	})

	t.Run("names the window after the file", func(t *testing.T) {
		s, open := openSrv(t)
		open(nil, filepath.Join(s.work, ".env"))
		waitUntil(settle, "\n", argsSeen(s))
		wantText(t, "window name", ".env", strings.ReplaceAll(s.q("list-windows", "-F", "#{window_name}"), "\n", " "))
	})

	t.Run("a path with spaces arrives as one argument", func(t *testing.T) {
		s, open := openSrv(t)
		spaced := filepath.Join(s.work, "my notes.md")
		writeFile(t, spaced, "# hi\n")
		open(nil, spaced)
		waitUntil(settle, "\n", argsSeen(s))
		eq(t, "argv", spaced+"\n", s.fakeFile("args.txt"))
	})

	t.Run("passes every selected file", func(t *testing.T) {
		s, open := openSrv(t)
		a, b := filepath.Join(s.work, "a.md"), filepath.Join(s.work, "b.md")
		writeFile(t, a, "a\n")
		writeFile(t, b, "b\n")
		open(nil, a, b)
		waitUntil(settle, b, argsSeen(s))
		eq(t, "argv", a+"\n"+b+"\n", s.fakeFile("args.txt"))
	})

	// yazi's opener inherits the tmux server's environment, which is launchd's
	// minimal PATH when Ghostty started the server.
	t.Run("opens a window under launchd's minimal PATH", func(t *testing.T) {
		s, open := openSrv(t)
		env := []string{"PATH=" + minimalPATH, "HOME=" + s.home, "E2E_FAKE_DIR=" + s.fake, "TMUX_SOCKET=" + s.sock, "FILE_EDITOR=" + fakeTool("fakeedit")}
		open(env, filepath.Join(s.work, ".env"))
		eq(t, "opens a window under minimal PATH", true, waitFor(settle, func() bool { return s.windows() == 2 }))
	})

	// Outside tmux, plain fullscreen yazi must keep working: the editor runs in
	// place of the helper. The stand-in blocks, as an editor does, so it is
	// stopped once it has reported its argv.
	t.Run("falls back to running the editor directly outside tmux", func(t *testing.T) {
		s, _ := openSrv(t)
		env := s.with("TMUX_SOCKET=", "TMUX=", "TMUX_BIN=/nonexistent/tmux", "FILE_EDITOR="+fakeTool("fakeedit"))
		file := filepath.Join(s.work, ".env")
		p := startCmd(t, env, avalBin, "tmux", "open-file", file)
		defer p.stop()
		waitUntil(settle, file, argsSeen(s))
		eq(t, "falls back to running the editor directly", file, lastLine(s.fakeFile("args.txt")))
		eq(t, "no window was created", 1, s.windows())
	})

	// The sidebar deliberately does NOT follow $EDITOR: that is nvim here, which
	// needs :q. The requirement is a file you can close with Ctrl+X, so the
	// default is nano, which asks "Save modified buffer?" instead of discarding
	// work. Asserted by behaviour: what matters is which editor actually runs.
	t.Run("default editor exits with Ctrl+X and ignores $EDITOR", func(t *testing.T) {
		if _, err := exec.LookPath("nano"); err != nil {
			if _, err := os.Stat("/usr/bin/nano"); err != nil {
				t.Skip("nano is not installed")
			}
		}
		s, _ := openSrv(t)
		s.aval(s.with("EDITOR=/should/not/be/used"), "tmux", "open-file", filepath.Join(s.work, ".env"))

		win, ok := waitUntil(settle, "@", func() string { return s.fileWindow() })
		if !ok {
			t.Fatal("no file window")
		}
		pane := s.q("list-panes", "-t", win, "-F", "#{pane_id}")
		// macOS ships nano as Apple's PICO build, so tmux reports "pico";
		// close-file matches both names for exactly this reason.
		isNano := func() bool {
			c := s.q("display", "-p", "-t", pane, "#{pane_current_command}")
			return c == "nano" || c == "pico"
		}
		eq(t, "defaults to nano/pico, which exits with Ctrl+X", true, waitFor(settle, isNano))
		eq(t, "ignores $EDITOR for this flow", false, strings.Contains(s.q("capture-pane", "-p", "-t", pane), "should/not/be/used"))

		// Ctrl+X must actually close it, the whole reason nano is the default.
		s.must("send-keys", "-t", pane, "C-x")
		eq(t, "Ctrl+X closes the file window", true, waitFor(settle, func() bool { return s.windows() == 1 }))
	})

	t.Run("FILE_EDITOR overrides the default", func(t *testing.T) {
		s, _ := openSrv(t)
		file := filepath.Join(s.work, ".env")
		s.aval(s.with("EDITOR=/should/not/be/used", "FILE_EDITOR="+fakeTool("fakeedit")), "tmux", "open-file", file)
		waitUntil(settle, "\n", argsSeen(s))
		eq(t, "FILE_EDITOR overrides the default", file, lastLine(s.fakeFile("args.txt")))
	})
}

// close-file closes a file opened from the sidebar without typing the editor's
// own quit command. The safety property: it must NEVER close a window the user
// was working in. Only windows this setup opened are eligible, and they are
// tagged at creation rather than guessed at.
func TestCloseFile(t *testing.T) {
	closeIn := func(s *srv, pane string) result {
		return s.aval(s.with("TMUX_PANE="+pane), "tmux", "close-file")
	}
	openWith := func(s *srv, editor string) (win, pane string) {
		s.aval(s.with("FILE_EDITOR="+editor), "tmux", "open-file", filepath.Join(s.work, ".env"))
		win, _ = waitUntil(settle, "@", func() string { return s.fileWindow() })
		pane = s.q("list-panes", "-t", win, "-F", "#{pane_id}")
		return
	}

	t.Run("tags the window it opens, and closes it", func(t *testing.T) {
		s, _ := openSrv(t)
		win, pane := openWith(s, fakeTool("fakeedit"))
		eq(t, "open-file tags the window it creates", true, win != "")
		eq(t, "two windows now", 2, s.windows())

		closeIn(s, pane)
		eq(t, "closes the window opened from the sidebar", true, waitFor(settle, func() bool { return s.windows() == 1 }))
	})

	t.Run("NEVER closes a working window", func(t *testing.T) {
		s, _ := openSrv(t)
		work := s.q("list-panes", "-t", "main", "-F", "#{pane_id}")
		r := closeIn(s, work)

		eq(t, "refuses with a non-zero status", true, r.rc != 0)
		eq(t, "refuses to close an untagged window", 1, s.windows())
		eq(t, "the working pane survives", work, s.q("list-panes", "-t", "main", "-F", "#{pane_id}"))
		eq(t, "says why it refused", true, regexp.MustCompile(`(?i)not.*opened|refus|only`).MatchString(r.out))
	})

	// It must ask the editor to quit rather than killing it, so unsaved changes
	// are the editor's call and not silently discarded. The nvim here is a
	// stand-in that only has to be NAMED nvim, which is what tmux reports.
	t.Run("an editor with unsaved work decides for itself", func(t *testing.T) {
		s, _ := openSrv(t)
		_, pane := openWith(s, fakeTool("nvim"))
		if _, ok := waitUntil(settle, "nvim", func() string { return s.q("display", "-p", "-t", pane, "#{pane_current_command}") }); !ok {
			t.Skip("this tmux does not report the stand-in as nvim")
		}
		r := closeIn(s, pane)

		eq(t, "asks a vim-family editor to quit instead of killing it", true, regexp.MustCompile(`(?i)quit|:q`).MatchString(r.out))
		eq(t, "does not force-kill the editor window", 2, s.windows())
		_, typed := waitUntil(settle, ":q", func() string { return s.q("capture-pane", "-p", "-t", pane) })
		eq(t, "sends the quit sequence to the editor", true, typed)
	})
}

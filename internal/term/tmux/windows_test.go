package tmux

import (
	"errors"
	"strings"
	"testing"
)

func TestCloseFile(t *testing.T) {
	const tagKey = "display -p -t %5 #{?@file_window,1,0}"
	const cmdKey = "display -p -t %5 #{pane_current_command}"

	cases := []struct {
		name     string
		tagged   string
		command  string
		wantRC   int
		wantOut  string
		wantCall []string
		noCall   string
	}{
		{"nvim is asked to quit", "1", "nvim", 0, "asked nvim to quit\n",
			[]string{"send-keys -t %5 Escape", "send-keys -t %5 :q Enter"}, "kill-window"},
		{"vi family: view", "1", "view", 0, "asked view to quit\n",
			[]string{"send-keys -t %5 :q Enter"}, "kill-window"},
		{"nano is asked with C-x", "1", "nano", 0, "asked nano to quit\n",
			[]string{"send-keys -t %5 C-x"}, "kill-window"},
		{"macOS nano reports pico", "1", "pico", 0, "asked pico to quit\n",
			[]string{"send-keys -t %5 C-x"}, "kill-window"},
		{"anything else is killed", "1", "fakeedit", 0, "",
			[]string{"kill-window -t %5"}, "send-keys"},
		{"untagged windows are never closed", "0", "zsh", 1, "",
			nil, "kill-window"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness()
			h.vars["TMUX_PANE"] = "%5"
			h.exec["/opt/homebrew/bin/tmux"] = true
			h.runner.reply = func(k string) (string, error) {
				switch k {
				case tagKey:
					return c.tagged + "\n", nil
				case cmdKey:
					return c.command + "\n", nil
				}
				return "", nil
			}
			assertEq(t, "exit code", c.wantRC, h.run("close-file"))
			assertEq(t, "stdout", c.wantOut, h.out.String())
			for _, w := range c.wantCall {
				if !h.runner.called(w) {
					t.Errorf("missing call %q in %v", w, h.runner.calls)
				}
			}
			if h.runner.called(c.noCall) {
				t.Errorf("unexpected %q in %v", c.noCall, h.runner.calls)
			}
			if c.tagged == "0" && !strings.Contains(h.errb.String(), "refusing to close it") {
				t.Errorf("should say why it refused, got %q", h.errb.String())
			}
		})
	}
}

func TestCloseFileTargetPane(t *testing.T) {
	t.Run("falls back to the active pane", func(t *testing.T) {
		h := newHarness()
		h.exec["/usr/bin/tmux"] = true
		h.runner.reply = func(k string) (string, error) {
			switch k {
			case "display -p #{pane_id}":
				return "%2\n", nil
			case "display -p -t %2 #{?@file_window,1,0}":
				return "1\n", nil
			case "display -p -t %2 #{pane_current_command}":
				return "sleep\n", nil
			}
			return "", nil
		}
		assertEq(t, "rc", 0, h.run("close-file"))
		assertEq(t, "kills the discovered pane", true, h.runner.called("kill-window -t %2"))
	})
	t.Run("no pane at all", func(t *testing.T) {
		h := newHarness()
		h.exec["/usr/bin/tmux"] = true
		h.runner.reply = func(string) (string, error) { return "", errors.New("no server") }
		assertEq(t, "rc", 1, h.run("close-file"))
		assertEq(t, "stderr", "close-file: no target pane\n", h.errb.String())
	})
	t.Run("honours the socket and the resolved binary", func(t *testing.T) {
		h := newHarness()
		h.vars["TMUX_PANE"] = "%1"
		h.vars["TMUX_SOCKET"] = "iso"
		h.exec["/opt/homebrew/bin/tmux"] = true
		h.run("close-file")
		assertEq(t, "bin", "/opt/homebrew/bin/tmux", h.runnerBin)
		assertEq(t, "socket", "iso", h.runnerSock)
	})
}

func TestOpenFile(t *testing.T) {
	t.Run("usage", func(t *testing.T) {
		h := newHarness()
		assertEq(t, "rc", 2, h.run("open-file"))
		assertEq(t, "stderr", "usage: open-file <file>...\n", h.errb.String())
	})

	tmuxUp := func(h *harness) {
		h.exec["/opt/homebrew/bin/tmux"] = true
		h.exec["/usr/bin/nano"] = true
		h.runner.reply = func(k string) (string, error) {
			if strings.HasPrefix(k, "new-window") {
				return "@4\n", nil
			}
			return "", nil
		}
	}

	// "exec": tmux runs the command through sh, and dash (Linux /bin/sh) does
	// not exec the last command itself, so pane_current_command read "sh" and
	// close-file never recognised the editor (caught by CI on ubuntu).
	cases := []struct {
		name    string
		files   []string
		editor  string
		wantCmd string
		wantNm  string
	}{
		{"plain path", []string{"/w/.env"}, "", "new-window -P -F #{window_id} -n .env exec /usr/bin/nano /w/.env", ".env"},
		{"spaces survive as one argument", []string{"/w/my notes.md"}, "",
			"new-window -P -F #{window_id} -n my notes.md exec /usr/bin/nano '/w/my notes.md'", "my notes.md"},
		{"every file is passed", []string{"/w/a.md", "/w/b.md"}, "",
			"new-window -P -F #{window_id} -n a.md exec /usr/bin/nano /w/a.md /w/b.md", "a.md"},
		{"metacharacters are quoted", []string{"/w/a'b;$x"}, "",
			`new-window -P -F #{window_id} -n a'b;$x exec /usr/bin/nano '/w/a'\''b;$x'`, "a'b;$x"},
		{"FILE_EDITOR overrides", []string{"/w/a"}, "/opt/ed",
			"new-window -P -F #{window_id} -n a exec /opt/ed /w/a", "a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness()
			tmuxUp(h)
			if c.editor != "" {
				h.vars["FILE_EDITOR"] = c.editor
			}
			assertEq(t, "rc", 0, h.run(append([]string{"open-file"}, c.files...)...))
			var got string
			for _, call := range h.runner.calls {
				if strings.HasPrefix(call, "new-window") {
					got = call
				}
			}
			assertEq(t, "new-window", c.wantCmd, got)
			assertEq(t, "window is tagged", true, h.runner.called("set -w -t @4 @file_window 1"))
			assertEq(t, "never execs the editor", 0, len(h.execCalls))
		})
	}

	t.Run("falls back to the editor outside tmux", func(t *testing.T) {
		h := newHarness()
		h.exec["/usr/bin/nano"] = true
		h.vars["TMUX_BIN"] = "/nonexistent/tmux"
		assertEq(t, "rc", 0, h.run("open-file", "/w/.env"))
		assertEq(t, "exec", "/usr/bin/nano|/usr/bin/nano|/w/.env", strings.Join(h.execCalls[0], "|"))
	})
	t.Run("falls back when there is no session", func(t *testing.T) {
		h := newHarness()
		tmuxUp(h)
		h.runner.reply = func(k string) (string, error) {
			if k == "has-session" {
				return "", errors.New("no server running")
			}
			return "", nil
		}
		h.run("open-file", "/w/.env")
		assertEq(t, "execs once", 1, len(h.execCalls))
		assertEq(t, "no window", false, h.runner.called("new-window"))
	})
	t.Run("editor with arguments is split on fallback", func(t *testing.T) {
		h := newHarness()
		h.vars["FILE_EDITOR"] = "/e/ed -R"
		h.vars["TMUX_BIN"] = "/nonexistent/tmux"
		h.run("open-file", "/w/a")
		assertEq(t, "argv", "/e/ed|/e/ed|-R|/w/a", strings.Join(h.execCalls[0], "|"))
	})
	t.Run("nix profile is searched before homebrew", func(t *testing.T) {
		h := newHarness()
		tmuxUp(h)
		h.exec["/home/u/.nix-profile/bin/nano"] = true
		h.run("open-file", "/w/a")
		assertEq(t, "uses nix nano", true, h.runner.called("new-window -P -F #{window_id} -n a exec /home/u/.nix-profile/bin/nano /w/a"))
	})
}

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/bin/nano": "/usr/bin/nano",
		"a b":           "'a b'",
		"it's":          `'it'\''s'`,
		"":              "''",
		"a$b":           "'a$b'",
	} {
		assertEq(t, in, want, shellQuote(in))
	}
}

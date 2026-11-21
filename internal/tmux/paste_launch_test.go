package tmux

import (
	"errors"
	"strings"
	"testing"
)

const (
	clipScreenshot = "«class PNGf», 703838, «class AVIF», 164185, «class 8BPS», 5245480, GIF picture, 265978, TIFF picture, 23758990"
	clipText       = "«class utxt», 10, «class ut16», 22"
	clipRichText   = "«class utxt», 41, «class ut16», 84, «class RTF », 233, TIFF picture, 9210"
)

func TestPasteRoute(t *testing.T) {
	cases := []struct {
		name, goos, info, want string
	}{
		{"screenshot", "Darwin", clipScreenshot, "image"},
		{"plain text", "Darwin", clipText, "text"},
		{"rich text with a TIFF still text", "Darwin", clipRichText, "text"},
		{"empty clipboard", "Darwin", "", "text"},
		{"linux image alone", "Linux", "image/png", "image"},
		{"linux text/plain wins", "Linux", "text/plain;charset=utf-8\nimage/png", "text"},
		{"linux UTF8_STRING wins", "Linux", "UTF8_STRING\nimage/png", "text"},
		{"linux empty", "Linux", "", "text"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness()
			h.vars["CLIPBOARD_INFO"] = c.info
			h.vars["PASTE_OS"] = c.goos
			h.vars["ROUTE_ONLY"] = "1"
			assertEq(t, "rc", 0, h.run("paste-router"))
			assertEq(t, "route", c.want+"\n", h.out.String())
			assertEq(t, "no tmux call", 0, len(h.runner.calls))
		})
	}
}

func TestPasteBackend(t *testing.T) {
	cases := []struct {
		name, goos, wayland string
		tools               []string
		want                string
	}{
		{"macOS uses pbpaste", "Darwin", "", []string{"pbpaste", "wl-paste", "xclip"}, "pbpaste"},
		{"macOS without pbpaste", "Darwin", "", nil, "none"},
		{"wayland prefers wl-paste", "Linux", "wayland-0", []string{"wl-paste", "xclip", "xsel"}, "wl-paste"},
		{"no wayland skips wl-paste", "Linux", "", []string{"wl-paste", "xclip", "xsel"}, "xclip"},
		{"xsel when no xclip", "Linux", "", []string{"xsel"}, "xsel"},
		{"wayland session without wl-paste", "Linux", "wayland-0", []string{"xclip"}, "xclip"},
		{"nothing installed", "Linux", "", nil, "none"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness()
			h.vars["PASTE_OS"] = c.goos
			h.vars["CLIPBOARD_BACKEND_ONLY"] = "1"
			if c.wayland != "" {
				h.vars["WAYLAND_DISPLAY"] = c.wayland
			}
			for _, tool := range c.tools {
				h.look[tool] = "/fake/" + tool
			}
			assertEq(t, "rc", 0, h.run("paste-router"))
			assertEq(t, "backend", c.want+"\n", h.out.String())
		})
	}
}

func TestPasteResolveOnly(t *testing.T) {
	t.Run("search order, homebrew first", func(t *testing.T) {
		h := newHarness()
		h.vars["RESOLVE_ONLY"] = "1"
		h.exec["/usr/bin/tmux"] = true
		h.exec["/opt/homebrew/bin/tmux"] = true
		h.run("paste-router")
		assertEq(t, "out", "/opt/homebrew/bin/tmux\n", h.out.String())
	})
	t.Run("TMUX_BIN wins when executable", func(t *testing.T) {
		h := newHarness()
		h.vars["RESOLVE_ONLY"] = "1"
		h.vars["TMUX_BIN"] = "/stub/tmux"
		h.exec["/stub/tmux"] = true
		h.exec["/opt/homebrew/bin/tmux"] = true
		h.run("paste-router")
		assertEq(t, "out", "/stub/tmux\n", h.out.String())
	})
	t.Run("falls back to command -v", func(t *testing.T) {
		h := newHarness()
		h.vars["RESOLVE_ONLY"] = "1"
		h.look["tmux"] = "/weird/tmux"
		h.exec["/weird/tmux"] = true
		h.run("paste-router")
		assertEq(t, "out", "/weird/tmux\n", h.out.String())
	})
	t.Run("nothing found prints nothing and succeeds", func(t *testing.T) {
		h := newHarness()
		h.vars["RESOLVE_ONLY"] = "1"
		assertEq(t, "rc", 0, h.run("paste-router"))
		assertEq(t, "out", "", h.out.String())
	})
}

func TestPasteActions(t *testing.T) {
	setup := func(info string) *harness {
		h := newHarness()
		h.vars["TMUX_BIN"] = "/stub/tmux"
		h.exec["/stub/tmux"] = true
		h.vars["CLIPBOARD_INFO"] = info
		h.vars["TMUX_PANE"] = "%3"
		h.look["pbpaste"] = "/usr/bin/pbpaste"
		return h
	}

	t.Run("image sends the 0x16 byte to the pane", func(t *testing.T) {
		h := setup(clipScreenshot)
		assertEq(t, "rc", 0, h.run("paste-router"))
		assertEq(t, "calls", "send-keys -t %3 0x16", strings.Join(h.runner.calls, ";"))
		assertEq(t, "bin", "/stub/tmux", h.runnerBin)
	})
	t.Run("image without a pane targets the active one", func(t *testing.T) {
		h := setup(clipScreenshot)
		delete(h.vars, "TMUX_PANE")
		h.run("paste-router")
		assertEq(t, "calls", "send-keys 0x16", strings.Join(h.runner.calls, ";"))
	})
	t.Run("text goes through the paste buffer, never 0x16", func(t *testing.T) {
		h := setup(clipText)
		h.outputs["/usr/bin/pbpaste"] = "hello\n\n"
		assertEq(t, "rc", 0, h.run("paste-router"))
		assertEq(t, "calls", "set-buffer -- hello;paste-buffer -t %3 -p", strings.Join(h.runner.calls, ";"))
	})
	t.Run("linux wl-paste reads without a trailing newline", func(t *testing.T) {
		h := setup("text/plain")
		h.vars["PASTE_OS"] = "Linux"
		h.vars["WAYLAND_DISPLAY"] = "wayland-0"
		h.look["wl-paste"] = "/usr/bin/wl-paste"
		h.outputs["/usr/bin/wl-paste --no-newline"] = "hi"
		h.run("paste-router")
		assertEq(t, "calls", "set-buffer -- hi;paste-buffer -t %3 -p", strings.Join(h.runner.calls, ";"))
	})
	t.Run("no clipboard tool: quiet no-op", func(t *testing.T) {
		h := setup(clipText)
		delete(h.look, "pbpaste")
		assertEq(t, "rc", 0, h.run("paste-router"))
		assertEq(t, "no calls", 0, len(h.runner.calls))
		assertEq(t, "no stderr", "", h.errb.String())
	})
	t.Run("probes the real clipboard when no override is given", func(t *testing.T) {
		h := setup("")
		delete(h.vars, "CLIPBOARD_INFO")
		h.look["osascript"] = "/usr/bin/osascript"
		h.outputs["/usr/bin/osascript -e clipboard info"] = clipScreenshot
		h.run("paste-router")
		assertEq(t, "image route", "send-keys -t %3 0x16", strings.Join(h.runner.calls, ";"))
	})
	t.Run("probe via osascript falls back to /usr/bin", func(t *testing.T) {
		h := setup("")
		delete(h.vars, "CLIPBOARD_INFO")
		h.outputs["/usr/bin/osascript -e clipboard info"] = clipScreenshot
		h.run("paste-router")
		assertEq(t, "image route", "send-keys -t %3 0x16", strings.Join(h.runner.calls, ";"))
	})
	t.Run("tmux missing is 127", func(t *testing.T) {
		h := newHarness()
		h.vars["CLIPBOARD_INFO"] = clipScreenshot
		assertEq(t, "rc", 127, h.run("paste-router"))
	})
}

func TestLaunch(t *testing.T) {
	t.Run("resolve-only: override is authoritative", func(t *testing.T) {
		h := newHarness()
		h.vars["LAUNCH_RESOLVE_ONLY"] = "1"
		h.vars["TMUX_BIN"] = "/nonexistent/tmux"
		h.exec["/opt/homebrew/bin/tmux"] = true
		assertEq(t, "rc", 0, h.run("launch"))
		assertEq(t, "out", "", h.out.String())
	})
	t.Run("resolve-only: search order", func(t *testing.T) {
		h := newHarness()
		h.vars["LAUNCH_RESOLVE_ONLY"] = "1"
		h.exec["/usr/local/bin/tmux"] = true
		h.exec["/home/u/.nix-profile/bin/tmux"] = true
		h.run("launch")
		assertEq(t, "out", "/usr/local/bin/tmux\n", h.out.String())
	})
	t.Run("resolve-only: command -v last", func(t *testing.T) {
		h := newHarness()
		h.vars["LAUNCH_RESOLVE_ONLY"] = "1"
		h.look["tmux"] = "/x/tmux"
		h.exec["/x/tmux"] = true
		h.run("launch")
		assertEq(t, "out", "/x/tmux\n", h.out.String())
	})

	picks := []struct {
		name, sessions, want string
	}{
		{"no sessions", "", "new"},
		{"first free session", "a|1\nb|0\nc|0\n", "b"},
		{"all attached", "a|1\nb|2\n", "new"},
	}
	for _, c := range picks {
		t.Run("pick-only: "+c.name, func(t *testing.T) {
			h := newHarness()
			h.vars["LAUNCH_PICK_ONLY"] = "1"
			h.exec["/opt/homebrew/bin/tmux"] = true
			h.runner.reply = func(k string) (string, error) {
				assertEq(t, "query", "list-sessions -F #{session_name}|#{session_attached}", k)
				return c.sessions, nil
			}
			assertEq(t, "rc", 0, h.run("launch"))
			assertEq(t, "out", c.want+"\n", h.out.String())
			assertEq(t, "no exec", 0, len(h.execCalls))
		})
	}

	t.Run("attaches to a free session", func(t *testing.T) {
		h := newHarness()
		h.exec["/opt/homebrew/bin/tmux"] = true
		h.runner.reply = func(string) (string, error) { return "a|1\nb|0\n", nil }
		h.run("launch")
		assertEq(t, "exec", "/opt/homebrew/bin/tmux|/opt/homebrew/bin/tmux|attach|-t|b", strings.Join(h.execCalls[0], "|"))
	})
	t.Run("creates a session when all are in use, with the socket", func(t *testing.T) {
		h := newHarness()
		h.exec["/opt/homebrew/bin/tmux"] = true
		h.vars["TMUX_SOCKET"] = "iso"
		h.runner.reply = func(string) (string, error) { return "a|1\n", nil }
		h.run("launch")
		assertEq(t, "exec", "/opt/homebrew/bin/tmux|/opt/homebrew/bin/tmux|-L|iso|new-session", strings.Join(h.execCalls[0], "|"))
		assertEq(t, "runner socket", "iso", h.runnerSock)
	})
	t.Run("no server yet: new session", func(t *testing.T) {
		h := newHarness()
		h.exec["/opt/homebrew/bin/tmux"] = true
		h.runner.reply = func(string) (string, error) { return "", errors.New("no server running") }
		h.run("launch")
		assertEq(t, "exec", "/opt/homebrew/bin/tmux|/opt/homebrew/bin/tmux|new-session", strings.Join(h.execCalls[0], "|"))
	})
	t.Run("no tmux: a login shell, never a dead window", func(t *testing.T) {
		h := newHarness()
		h.vars["SHELL"] = "/bin/fish"
		h.run("launch")
		assertEq(t, "exec", "/bin/fish|/bin/fish|-l", strings.Join(h.execCalls[0], "|"))
	})
	t.Run("no tmux and no SHELL: zsh", func(t *testing.T) {
		h := newHarness()
		h.run("launch")
		assertEq(t, "exec", "/bin/zsh|/bin/zsh|-l", strings.Join(h.execCalls[0], "|"))
	})
	t.Run("exec failure is reported", func(t *testing.T) {
		h := newHarness()
		h.execErr = errors.New("exec format error")
		assertEq(t, "rc", 1, h.run("launch"))
		assertEq(t, "stderr", true, strings.Contains(h.errb.String(), "exec format error"))
	})
}

func TestRunDispatch(t *testing.T) {
	h := newHarness()
	assertEq(t, "unknown subcommand", 2, h.run("bogus"))
	assertEq(t, "usage on stderr", true, strings.Contains(h.errb.String(), "usage: aval tmux"))
	h = newHarness()
	assertEq(t, "no subcommand", 2, h.run())
}

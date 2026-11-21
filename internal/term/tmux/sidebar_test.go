package tmux

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSidebarFollow(t *testing.T) {
	t.Run("outside tmux it does nothing at all", func(t *testing.T) {
		h := newHarness()
		assertEq(t, "rc", 0, h.run("sidebar-follow", "/x"))
		assertEq(t, "no tmux call", 0, len(h.runner.calls))
		assertEq(t, "no output", "", h.out.String()+h.errb.String())
	})

	follow := func(h *harness) {
		h.vars["TMUX_PANE"] = "%3"
		h.exec["/opt/homebrew/bin/tmux"] = true
		h.exec["/opt/homebrew/bin/ya"] = true
	}

	t.Run("no sidebar in the window: no ya", func(t *testing.T) {
		h := newHarness()
		follow(h)
		assertEq(t, "rc", 0, h.run("sidebar-follow", "/x"))
		assertEq(t, "asked once", "display -p -t %3 #{@sidebar_client_id}", strings.Join(h.runner.calls, ";"))
		assertEq(t, "ya never run", 0, len(h.outCalls))
	})
	t.Run("emits cd to the recorded instance", func(t *testing.T) {
		h := newHarness()
		follow(h)
		h.runner.reply = func(string) (string, error) { return "4242\n", nil }
		assertEq(t, "rc", 0, h.run("sidebar-follow", "/some dir"))
		assertEq(t, "ya", "/opt/homebrew/bin/ya emit-to 4242 cd /some dir", strings.Join(h.outCalls, ";"))
	})
	t.Run("defaults to $PWD", func(t *testing.T) {
		h := newHarness()
		follow(h)
		h.vars["PWD"] = "/cwd"
		h.runner.reply = func(string) (string, error) { return "7\n", nil }
		h.run("sidebar-follow")
		assertEq(t, "ya", "/opt/homebrew/bin/ya emit-to 7 cd /cwd", strings.Join(h.outCalls, ";"))
	})
	t.Run("no pane id: asks tmux for the active pane", func(t *testing.T) {
		h := newHarness()
		h.vars["TMUX"] = "/tmp/tmux-1/default,1,0"
		h.exec["/usr/bin/tmux"] = true
		h.run("sidebar-follow", "/x")
		assertEq(t, "call", "display -p #{@sidebar_client_id}", h.runner.calls[0])
	})
	t.Run("never fails loudly: ya missing", func(t *testing.T) {
		h := newHarness()
		follow(h)
		h.vars["YA_BIN"] = "/nonexistent/ya"
		h.runner.reply = func(string) (string, error) { return "4242\n", nil }
		assertEq(t, "rc", 0, h.run("sidebar-follow", "/x"))
		assertEq(t, "silent", "", h.out.String()+h.errb.String())
		assertEq(t, "ya not run", 0, len(h.outCalls))
	})
	t.Run("never fails loudly: ya errors", func(t *testing.T) {
		h := newHarness()
		follow(h)
		h.runner.reply = func(string) (string, error) { return "4242\n", nil }
		h.outErr["/opt/homebrew/bin/ya emit-to 4242 cd /x"] = true
		assertEq(t, "rc", 0, h.run("sidebar-follow", "/x"))
		assertEq(t, "silent", "", h.out.String()+h.errb.String())
	})
	t.Run("never fails loudly: tmux errors", func(t *testing.T) {
		h := newHarness()
		follow(h)
		h.runner.reply = func(string) (string, error) { return "", errors.New("boom") }
		assertEq(t, "rc", 0, h.run("sidebar-follow", "/x"))
	})
}

func TestSidebarWatch(t *testing.T) {
	const list = "list-panes -t %1 -F #{pane_id}|#{?@sidebar,1,0}|#{pane_current_path}"

	t.Run("needs both arguments", func(t *testing.T) {
		h := newHarness()
		assertEq(t, "rc", 1, h.run("sidebar-watch", "%1"))
	})

	t.Run("follows the work pane, one tmux call per cycle, then dies with the server", func(t *testing.T) {
		h := newHarness()
		h.exec["/opt/homebrew/bin/tmux"] = true
		h.exec["/opt/homebrew/bin/ya"] = true
		h.vars["WATCH_INTERVAL"] = "0.3"
		replies := []struct {
			out string
			err bool
		}{
			{"%1|1|/a\n%2|0|/a\n", false}, // first pass: record, no emit
			{"%1|1|/a\n%2|0|/b\n", false}, // moved: emit
			{"%1|1|/a\n%2|0|/b\n", false}, // unchanged: quiet
			{"", true},                    // server gone: exit
		}
		i := 0
		h.runner.reply = func(k string) (string, error) {
			assertEq(t, "the only tmux call is list-panes", list, k)
			r := replies[i]
			i++
			if r.err {
				return "", errors.New("no server")
			}
			return r.out, nil
		}
		assertEq(t, "rc", 0, h.run("sidebar-watch", "%1", "99"))
		assertEq(t, "calls", 4, len(h.runner.calls))
		assertEq(t, "single emit", "/opt/homebrew/bin/ya emit-to 99 cd /b", strings.Join(h.outCalls, ";"))
		assertEq(t, "slept between cycles", 3, len(h.sleeps))
		assertEq(t, "interval", 300*time.Millisecond, h.sleeps[0])
	})

	t.Run("exits when the sidebar lost its tag", func(t *testing.T) {
		h := newHarness()
		h.exec["/usr/bin/tmux"] = true
		h.runner.reply = func(string) (string, error) { return "%1|0|/a\n%2|0|/a\n", nil }
		assertEq(t, "rc", 0, h.run("sidebar-watch", "%1", "99"))
		assertEq(t, "one call", 1, len(h.runner.calls))
	})
	t.Run("exits on an empty listing", func(t *testing.T) {
		h := newHarness()
		h.exec["/usr/bin/tmux"] = true
		h.runner.reply = func(string) (string, error) { return "\n", nil }
		assertEq(t, "rc", 0, h.run("sidebar-watch", "%1", "99"))
		assertEq(t, "no sleep", 0, len(h.sleeps))
	})
	t.Run("default interval is two seconds", func(t *testing.T) {
		h := newHarness()
		h.exec["/usr/bin/tmux"] = true
		n := 0
		h.runner.reply = func(string) (string, error) {
			n++
			if n > 1 {
				return "", errors.New("gone")
			}
			return "%1|1|/a\n%2|0|/a\n", nil
		}
		h.run("sidebar-watch", "%1", "99")
		assertEq(t, "interval", 2*time.Second, h.sleeps[0])
	})
}

func TestSidebarToggle(t *testing.T) {
	ready := func(h *harness) {
		h.exec["/opt/homebrew/bin/tmux"] = true
		h.vars["TMUX_PANE"] = "%1"
		h.runner.reply = func(k string) (string, error) {
			switch {
			case k == "display -p -t %1 #{window_id}":
				return "@1\n", nil
			case strings.HasPrefix(k, "list-panes"):
				return "%1|0\n", nil
			case k == "display -p -t %1 #{pane_current_path}":
				return "/work\n", nil
			case strings.HasPrefix(k, "split-window"):
				return "%9\n", nil
			}
			return "", nil
		}
	}

	t.Run("tmux not found is 127", func(t *testing.T) {
		h := newHarness()
		assertEq(t, "rc", 127, h.run("sidebar-toggle"))
		assertEq(t, "stderr", "sidebar-toggle: tmux not found\n", h.errb.String())
	})
	t.Run("a stale pane is refused loudly", func(t *testing.T) {
		h := newHarness()
		ready(h)
		h.vars["TMUX_PANE"] = "%999"
		h.runner.reply = func(string) (string, error) { return "", errors.New("can't find pane") }
		assertEq(t, "rc", 1, h.run("sidebar-toggle"))
		assertEq(t, "stderr", "sidebar-toggle: no such pane or window: %999\n", h.errb.String())
	})
	t.Run("second toggle closes the sidebar", func(t *testing.T) {
		h := newHarness()
		ready(h)
		h.runner.reply = func(k string) (string, error) {
			if strings.HasPrefix(k, "list-panes") {
				return "%1|0\n%7|1\n", nil
			}
			return "@1\n", nil
		}
		assertEq(t, "rc", 0, h.run("sidebar-toggle"))
		assertEq(t, "kills the tagged pane only", true, h.runner.called("kill-pane -t %7"))
		assertEq(t, "no split", false, h.runner.called("split-window"))
	})

	t.Run("opens a yazi sidebar with a client id and a watcher", func(t *testing.T) {
		h := newHarness()
		ready(h)
		h.vars["SIDEBAR_CMD"] = "/x/yazi"
		h.vars["TMUX_SOCKET"] = "iso"
		h.dirs["/home/u/.config/yazi-sidebar"] = true
		h.randVal = 0
		assertEq(t, "rc", 0, h.run("sidebar-toggle"))
		want := "split-window -t %1 -h -b -l 30% -c /work -e YAZI_CONFIG_HOME=/home/u/.config/yazi-sidebar -P -F #{pane_id} /x/yazi --client-id 2000"
		assertEq(t, "split", true, h.runner.called(want))
		assertEq(t, "client id tag", true, h.runner.called("set -w -t %9 @sidebar_client_id 2000"))
		assertEq(t, "sidebar tag", true, h.runner.called("set -p -t %9 @sidebar 1"))
		if len(h.spawns) != 1 {
			t.Fatalf("want one watcher, got %d", len(h.spawns))
		}
		sp := h.spawns[0]
		assertEq(t, "watcher binary", h.self, sp.path)
		assertEq(t, "watcher args", "tmux sidebar-watch %9 2000", strings.Join(sp.args, " "))
		env := strings.Join(sp.env, "\n")
		for _, w := range []string{"TMUX_SOCKET=iso", "TMUX_BIN=/opt/homebrew/bin/tmux", "WATCH_INTERVAL=2", "YA_BIN="} {
			if !strings.Contains(env, w) {
				t.Errorf("watcher env lacks %q: %v", w, sp.env)
			}
		}
		assertEq(t, "runner socket", "iso", h.runnerSock)
	})
	t.Run("tag is set before the watcher starts", func(t *testing.T) {
		h := newHarness()
		ready(h)
		h.vars["SIDEBAR_CMD"] = "/x/yazi"
		h.env() // no-op; ordering is asserted through call counts below
		h.run("sidebar-toggle")
		last := h.runner.calls[len(h.runner.calls)-1]
		assertEq(t, "last call before spawn", "set -p -t %9 @sidebar 1", last)
		assertEq(t, "spawned after", 1, len(h.spawns))
	})
	t.Run("a non-yazi command gets no client id and no watcher", func(t *testing.T) {
		h := newHarness()
		ready(h)
		h.vars["SIDEBAR_CMD"] = "sleep 600"
		h.vars["SIDEBAR_WIDTH"] = "40"
		assertEq(t, "rc", 0, h.run("sidebar-toggle"))
		assertEq(t, "split", true, h.runner.called("split-window -t %1 -h -b -l 40 -c /work -P -F #{pane_id} sleep 600"))
		assertEq(t, "no client id", false, h.runner.called("set -w"))
		assertEq(t, "sidebar tag", true, h.runner.called("set -p -t %9 @sidebar 1"))
		assertEq(t, "no watcher", 0, len(h.spawns))
	})
	t.Run("a bare command is resolved to an absolute path", func(t *testing.T) {
		h := newHarness()
		ready(h)
		h.vars["SIDEBAR_CMD"] = "sleepy"
		h.exec["/opt/homebrew/bin/sleepy"] = true
		h.run("sidebar-toggle")
		assertEq(t, "resolved", true, h.runner.called("split-window -t %1 -h -b -l 30% -c /work -P -F #{pane_id} /opt/homebrew/bin/sleepy"))
	})
	t.Run("default command is yazi, resolved", func(t *testing.T) {
		h := newHarness()
		ready(h)
		h.exec["/opt/homebrew/bin/yazi"] = true
		h.run("sidebar-toggle")
		assertEq(t, "yazi with client id", true, h.runner.called("split-window -t %1 -h -b -l 30% -c /work -P -F #{pane_id} /opt/homebrew/bin/yazi --client-id "))
	})
	t.Run("directory comes from the target pane, HOME as a last resort", func(t *testing.T) {
		h := newHarness()
		ready(h)
		h.vars["SIDEBAR_CMD"] = "sleep 1"
		base := h.runner.reply
		h.runner.reply = func(k string) (string, error) {
			if k == "display -p -t %1 #{pane_current_path}" {
				return "", errors.New("gone")
			}
			return base(k)
		}
		h.run("sidebar-toggle")
		assertEq(t, "home", true, h.runner.called("split-window -t %1 -h -b -l 30% -c /home/u "))
	})
	t.Run("without TMUX_PANE the active window is used", func(t *testing.T) {
		h := newHarness()
		ready(h)
		delete(h.vars, "TMUX_PANE")
		h.runner.reply = func(k string) (string, error) {
			switch k {
			case "display -p #{window_id}":
				return "@3\n", nil
			case "display -p -t @3 #{window_id}":
				return "@3\n", nil
			case "list-panes -t @3 -F #{pane_id}|#{?@sidebar,1,0}":
				return "%4|1\n", nil
			}
			return "", nil
		}
		assertEq(t, "rc", 0, h.run("sidebar-toggle"))
		assertEq(t, "kill", true, h.runner.called("kill-pane -t %4"))
	})
	t.Run("a split failure is an error", func(t *testing.T) {
		h := newHarness()
		ready(h)
		base := h.runner.reply
		h.runner.reply = func(k string) (string, error) {
			if strings.HasPrefix(k, "split-window") {
				return "", errors.New("no space for new pane")
			}
			return base(k)
		}
		assertEq(t, "rc", 1, h.run("sidebar-toggle"))
	})
}

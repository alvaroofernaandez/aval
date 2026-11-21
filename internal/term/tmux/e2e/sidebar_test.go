//go:build !windows

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const minimalPATH = "/usr/bin:/bin:/usr/sbin:/sbin"

func TestSidebarToggle(t *testing.T) {
	s := newSrv(t)
	work := s.start("")

	// --- opening ---
	if r := s.toggle(""); r.rc != 0 {
		t.Fatalf("toggle: rc %d: %s", r.rc, r.out)
	}
	eq(t, "opens a second pane", 2, s.panes())

	sidebar := s.sidebarPane("main")
	if sidebar == "" {
		t.Fatal("no pane carries @sidebar")
	}
	eq(t, "does not mark the working pane", "", s.q("display", "-p", "-t", work, "#{@sidebar}"))
	// Leftmost pane: x offset 0, and narrower than the working pane.
	eq(t, "places the sidebar on the left", "0", s.q("display", "-p", "-t", sidebar, "#{pane_left}"))

	sw, _ := strconv.Atoi(s.q("display", "-p", "-t", sidebar, "#{pane_width}"))
	ww, _ := strconv.Atoi(s.q("display", "-p", "-t", work, "#{pane_width}"))
	if sw >= ww || sw == 0 {
		t.Errorf("sidebar width %d should be narrower than the working pane's %d", sw, ww)
	}

	got, _ := waitUntil(settle, s.work, func() string {
		return resolved(s.q("display", "-p", "-t", sidebar, "#{pane_current_path}"))
	})
	eq(t, "inherits the working directory", s.work, got)
	eq(t, "focuses the sidebar so it is usable immediately", sidebar, s.q("display", "-p", "-t", "main", "#{pane_id}"))

	// --- closing ---
	s.toggle("")
	eq(t, "second toggle closes the sidebar", 1, s.panes())
	eq(t, "keeps the working pane alive", work, s.q("list-panes", "-t", "main", "-F", "#{pane_id}"))

	// --- reopening ---
	s.toggle("")
	eq(t, "reopens after closing", 2, s.panes())

	// --- isolation: a sidebar belongs to its window ---
	s.must("new-window", "-t", "main", "-c", s.work)
	eq(t, "other windows have no sidebar", 1, countLines(s.q("list-panes", "-F", "#{pane_id}")))
}

// tmux's run-shell inherits the SERVER's environment. When Ghostty starts the
// server, that PATH is launchd's minimal one, with no /opt/homebrew/bin. A bare
// `tmux` inside the helper is then not found and the whole thing exits 127,
// which is exactly how this failed in practice.
//
// launchd is macOS. On Linux this reconstructs an environment that machine
// never has, and a failure would say nothing about the helper, so it is skipped
// there rather than asserted against a scenario that cannot occur.
func TestSidebarToggleUnderLaunchdPath(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd's minimal PATH is macOS-only")
	}

	// minimal is `env -i`: nothing but the minimal PATH and what the case names.
	minimal := func(s *srv, extra ...string) []string {
		return append([]string{"PATH=" + minimalPATH, "HOME=" + s.home, "E2E_FAKE_DIR=" + s.fake, "WATCH_INTERVAL=0.2", "TMUX_SOCKET=" + s.sock}, extra...)
	}

	t.Run("stand-in command", func(t *testing.T) {
		s := newSrv(t)
		work := s.start("")
		r := s.aval(minimal(s, "SIDEBAR_CMD=sleep 600", "TMUX_PANE="+work), "tmux", "sidebar-toggle")

		eq(t, "runs under launchd's minimal PATH", 0, r.rc)
		if regexp.MustCompile(`(?i)not found`).MatchString(r.out) {
			t.Errorf("fails with command-not-found: %s", r.out)
		}
		eq(t, "opens the sidebar under minimal PATH", 2, s.panes())
	})

	// The pane surviving matters more than the pane being created. With the real
	// sidebar command under a minimal PATH, tmux opens the pane, the command
	// fails to resolve, and the pane dies at once, which looks like the sidebar
	// "flickering and closing". `sleep` as a stand-in hides that entirely,
	// because /bin/sleep resolves anywhere.
	t.Run("real sidebar command survives", func(t *testing.T) {
		yazi := skipUnlessYazi(t)
		s := newSrv(t)
		work := s.start("")
		// Where an installation puts it, so resolution has to find it itself.
		nix := filepath.Join(s.home, ".nix-profile", "bin")
		if err := os.MkdirAll(nix, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(yazi, filepath.Join(nix, "yazi")); err != nil {
			t.Fatal(err)
		}
		r := s.aval(minimal(s, "TMUX_PANE="+work), "tmux", "sidebar-toggle")
		eq(t, "toggle rc", 0, r.rc)

		// Observe for a window: a dead pane vanishes (remain-on-exit is off), so
		// the pane count dropping back is the failure signature.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if n := s.panes(); n != 2 {
				t.Fatalf("the real sidebar command did not survive: %d pane(s)", n)
			}
			time.Sleep(100 * time.Millisecond)
		}
		eq(t, "the sidebar pane is not dead", 0, strings.Count(s.q("list-panes", "-t", "main", "-F", "#{pane_dead}"), "1"))
	})
}

// Working on two projects at once is the normal case. Each window's sidebar
// must start in ITS OWN directory: `split-window -t <pane> -c '#{pane_current_path}'`
// creates the pane in the right window but expands -c against the ACTIVE pane,
// so the second sidebar inherited the first project's directory.
//
// Its own server, so nothing here depends on another test's leftovers: the old
// suite did kill-server and rebuilt on one socket, and kill-server only STARTS
// the shutdown, so the next new-session could land on a dying server and vanish
// with it. A fresh socket cannot collide with a server that is shutting down.
func TestSidebarToggleTwoWindows(t *testing.T) {
	s := newSrv(t)
	dirA := filepath.Join(s.work, "proyecto-a")
	dirB := filepath.Join(s.work, "proyecto-b")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	paneA := s.start(dirA)
	winA := s.must("list-windows", "-t", "main", "-F", "#{window_id}")

	s.must("new-window", "-t", "main", "-c", dirB)
	lastWindow := func() string {
		ids := strings.Split(s.q("list-windows", "-t", "main", "-F", "#{window_id}"), "\n")
		return ids[len(ids)-1]
	}
	// Poll until the newest window id differs from A's. Reading too early
	// returns A again, and every assertion below would compare A with itself
	// and pass for the wrong reason, which is worse than failing.
	if !waitFor(settle, func() bool { return lastWindow() != winA }) {
		t.Fatal("window B never appeared")
	}
	winB := lastWindow()
	paneB, ok := waitUntil(settle, "%", func() string { return s.q("list-panes", "-t", winB, "-F", "#{pane_id}") })
	if !ok {
		t.Fatal("window B has no pane")
	}

	// Window B is active here, so opening A's sidebar is the case that broke.
	s.toggle(paneA)
	s.toggle(paneB)

	sbA, _ := waitUntil(settle, "%", func() string { return s.sidebarPane(winA) })
	sbB, _ := waitUntil(settle, "%", func() string { return s.sidebarPane(winB) })
	eq(t, "each window gets its own sidebar", true, sbA != "" && sbB != "" && sbA != sbB)

	path := func(pane string) func() string {
		return func() string { return resolved(s.q("display", "-p", "-t", pane, "#{pane_current_path}")) }
	}
	gotA, _ := waitUntil(settle, dirA, path(sbA))
	gotB, _ := waitUntil(settle, dirB, path(sbB))
	eq(t, "window A's sidebar starts in project A", dirA, gotA)
	eq(t, "window B's sidebar starts in project B", dirB, gotB)
}

func TestSidebarToggleStalePane(t *testing.T) {
	// A stale TMUX_PANE (e.g. inherited from another tmux server) must not be
	// swallowed: a keybind that silently does nothing is the worst outcome.
	s := newSrv(t)
	s.start("")
	r := s.toggle("%999")
	eq(t, "exits non-zero when the target pane is stale", true, r.rc != 0)
	eq(t, "opened nothing", 1, s.panes())
}

// The sidebar runs yazi with its own YAZI_CONFIG_HOME; the toggle has to inject
// it, or yazi silently falls back to its defaults and loses the narrow layout.
func TestSidebarToggleInjectsYaziConfigHome(t *testing.T) {
	s := newSrv(t)
	work := s.start("")
	s.toggle(work, "SIDEBAR_CMD="+fakeTool("yazi"))

	got, _ := waitUntil(settle, "yazi", func() string { return s.fakeFile("yazi-env.txt") })
	eq(t, "toggle launches the sidebar with YAZI_CONFIG_HOME", filepath.Join(s.home, ".config", "yazi-sidebar"), got)
}

// yazi at sidebar width, for real: with the three-column layout every name is
// squeezed into "c...". Needs the real yazi.
func TestSidebarShowsFullFilenames(t *testing.T) {
	yazi := skipUnlessYazi(t)
	s := newSrv(t)
	long := "un-nombre-de-archivo-bastante-largo.md"
	writeFile(t, filepath.Join(s.work, long), "# hi\n")
	work := s.start("")

	if r := s.toggle(work, "SIDEBAR_CMD="+yazi); r.rc != 0 {
		t.Fatalf("toggle: %s", r.out)
	}

	// Two waits, because there are two races: the sidebar pane has to exist and
	// carry @sidebar, then yazi has to paint its first frame into it.
	sidebar, ok := waitUntil(settle, "%", func() string { return s.sidebarPane("main") })
	if !ok {
		t.Fatal("no sidebar pane")
	}
	screen, _ := waitUntil(settle, long, func() string { return s.q("capture-pane", "-p", "-t", sidebar) })
	wantText(t, "shows the full filename at sidebar width", long, screen)
}

// sidebar-follow keeps the sidebar tree on the directory the work pane is in. It
// runs from a zsh chpwd hook, so it fires on EVERY directory change: it must do
// nothing at all when there is no sidebar, and never fail in a way that shows up
// in the prompt.
func TestSidebarFollow(t *testing.T) {
	follow := func(s *srv, pane, dir string, ya string) result {
		return s.aval(s.with("TMUX_PANE="+pane, "YA_BIN="+ya), "tmux", "sidebar-follow", dir)
	}

	t.Run("no sidebar: does nothing, cheaply and quietly", func(t *testing.T) {
		s := newSrv(t)
		work := s.start("")
		r := follow(s, work, s.work, fakeTool("ya"))
		eq(t, "exits cleanly when there is no sidebar", 0, r.rc)
		eq(t, "says nothing when there is no sidebar", "", r.out)
		eq(t, "does not invoke ya when there is no sidebar", "", s.fakeFile("ya-calls.txt"))
	})

	t.Run("with a sidebar: sends cd to the recorded instance", func(t *testing.T) {
		s := newSrv(t)
		work := s.start("")
		// The stand-in must be NAMED yazi: --client-id is yazi's own flag, so the
		// toggle only appends it for yazi. A generic `sleep` double would take a
		// different code path and prove nothing about the real one.
		s.toggle(work, "SIDEBAR_CMD="+fakeTool("yazi"))

		id := waitNonEmpty(func() string { return s.q("display", "-p", "-t", work, "#{@sidebar_client_id}") })
		eq(t, "the toggle stores a yazi client id", true, id != "")
		eq(t, "the stored id is numeric", true, regexp.MustCompile(`^[0-9]+$`).MatchString(id))

		s.rmFake("ya-calls.txt")
		target := filepath.Join(s.work, "some dir")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		r := follow(s, work, target, fakeTool("ya"))
		eq(t, "rc", 0, r.rc)

		calls := s.fakeFile("ya-calls.txt")
		wantText(t, "emits to the recorded instance", "emit-to "+id, calls)
		wantText(t, "emits a cd command", " cd ", calls)
		// The stand-in records its argv joined by spaces, so the path landing
		// intact proves it was passed as one argument and not split.
		wantText(t, "a directory with spaces arrives whole", target, calls)

		// The hook runs on every cd. If yazi died, or the id is stale, this still
		// has to exit 0 and print nothing, or the failure shows in the shell.
		r = follow(s, work, target, "/nonexistent/ya")
		eq(t, "exits 0 even when ya is missing", 0, r.rc)
		eq(t, "prints nothing when ya is missing", "", r.out)
	})
}

func waitNonEmpty(read func() string) string {
	var v string
	waitFor(settle, func() bool { v = read(); return v != "" })
	return v
}

// sidebar-watch is the safety net that keeps the tree in sync when the shell has
// no chpwd hook. Two properties decide whether it is acceptable: it must die
// with the sidebar (no orphan loops), and stay quiet when nothing changed.
func TestSidebarWatch(t *testing.T) {
	skipUnlessPS(t)
	s := newSrv(t)
	dirA := filepath.Join(s.work, "a")
	dirB := filepath.Join(s.work, "b")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	work := s.start(dirA)
	before := watchers()

	start := func() {
		r := s.toggle(work, "SIDEBAR_CMD="+fakeTool("yazi"), "YA_BIN="+fakeTool("ya"), "WATCH_INTERVAL=0.3")
		if r.rc != 0 {
			t.Fatalf("toggle: %s", r.out)
		}
	}

	// --- the toggle starts a watcher ---
	start()
	eq(t, "opening the sidebar starts a watcher", true, waitFor(settle, func() bool { return watchers() > before }))

	// --- it syncs a directory change without any shell hook ---
	// No chpwd hook is involved: the cwd is changed by the pane's own shell and
	// only the watcher can notice.
	s.rmFake("ya-calls.txt")
	s.must("send-keys", "-t", work, "cd '"+dirB+"'", "Enter")
	calls, ok := waitUntil(settle, dirB, func() string { return s.fakeFile("ya-calls.txt") })
	eq(t, "notices the change with no shell hook", true, calls != "")
	eq(t, "emits cd to the new directory", true, ok)

	// --- it stays quiet when nothing moves ---
	// A negative needs a window; 1.5s is five watch cycles.
	s.rmFake("ya-calls.txt")
	time.Sleep(1500 * time.Millisecond)
	eq(t, "emits nothing while the directory is unchanged", "", s.fakeFile("ya-calls.txt"))

	// --- it dies with the sidebar ---
	// An orphaned polling loop would keep waking up forever after the panel closed.
	s.toggle(work, "SIDEBAR_CMD="+fakeTool("yazi"), "YA_BIN="+fakeTool("ya"))
	eq(t, "closing the sidebar stops the watcher", true, waitFor(settle, func() bool { return watchers() == before }))

	// --- and dies if the whole server goes away ---
	start()
	eq(t, "the reopened sidebar has a watcher again", true, waitFor(settle, func() bool { return watchers() > before }))
	s.tmux("kill-server")
	eq(t, "killing tmux stops the watcher", true, waitFor(settle, func() bool { return watchers() == before }))
}

// Each tmux invocation costs ~9ms because it starts a client. With 8 sessions
// open, three calls per cycle every 2s is 15.6% of a core spent asking the same
// question three ways.
func TestSidebarWatchOneTmuxCallPerCycle(t *testing.T) {
	s := newSrv(t)
	s.start("")
	sb := s.must("split-window", "-t", "main", "-h", "-b", "-P", "-F", "#{pane_id}", "sleep 600")
	s.must("set", "-p", "-t", sb, "@sidebar", "1")

	// One iteration only: the interval is long, so the loop is killed mid-sleep.
	env := s.with("TMUX_BIN="+fakeTool("tmux-counted"), "YA_BIN="+fakeTool("ya"), "WATCH_INTERVAL=30")
	cmd := startCmd(t, env, avalBin, "tmux", "sidebar-watch", sb, "4242")
	defer cmd.stop()

	count := func() int { return strings.Count(s.fakeFile("tmux-calls.txt"), "x") }
	eq(t, "the watcher asked tmux at least once", true, waitFor(settle, func() bool { return count() >= 1 }))
	time.Sleep(300 * time.Millisecond) // room for a second call, were there one
	eq(t, "one tmux call per cycle, not three", 1, count())
}

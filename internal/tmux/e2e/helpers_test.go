//go:build !windows

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Bounded waits
//
// These suites drive a real tmux server: they toggle a sidebar, open yazi, then
// read the result back with capture-pane or display -p. Between the action and
// the read sits work the test does not control (a pane spawning, a shell
// starting, yazi painting its first frame, a directory change landing).
//
// A fixed sleep there is a bet on machine speed, and shared CI runners lose it:
// the same commit failed twice in two different suites, once with an empty pane
// and once with a path that had not updated yet, while passing every time
// locally. Rotating failures on an unchanged commit are a race, and raising the
// sleep only moves the threshold.
//
// Polling removes it: read until the value is the expected one or a deadline
// passes. Fast machines return at once; slow ones get the time they need.
// ---------------------------------------------------------------------------

// pollInterval is how often a wait re-reads. A variable so the wait helpers'
// own tests can run fast.
var pollInterval = 50 * time.Millisecond

// settle is the budget for anything the machine does asynchronously.
const settle = 15 * time.Second

// waitUntil polls read until its result contains match or timeout passes.
//
// It returns the last value read, matching or not, and whether it matched.
// Handing back the last value rather than an empty string on timeout is
// deliberate: the assertion that follows can then report what the pane really
// held. A timeout that reports nothing sends you off to re-run the suite by
// hand, and a race may not reproduce when you do.
func waitUntil(timeout time.Duration, match string, read func() string) (string, bool) {
	deadline := time.Now().Add(timeout)
	for {
		out := read()
		if strings.Contains(out, match) {
			return out, true
		}
		if !time.Now().Before(deadline) {
			return out, false
		}
		time.Sleep(pollInterval)
	}
}

// waitFor polls cond until it is true or timeout passes. For conditions that
// are not a substring of any output: two pane ids being distinct, a pane having
// appeared at all.
func waitFor(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(pollInterval)
	}
}

// stable polls read until it returns the same value for quiet, so a value that
// something else is still rewriting is not read mid-change. Returns the value.
func stable(timeout, quiet time.Duration, read func() string) string {
	deadline := time.Now().Add(timeout)
	last, since := read(), time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		if v := read(); v != last {
			last, since = v, time.Now()
		} else if time.Since(since) >= quiet {
			return last
		}
	}
	return last
}

// ---------------------------------------------------------------------------
// A real tmux server on a private socket
// ---------------------------------------------------------------------------

var serverSeq atomic.Int64

// skipUnlessTmux skips integration tests under -short or without tmux.
func skipUnlessTmux(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: needs a real tmux server (skipped under -short)")
	}
	if realTmux == "" {
		t.Skip("tmux is not installed")
	}
}

// skipUnlessYazi returns the absolute path of the real yazi, or skips.
func skipUnlessYazi(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("yazi")
	if err != nil {
		t.Skip("yazi is not installed")
	}
	return p
}

type srv struct {
	t    *testing.T
	sock string
	home string // HOME of every process: an "installed" layout in a temp dir
	work string // a working directory (symlinks resolved)
	fake string // $E2E_FAKE_DIR: where the stand-ins write what they saw
	env  []string
}

// newSrv prepares, but does not start, a tmux server of its own.
//
// HOME is laid out like an installed machine, using the very links
// install.manifest declares (.tmux.conf, .config/yazi, .config/yazi-sidebar,
// .local/bin/aval), so tmux.conf's `~/.local/bin/aval ...` bindings and the
// toggle's ~/.config/yazi-sidebar lookup resolve for real. The old bash suites
// read the developer's installed copies instead and so tested whatever happened
// to be on that machine; here the files under test are the repository's.
func newSrv(t *testing.T) *srv {
	t.Helper()
	skipUnlessTmux(t)

	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &srv{
		t:    t,
		sock: fmt.Sprintf("aval-e2e-%d-%d", os.Getpid(), serverSeq.Add(1)),
		home: filepath.Join(tmp, "home"),
		work: filepath.Join(tmp, "work"),
		fake: filepath.Join(tmp, "fake"),
	}
	for _, d := range []string{s.home, s.work, s.fake, filepath.Join(s.home, ".config"), filepath.Join(s.home, ".local", "bin")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := func(target, name string) {
		if _, err := os.Stat(target); err != nil {
			return // a missing config surfaces in the test that needs it
		}
		if err := os.Symlink(target, filepath.Join(s.home, name)); err != nil {
			t.Fatal(err)
		}
	}
	link(filepath.Join(configRoot, "home", "tmux.conf"), ".tmux.conf")
	link(filepath.Join(configRoot, "config", "yazi"), ".config/yazi")
	link(filepath.Join(configRoot, "config", "yazi-sidebar"), ".config/yazi-sidebar")
	link(avalBin, ".local/bin/aval")
	// TPM and its plugins, when this machine has them, so the status-bar
	// assertions see the same load order as a real session (the kanagawa theme
	// assigns during plugin load). Read-only use; absent is fine.
	if real, err := os.UserHomeDir(); err == nil {
		_ = os.MkdirAll(filepath.Join(s.home, ".tmux"), 0o755)
		link(filepath.Join(real, ".tmux", "plugins"), ".tmux/plugins")
	}

	s.env = s.baseEnv()
	t.Cleanup(s.shutdown)
	return s
}

// baseEnv is the environment of every process the test starts. It is built, not
// inherited: running the suite from inside tmux would otherwise leak the
// developer's TMUX_PANE into every invocation, which the helpers honour, so
// each would resolve a pane of the REAL server while talking to the private one.
func (s *srv) baseEnv() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + s.home,
		"TERM=xterm-256color",
		"SHELL=/bin/sh",
		"LANG=C",
		"TMPDIR=" + os.TempDir(),
		"TMUX_SOCKET=" + s.sock,
		"E2E_FAKE_DIR=" + s.fake,
		"E2E_REAL_TMUX=" + realTmux,
		// Watchers poll every 2s by default; a short interval keeps tests and
		// their cleanup quick. Tests about the watcher itself set their own.
		"WATCH_INTERVAL=0.2",
	}
}

// with returns the base environment plus overrides (KEY=VALUE).
func (s *srv) with(extra ...string) []string { return append(append([]string{}, s.env...), extra...) }

// shutdown kills the server and waits for its watchers, so no test leaves a
// process behind. Targeted by socket and by this run's binary path: the user's
// real tmux sessions and sidebars are never matched.
func (s *srv) shutdown() {
	cmd := exec.Command(realTmux, "-L", s.sock, "kill-server")
	cmd.Env = s.env
	_ = cmd.Run()
	// Watchers exit by themselves on the next cycle once the server is gone.
	waitFor(5*time.Second, func() bool { return watchers() == 0 })
}

// tmux runs a tmux command on the private server.
func (s *srv) tmux(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, realTmux, append([]string{"-L", s.sock}, args...)...)
	cmd.Env = s.env
	cmd.Dir = s.work
	cmd.WaitDelay = 2 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		err = fmt.Errorf("tmux %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimRight(string(out), "\n"), err
}

// q is tmux for reads inside a poll: an error reads as an empty value.
func (s *srv) q(args ...string) string {
	out, _ := s.tmux(args...)
	return out
}

func (s *srv) must(args ...string) string {
	s.t.Helper()
	out, err := s.tmux(args...)
	if err != nil {
		s.t.Fatal(err)
	}
	return out
}

// start creates session "main" in dir (default: the work dir) and returns the
// id of its only pane, once the pane exists.
func (s *srv) start(dir string, extra ...string) string {
	s.t.Helper()
	if dir == "" {
		dir = s.work
	}
	// No config unless the test brings one: the user's real tmux.conf would
	// otherwise be loaded (tmux reads ~/.tmux.conf by default), which slows every
	// server by a plugin load and couples tests to unrelated settings.
	if !contains(extra, "-f") {
		extra = append([]string{"-f", "/dev/null"}, extra...)
	}
	args := append(append([]string{}, extra...), "new-session", "-d", "-s", "main", "-c", dir, "-x", "200", "-y", "50")
	s.must(args...)
	pane, ok := waitUntil(settle, "%", func() string { return s.q("list-panes", "-t", "main", "-F", "#{pane_id}") })
	if !ok {
		s.t.Fatalf("no pane after new-session: %q", pane)
	}
	return pane
}

func (s *srv) panes() int {
	return countLines(s.q("list-panes", "-t", "main", "-F", "#{pane_id}"))
}

func (s *srv) windows() int {
	return countLines(s.q("list-windows", "-F", "#{window_id}"))
}

// paneWith finds the pane (or window) whose flag option is set.
func (s *srv) flagged(list, id, opt, target string) string {
	args := []string{list}
	if target != "" {
		args = append(args, "-t", target)
	}
	for _, line := range strings.Split(s.q(append(args, "-F", "#{"+id+"}|#{?"+opt+",1,0}")...), "\n") {
		if a, b, ok := strings.Cut(line, "|"); ok && b == "1" {
			return a
		}
	}
	return ""
}

func (s *srv) sidebarPane(target string) string {
	return s.flagged("list-panes", "pane_id", "@sidebar", target)
}

func (s *srv) fileWindow() string {
	return s.flagged("list-windows", "window_id", "@file_window", "")
}

// result of running aval.
type result struct {
	out string // stdout and stderr together
	rc  int
}

// aval runs `aval <args>` with env (default: the base environment).
func (s *srv) aval(env []string, args ...string) result {
	s.t.Helper()
	return runCmd(s.t, env, 30*time.Second, avalBin, args...)
}

// toggle runs sidebar-toggle with a stand-in command (`sleep 600`: the pane
// lifecycle of yazi without any TUI rendering) against pane.
//
// It runs the command directly, not through `run-shell -b`, which detaches and
// swallows both the exit status and stderr.
func (s *srv) toggle(pane string, extra ...string) result {
	s.t.Helper()
	env := s.with(append([]string{"SIDEBAR_CMD=sleep 600"}, extra...)...)
	if pane != "" {
		env = append(env, "TMUX_PANE="+pane)
	}
	return s.aval(env, "tmux", "sidebar-toggle")
}

func runCmd(t *testing.T, env []string, timeout time.Duration, bin string, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.WaitDelay = 2 * time.Second
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	rc := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		} else {
			t.Fatalf("%s %v: %v", bin, args, err)
		}
	}
	return result{out: strings.TrimRight(buf.String(), "\n"), rc: rc}
}

// fakeTool is the path of a stand-in executable.
func fakeTool(name string) string { return filepath.Join(fakeBin, name) }

// fakeFile reads a file a stand-in wrote ("" until it exists).
func (s *srv) fakeFile(name string) string {
	b, _ := os.ReadFile(filepath.Join(s.fake, name))
	return string(b)
}

func (s *srv) rmFake(name string) { _ = os.Remove(filepath.Join(s.fake, name)) }

// watchers counts running sidebar-watch processes of THIS run's aval binary.
// Matching on the binary's unique temp path means a developer's real sidebars
// are never counted, and nothing is killed by pattern.
func watchers() int {
	out, err := exec.Command("ps", "-eo", "args=").Output()
	if err != nil {
		return 0
	}
	needle := avalBin + " tmux sidebar-watch"
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), needle) {
			n++
		}
	}
	return n
}

func skipUnlessPS(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ps"); err != nil {
		t.Skip("ps is not available to observe processes")
	}
	if runtime.GOOS == "windows" {
		t.Skip("no process listing")
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func countLines(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

// resolved returns p with symlinks resolved: on macOS /tmp is
// a symlink to /private/tmp, so tmux reports one spelling and a test holds the
// other.
func resolved(p string) string {
	if p == "" {
		return ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func eq(t *testing.T, name string, want, got any) {
	t.Helper()
	if want != got {
		t.Errorf("%s:\n  want %#v\n  got  %#v", name, want, got)
	}
}

func wantText(t *testing.T, name, want, got string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("%s: want text containing %q, got %q", name, want, got)
	}
}

// proc is a long-running child the test stops itself.
type proc struct {
	t   *testing.T
	cmd *exec.Cmd
}

func startCmd(t *testing.T, env []string, bin string, args ...string) *proc {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &proc{t: t, cmd: cmd}
	t.Cleanup(p.stop)
	return p
}

func (p *proc) stop() {
	if p.cmd.ProcessState != nil {
		return
	}
	_ = p.cmd.Process.Kill()
	_ = p.cmd.Wait()
}

// probe runs a tool bounded at 15s with no stdin and no controlling terminal and
// returns its combined output ("" when it is missing, fails or times out: a probe
// that does not answer in time is the same skip as one that reports nothing).
func probe(t *testing.T, env []string, name string, args ...string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.WaitDelay = time.Second
	out, _ := cmd.CombinedOutput()
	return string(out)
}

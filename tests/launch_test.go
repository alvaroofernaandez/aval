package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// `aval tmux launch`, the Ghostty launch command. Ghostty starts it with
// launchd's minimal PATH, so a bare `tmux` is not found; these cases pin that
// down against the real binary. Hermetic variants (fake tmux, no server) live
// in internal/term/tmux; the cases here use the real tmux where it matters.

const minimalPath = "/usr/bin:/bin:/usr/sbin:/sbin"

func launchEnv(home string, kv ...string) []string {
	// env -i equivalent: nothing but what the launcher is given.
	return append([]string{"HOME=" + home}, kv...)
}

func TestLaunchResolvesTmuxUnderLaunchdMinimalPath(t *testing.T) {
	testenv.SkipWindows(t, "tests the tmux/Ghostty launcher")
	needTool(t, "tmux")
	r := run(repo, launchEnv(t.TempDir(), "PATH="+minimalPath, "LAUNCH_RESOLVE_ONLY=1"), "", avalBin(t), "tmux", "launch")
	resolved := strings.TrimSpace(r.Stdout)
	st, err := os.Stat(resolved)
	isTrue(t, "resolves tmux with launchd's minimal PATH: "+resolved, resolved != "" && err == nil && st.Mode()&0o111 != 0)
	mustContain(t, resolved, "tmux", "resolves to a real tmux binary")
}

func TestLaunchFallsBackInsteadOfLeavingADeadTerminal(t *testing.T) {
	testenv.SkipWindows(t, "tests the tmux/Ghostty launcher")
	aval := avalBin(t)
	tmp := t.TempDir()
	env := launchEnv(tmp, "PATH="+tmp, "TMUX_BIN="+tmp+"/absent", "LAUNCH_RESOLVE_ONLY=1")
	r := run(repo, env, "", aval, "tmux", "launch")
	eq(t, "reports no tmux rather than erroring out (exit)", 0, r.Code)
	eq(t, "resolves to nothing when tmux is absent", "", r.Out())

	// With no tmux anywhere, it must still hand over a working shell.
	shell := filepath.Join(tmp, "myshell")
	write(t, shell, "#!/bin/sh\necho FELL_BACK\n")
	_ = os.Chmod(shell, 0o755)
	r = run(repo, launchEnv(tmp, "PATH="+tmp, "SHELL="+shell, "TMUX_BIN="+tmp+"/absent"), "", aval, "tmux", "launch")
	mustContain(t, r.Out(), "FELL_BACK", "falls back to a login shell when tmux is missing")
}

func TestLaunchExplicitOverrideWins(t *testing.T) {
	testenv.SkipWindows(t, "tests the tmux/Ghostty launcher")
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "tmux")
	write(t, fake, "#!/bin/sh\necho \"TMUX_RAN $*\"\n")
	_ = os.Chmod(fake, 0o755)
	r := run(repo, launchEnv(tmp, "PATH="+minimalPath, "TMUX_BIN="+fake), "", avalBin(t), "tmux", "launch")
	out := r.Out()
	mustContain(t, out, "TMUX_RAN", "runs tmux when available")
	// A session of its own, not a shared one: tabs attaching to the same session
	// render the same window and stop being independent workspaces.
	mustContain(t, out, "new-session", "starts a session rather than joining a shared one")
	mustNotHave(t, `-A -s main`, out, "forces every tab onto one named session")
}

// Each Ghostty tab is a separate tmux CLIENT. Attaching them all to one session
// means they render the same window: changing directory in one changes the
// other, the opposite of working on two projects in parallel.
func TestLaunchEveryTabGetsItsOwnSession(t *testing.T) {
	testenv.SkipWindows(t, "tests the tmux/Ghostty launcher")
	t.Parallel()
	needTool(t, "tmux")
	aval := avalBin(t)
	tmp := t.TempDir()
	socket := "launch-sess-" + filepath.Base(tmp)
	sess := func(args ...string) result {
		return run(tmp, nil, "", "tmux", append([]string{"-L", socket}, args...)...)
	}
	sess("kill-server")
	t.Cleanup(func() { sess("kill-server") })

	// The launcher is asked what it WOULD attach to, rather than actually execing.
	pick := func() string {
		r := run(repo, launchEnv(tmp, "PATH="+minimalPath, "TMUX_SOCKET="+socket, "LAUNCH_PICK_ONLY=1"), "", aval, "tmux", "launch")
		return strings.TrimSpace(r.Stdout)
	}

	eq(t, "with no sessions, it creates one", "new", pick())

	sess("new-session", "-d", "-s", "alpha")
	// The server registers the session asynchronously: poll, bounded.
	got := ""
	eventually(10*time.Second, func() bool { got = pick(); return got == "alpha" })
	eq(t, "reuses a session that has no client", "alpha", got)

	sess("new-session", "-d", "-s", "beta")
	listing := sess("list-sessions", "-F", "#{session_name}|#{session_attached}").Stdout
	isTrue(t, "a detached session is detectable as free", strings.Contains(listing, "alpha|0"))

	// A session with a client is never picked: whatever pick returns is "new" or
	// a session that really has no client.
	got = pick()
	if got != "new" {
		isTrue(t, "never picks a session that already has a client: "+got,
			regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(got)+`\|0`).MatchString(listing))
	}
}

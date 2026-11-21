package tmux

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// sidebarToggle toggles a file-tree sidebar pane in the current tmux window.
//
// Open  -> splits a narrow pane on the left running $SIDEBAR_CMD (default:
// yazi), inheriting the working pane's directory, and focuses it.
// Close -> kills that pane and returns focus to the working pane.
//
// The pane is tagged with the @sidebar pane option, so the toggle never has to
// guess which pane is the sidebar and never touches a pane the user opened.
func sidebarToggle(env Env) int {
	home := env.Getenv("HOME")
	cmd := orDefault(env.Getenv("SIDEBAR_CMD"), "yazi")
	width := orDefault(env.Getenv("SIDEBAR_WIDTH"), "30%")
	cfgHome := orDefault(env.Getenv("SIDEBAR_CONFIG_HOME"), home+"/.config/yazi-sidebar")

	// A bare command name is resolved to an absolute path. split-window runs
	// the command with the tmux SERVER's PATH — launchd's minimal one when
	// Ghostty started the server — so an unresolvable name makes tmux open the
	// pane and close it the instant the exec fails. That reads as the sidebar
	// flickering open and vanishing, not as an error.
	cmd = resolveFirstOnly(env, cmd, toolDirs(home))

	bin := tmuxBin(env)
	if bin == "" {
		fmt.Fprintln(env.Stderr, "sidebar-toggle: tmux not found")
		return 127
	}
	tm := newRunner(env, bin)

	// Anchor every lookup to the window the toggle was fired from, so other
	// windows and sessions are never considered.
	window := env.Getenv("TMUX_PANE")
	if window == "" {
		out, _ := tm.Run("display", "-p", "#{window_id}")
		window = trim(out)
	}

	// TMUX_PANE can be stale when inherited from another tmux server. Acting on
	// a fallback target would open the sidebar in the wrong window, so refuse
	// loudly: a keybind that silently does nothing is impossible to diagnose.
	if _, err := tm.Query("display", "-p", "-t", window, "#{window_id}"); err != nil {
		fmt.Fprintf(env.Stderr, "sidebar-toggle: no such pane or window: %s\n", window)
		return 1
	}

	// @sidebar is normalised to 1/0 rather than read raw: an unset option
	// renders as an empty field, and splitting silently shifts later columns.
	list, _ := tm.Run("list-panes", "-t", window, "-F", "#{pane_id}|#{?@sidebar,1,0}")
	for _, line := range strings.Split(trim(list), "\n") {
		if id, flag, ok := strings.Cut(line, "|"); ok && flag == "1" {
			if _, err := tm.Run("kill-pane", "-t", id); err != nil {
				return 1
			}
			return 0
		}
	}

	var envArgs []string
	// Point yazi at the sidebar-only config (narrow, tree-only layout). Skipped
	// when that directory is absent: yazi would silently fall back to its
	// built-in defaults and lose the opener, so plain ~/.config/yazi is safer.
	if env.IsDir(cfgHome) {
		envArgs = []string{"-e", "YAZI_CONFIG_HOME=" + cfgHome}
	}

	// A per-window client id lets sidebar-follow steer THIS instance as the work
	// pane changes directory. yazi requires it to be globally unique.
	//
	// Only appended for yazi: --client-id is its flag, and passing it to any
	// other command makes that command fail its argument parsing and the pane
	// dies instantly.
	clientID := ""
	launchCmd := cmd
	if strings.HasSuffix(cmd, "yazi") {
		clientID = strconv.Itoa(env.Rand(60000) + 2000)
		launchCmd = cmd + " --client-id " + clientID
	}

	// The directory is resolved against the TARGET pane and passed literally.
	// `-c '#{pane_current_path}'` looks right but expands against the ACTIVE
	// pane, not the one in -t: a sidebar in a background window then inherited
	// the foreground window's directory.
	cwd, _ := tm.Query("display", "-p", "-t", window, "#{pane_current_path}")
	cwd = trim(cwd)
	if cwd == "" {
		cwd = home
	}

	args := []string{"split-window", "-t", window, "-h", "-b", "-l", width, "-c", cwd}
	args = append(args, envArgs...)
	args = append(args, "-P", "-F", "#{pane_id}", launchCmd)
	out, err := tm.Run(args...)
	if err != nil {
		return 1
	}
	pane := trim(out)

	if clientID != "" {
		tm.Run("set", "-w", "-t", pane, "@sidebar_client_id", clientID)
	}
	tm.Run("set", "-p", "-t", pane, "@sidebar", "1")

	// Started only after @sidebar is set: the watcher checks that tag on its
	// first iteration and exits at once if it is missing.
	if clientID != "" {
		// Safety net for shells that predate the chpwd hook, or are not zsh. It
		// exits on its own as soon as this sidebar is gone.
		if self, err := env.Executable(); err == nil {
			_ = env.Spawn(self, []string{"tmux", "sidebar-watch", pane, clientID}, []string{
				"TMUX_SOCKET=" + env.Getenv("TMUX_SOCKET"),
				"TMUX_BIN=" + bin,
				"YA_BIN=" + env.Getenv("YA_BIN"),
				"WATCH_INTERVAL=" + orDefault(env.Getenv("WATCH_INTERVAL"), "2"),
			})
		}
	}
	return 0
}

// sidebarFollow keeps the sidebar tree on the directory the work pane is
// actually in.
//
// Called from a zsh chpwd hook, so it fires on EVERY directory change. Two
// properties matter as much as the feature: it must cost nothing when there is
// no sidebar, and it must never fail loudly — an error here would surface in
// the user's prompt on every cd.
func sidebarFollow(args []string, env Env) int {
	target := env.Getenv("PWD")
	if len(args) > 0 && args[0] != "" {
		target = args[0]
	}

	// Nothing to do outside tmux. Checked first because it is the cheapest exit.
	if env.Getenv("TMUX")+env.Getenv("TMUX_SOCKET")+env.Getenv("TMUX_PANE") == "" {
		return 0
	}

	tm := newRunner(env, tmuxBin(env))
	pane := env.Getenv("TMUX_PANE")

	// No sidebar in this window means no work and no output.
	q := []string{"display", "-p"}
	if pane != "" {
		q = append(q, "-t", pane)
	}
	out, _ := tm.Query(append(q, "#{@sidebar_client_id}")...)
	clientID := trim(out)
	if clientID == "" {
		return 0
	}

	// From here on every failure is swallowed: yazi may have been closed, the
	// id may be stale, `ya` may not exist. None of that is worth interrupting a
	// cd for.
	if ya := yaBin(env); env.IsExec(ya) {
		_, _ = env.Output(ya, "emit-to", clientID, "cd", target)
	}
	return 0
}

// sidebarWatch keeps the sidebar tree on the work pane's directory, without
// relying on the shell.
//
// The zsh chpwd hook is instant and costs nothing, but it only exists in shells
// that were started after it was installed. Any older shell, or a shell that
// is not zsh, would silently stop syncing. This loop covers those cases.
//
// It only runs while its sidebar exists: every iteration re-checks, and the
// loop exits as soon as the pane, the window option, or the tmux server is
// gone. An orphaned polling loop would keep waking up forever.
func sidebarWatch(args []string, env Env) int {
	if len(args) < 2 || args[0] == "" || args[1] == "" {
		return 1
	}
	pane, clientID := args[0], args[1]
	interval := 2 * time.Second
	if v := env.Getenv("WATCH_INTERVAL"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			interval = time.Duration(math.Round(f * float64(time.Second)))
		}
	}

	tm := newRunner(env, tmuxBin(env))
	ya := yaBin(env)
	last := ""

	for {
		// ONE tmux call per cycle. Each invocation starts a client and costs
		// ~9ms, so with eight sessions open the earlier three-call version
		// burned 15.6% of a core asking the same question three ways. This
		// single listing answers all of them: a failure means the server or
		// window is gone, an absent sidebar row means the panel closed, and the
		// remaining row carries the work directory.
		info, err := tm.Query("list-panes", "-t", pane, "-F", "#{pane_id}|#{?@sidebar,1,0}|#{pane_current_path}")
		info = trim(info)
		if err != nil || info == "" {
			return 0
		}

		// Our sidebar must still be there and still be tagged.
		if !strings.Contains(info, pane+"|1|") {
			return 0
		}

		// The work pane is the one in this window that is not the sidebar.
		work := ""
		for _, line := range strings.Split(info, "\n") {
			f := strings.SplitN(line, "|", 3)
			if len(f) == 3 && f[1] != "1" {
				work = f[2]
				break
			}
		}

		if work != "" && work != last {
			// Skipped on the first pass: the sidebar already opened in that
			// directory, so emitting there would be a pointless round trip.
			if last != "" && env.IsExec(ya) {
				_, _ = env.Output(ya, "emit-to", clientID, "cd", work)
			}
			last = work
		}

		env.Sleep(interval)
	}
}

func yaBin(env Env) string {
	return resolve(env, orDefault(env.Getenv("YA_BIN"), "ya"), toolDirs(env.Getenv("HOME")), false)
}

// resolveFirstOnly resolves a bare command; anything that is already a path or
// carries arguments is left alone.
func resolveFirstOnly(env Env, cmd string, dirs []string) string {
	if strings.ContainsAny(cmd, "/ ") {
		return cmd
	}
	for _, d := range dirs {
		if p := d + "/" + cmd; env.IsExec(p) {
			return p
		}
	}
	return cmd
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

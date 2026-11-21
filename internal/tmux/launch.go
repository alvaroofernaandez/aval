package tmux

import (
	"fmt"
	"strings"
)

// launch is Ghostty's startup command: hand the window to tmux, or to a shell
// if tmux is unavailable.
//
// Ghostty runs it through `/usr/bin/login -flp <user> /bin/bash --noprofile
// --norc`, which inherits launchd's PATH (/usr/bin:/bin:/usr/sbin:/sbin)
// rather than the shell's. A bare `tmux` is NOT found there: the window dies
// and there is no terminal left to fix it from. So the binary is resolved by
// absolute path, and anything unexpected falls back to a login shell instead
// of leaving a dead window.
//
// TMUX_BIN overrides the binary. LAUNCH_RESOLVE_ONLY=1 prints what would run
// and exits; LAUNCH_PICK_ONLY=1 prints the session it would attach to.
func launch(env Env) int {
	tmuxPath := ""
	if v := env.Getenv("TMUX_BIN"); v != "" {
		// An explicit override is authoritative: if it is not usable, fall back
		// rather than quietly running some other tmux the caller did not ask for.
		if env.IsExec(v) {
			tmuxPath = v
		}
	} else {
		tmuxPath = findTmux(env)
	}

	if env.Getenv("LAUNCH_RESOLVE_ONLY") == "1" {
		if tmuxPath != "" {
			fmt.Fprintln(env.Stdout, tmuxPath)
		}
		return 0
	}

	if tmuxPath != "" {
		socket := env.Getenv("TMUX_SOCKET")
		tm := env.NewRunner(tmuxPath, socket)

		// Every Ghostty tab is a separate tmux client. Attaching them all to
		// one session makes them render the SAME window: a cd in one tab moves
		// the other, which defeats working on two projects side by side.
		//
		// So each tab takes a session of its own. A session with no client
		// attached is reused first, which is what makes work survive closing a
		// tab; only when they are all in use is a new one created.
		out, _ := tm.Query("list-sessions", "-F", "#{session_name}|#{session_attached}")
		target := ""
		for _, line := range strings.Split(trim(out), "\n") {
			if name, attached, ok := strings.Cut(line, "|"); ok && attached == "0" {
				target = name
				break
			}
		}

		if env.Getenv("LAUNCH_PICK_ONLY") == "1" {
			if target == "" {
				target = "new"
			}
			fmt.Fprintln(env.Stdout, target)
			return 0
		}

		argv := []string{tmuxPath}
		if socket != "" {
			argv = append(argv, "-L", socket)
		}
		if target != "" {
			argv = append(argv, "attach", "-t", target)
		} else {
			argv = append(argv, "new-session")
		}
		return execOrReport(env, tmuxPath, argv)
	}

	// No tmux: never leave the user without a terminal.
	shell := orDefault(env.Getenv("SHELL"), "/bin/zsh")
	path := shell
	if !strings.Contains(path, "/") {
		if p, err := env.LookPath(path); err == nil {
			path = p
		}
	}
	return execOrReport(env, path, []string{path, "-l"})
}

func execOrReport(env Env, path string, argv []string) int {
	if err := env.Exec(path, argv); err != nil {
		fmt.Fprintf(env.Stderr, "launch: exec %s: %v\n", path, err)
		return 1
	}
	return 0
}

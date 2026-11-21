package tmux

import (
	"fmt"
	"path/filepath"
	"strings"
)

// closeFile closes a window opened by openFile, without typing the editor's
// own quit command.
//
// Two rules make this safe:
//
//  1. Only windows tagged @file_window by openFile are eligible. A window you
//     were working in is never closed, whatever is running in it.
//  2. A vim-family editor is ASKED to quit (:q) rather than killed, so unsaved
//     changes stay the editor's decision. If it refuses, the window stays and
//     the editor says why.
//
// Anything else is closed directly: it was opened here and holds nothing the
// editor itself would object to losing.
func closeFile(env Env) int {
	tm := newRunner(env, tmuxBin(env))

	pane := env.Getenv("TMUX_PANE")
	if pane == "" {
		out, _ := tm.Query("display", "-p", "#{pane_id}")
		pane = trim(out)
	}
	if pane == "" {
		fmt.Fprintln(env.Stderr, "close-file: no target pane")
		return 1
	}

	tagged, _ := tm.Query("display", "-p", "-t", pane, "#{?@file_window,1,0}")
	if trim(tagged) != "1" {
		fmt.Fprintln(env.Stderr, "close-file: this window was not opened from the sidebar — refusing to close it")
		return 1
	}

	current, _ := tm.Query("display", "-p", "-t", pane, "#{pane_current_command}")
	current = trim(current)

	switch current {
	case "nano", "pico":
		// ^X is nano's own Exit. With unsaved changes it asks "Save modified
		// buffer?" and waits, so nothing is discarded behind the user's back.
		tm.Run("send-keys", "-t", pane, "C-x")
		fmt.Fprintf(env.Stdout, "asked %s to quit\n", current)
	case "nvim", "vim", "view", "vi":
		// Leave whatever mode it is in, then ask it to quit. An editor with
		// unsaved changes will refuse and say so, which is the point.
		tm.Run("send-keys", "-t", pane, "Escape")
		tm.Run("send-keys", "-t", pane, ":q", "Enter")
		fmt.Fprintf(env.Stdout, "asked %s to quit\n", current)
	default:
		tm.Run("kill-window", "-t", pane)
	}
	return 0
}

// openFile opens files from the yazi sidebar in a NEW tmux window.
//
// A new window rather than the current one on purpose: whatever is running
// where you were — a build, a REPL, an editor with unsaved changes — is never
// touched, never receives keystrokes, and never has to be interrupted.
//
// Outside tmux it falls back to running the editor here, so plain fullscreen
// `yazi` keeps working.
func openFile(files []string, env Env) int {
	if len(files) == 0 {
		fmt.Fprintln(env.Stderr, "usage: open-file <file>...")
		return 2
	}

	// Deliberately NOT $EDITOR. That may be vim/nvim, which needs :q to leave; the
	// requirement for this flow is a file you can close with Ctrl+X. nano does
	// that natively and asks "Save modified buffer?" instead of discarding.
	editorCmd := env.Getenv("FILE_EDITOR")
	if editorCmd == "" {
		editorCmd = "nano"
	}

	// Absolute paths throughout: this runs from yazi, which inherits the tmux
	// server's environment — launchd's minimal PATH when Ghostty started it. An
	// unresolvable name would make tmux open the window and close it instantly.
	editorBin := resolve(env, editorCmd, toolDirs(env.Getenv("HOME")), true)
	bin := tmuxBin(env)

	tm := newRunner(env, bin)
	if bin == "" || !env.IsExec(bin) || !hasSession(tm) {
		return runEditorHere(env, editorBin, files)
	}

	// exec: tmux runs this through sh, and dash (Linux's /bin/sh) does not
	// replace itself with the last command, so the pane would report "sh" and
	// closeFile could not tell the editor is there.
	quoted := "exec " + shellQuote(editorBin)
	for _, f := range files {
		quoted += " " + shellQuote(f)
	}
	name := filepath.Base(files[0])

	out, err := tm.Run("new-window", "-P", "-F", "#{window_id}", "-n", name, quoted)
	if err != nil {
		return 1
	}
	// Tagged so closeFile can tell an opened file from a window you were
	// working in. Guessing by window name or contents would eventually close
	// real work.
	tm.Run("set", "-w", "-t", trim(out), "@file_window", "1")
	return 0
}

func hasSession(tm Runner) bool {
	_, err := tm.Query("has-session")
	return err == nil
}

// runEditorHere replaces this process with the editor. The editor may carry
// arguments ("nvim -R"), so it is split like an unquoted shell word.
func runEditorHere(env Env, editor string, files []string) int {
	argv := strings.Fields(editor)
	if len(argv) == 0 {
		return 1
	}
	path := argv[0]
	if !strings.Contains(path, "/") {
		if p, err := env.LookPath(path); err == nil {
			path = p
		}
	}
	argv[0] = path
	if err := env.Exec(path, append(argv, files...)); err != nil {
		fmt.Fprintln(env.Stderr, "open-file:", err)
		return 127
	}
	return 0
}

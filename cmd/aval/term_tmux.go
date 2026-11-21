package main

import "github.com/alvaroofernaandez/aval/internal/term/tmux"

// runTmux dispatches `aval tmux <name>` and returns the exit code. These
// helpers are called by tmux, Ghostty, yazi and a zsh hook with launchd's
// minimal PATH, so they need no repository and print only what tmux should show.
func runTmux(args []string) int { return tmux.Run(args, tmux.OSEnv()) }

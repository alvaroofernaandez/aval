package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/alvaroofernaandez/aval/internal/lessons"
	"github.com/alvaroofernaandez/aval/internal/repoprofile"
)

// runHook is `aval hook <name>`: agent-hook entry points that must be fast and
// must never fail the tool call that triggered them. They run before the repo
// or the update check are even looked at, so they need no install.manifest.
func runHook(args []string, in io.Reader, out io.Writer) error {
	if len(args) > 0 && args[0] == "repo-profile" {
		repoprofile.Run(in, out) // SessionStart; never fails the session
		return nil
	}
	if len(args) == 0 || args[0] != "lessons" {
		return fmt.Errorf("usage: aval hook lessons|repo-profile (reads hook JSON on stdin)")
	}
	// Exit 0 whatever happens: a lessons hook that can fail is a hook that can
	// be mistaken for a block.
	_ = lessons.Run(in, out, lessonsOptions())
	return nil
}

func lessonsOptions() lessons.Options {
	home, _ := os.UserHomeDir()
	dir := os.Getenv("AVAL_LESSONS_DIR")
	if dir == "" && home != "" {
		dir = filepath.Join(home, ".claude", "lessons")
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" && home != "" {
		state = filepath.Join(home, ".local", "state")
	}
	if state != "" {
		state = filepath.Join(state, "aval", "lessons")
	}
	return lessons.Options{Dir: dir, StateDir: state}
}

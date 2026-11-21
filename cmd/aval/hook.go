package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/alvaroofernaandez/aval/internal/designack"
	"github.com/alvaroofernaandez/aval/internal/designhook"
	"github.com/alvaroofernaandez/aval/internal/lessons"
	"github.com/alvaroofernaandez/aval/internal/repoprofile"
	"github.com/alvaroofernaandez/aval/internal/rtkhook"
)

// runHook is `aval hook <name>`: agent-hook entry points that must be fast and
// must never fail the tool call that triggered them. They run before the repo
// or the update check are even looked at, so they need no install.manifest.
func runHook(args []string, in io.Reader, out io.Writer) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	switch name {
	case "repo-profile":
		repoprofile.Run(in, out) // SessionStart; never fails the session
	case "design-gate":
		// PreToolUse, sync: fails open, exits 0 whatever happens.
		cache, err := designack.CacheDir()
		if err != nil {
			return nil
		}
		designhook.Gate(in, out, designhook.GateOptions{
			Off: os.Getenv("DESIGN_PIPELINE_OFF") != "", Cache: cache, TTL: designack.TTLFromEnv(), Now: time.Now(),
		})
	case "design-shot-stop":
		cache, err := designack.CacheDir()
		if err != nil {
			return nil
		}
		designhook.ShotStop(in, out, designhook.StopOptions{Off: os.Getenv("DESIGN_PIPELINE_OFF") != "", Cache: cache})
	case "rtk-rewrite":
		rtkhook.Run(in, out, os.Stderr, rtkhook.Lookup())
	case "lessons":
		// Exit 0 whatever happens: a lessons hook that can fail is a hook that
		// can be mistaken for a block.
		_ = lessons.Run(in, out, lessonsOptions())
	default:
		return fmt.Errorf("usage: aval hook lessons|repo-profile|design-gate|design-shot-stop|rtk-rewrite (reads hook JSON on stdin)")
	}
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

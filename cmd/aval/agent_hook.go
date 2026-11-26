package main

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/alvaroofernaandez/aval/internal/agent/fmthook"
	"github.com/alvaroofernaandez/aval/internal/agent/lessons"
	"github.com/alvaroofernaandez/aval/internal/agent/rateguard"
	"github.com/alvaroofernaandez/aval/internal/agent/repoprofile"
	"github.com/alvaroofernaandez/aval/internal/agent/rtkhook"
	"github.com/alvaroofernaandez/aval/internal/deck"
	"github.com/alvaroofernaandez/aval/internal/design/ack"
	"github.com/alvaroofernaandez/aval/internal/design/hook"
)

// runHook is `aval hook <name>`: agent-hook entry points that must be fast and
// must never fail the tool call that triggered them. They run before the repo
// or the update check are even looked at, so they need no install.manifest.
func runHook(args []string, c cmdIO) error {
	in, out, getenv := c.stdin, c.stdout, c.env
	home := func() (string, error) { return userHomeDir(getenv) }
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	switch name {
	case "repo-profile":
		repoprofile.Run(in, out) // SessionStart; never fails the session
	case "design-gate":
		// PreToolUse, sync: fails open, exits 0 whatever happens.
		cache, err := ack.CacheDirFrom(getenv, home)
		if err != nil {
			return nil
		}
		hook.Gate(in, out, hook.GateOptions{
			Off: getenv("DESIGN_PIPELINE_OFF") != "", Cache: cache, TTL: ack.TTLFrom(getenv), Now: time.Now(),
		})
	case "plan-gate":
		// PreToolUse, sync: fails open, exits 0 whatever happens.
		cache, err := deck.PlanCacheDirFrom(getenv, home)
		if err != nil {
			return nil
		}
		deck.Gate(in, out, deck.GateOptions{Off: getenv("AVAL_PLAN_DECK_OFF") != "", Now: time.Now(), Cache: cache})
	case "design-shot-stop":
		cache, err := ack.CacheDirFrom(getenv, home)
		if err != nil {
			return nil
		}
		hook.ShotStop(in, out, hook.StopOptions{Off: getenv("DESIGN_PIPELINE_OFF") != "", Cache: cache})
	case "rate-guard":
		// PreToolUse on Bash, sync: fails open, exits 0 whatever happens.
		rateguard.Run(in, out, func() ([]string, error) { return rateGuardHosts(getenv) })
	case "rtk-rewrite":
		rtkhook.Run(in, out, c.stderr, rtkhook.Lookup())
	case "pre-push":
		return runPrePush(args[1:], in, out, c.stderr, getenv)
	case "fmt":
		// PostToolUse, sync: formats the edited file; a failure is one line on
		// stderr and never fails the edit.
		fmthook.Run(in, c.stderr, fmthook.DefaultOptions())
	case "relay-precompact", "relay-sessionstart", "relay-prompt", "relay-sessionend":
		relayHook(name, c)
	case "lessons":
		// Exit 0 whatever happens: a lessons hook that can fail is a hook that
		// can be mistaken for a block.
		_ = lessons.Run(in, out, lessonsOptions(getenv))
	default:
		return newUsage("usage: aval hook lessons|repo-profile|relay-precompact|relay-sessionstart|relay-prompt|relay-sessionend|design-gate|plan-gate|design-shot-stop|rate-guard|rtk-rewrite|pre-push|fmt (reads hook JSON on stdin)")
	}
	return nil
}

func lessonsOptions(getenv func(string) string) lessons.Options {
	home, _ := userHomeDir(getenv)
	dir := getenv("AVAL_LESSONS_DIR")
	if dir == "" && home != "" {
		dir = filepath.Join(home, ".claude", "lessons")
	}
	state := getenv("XDG_STATE_HOME")
	if state == "" && home != "" {
		state = filepath.Join(home, ".local", "state")
	}
	if state != "" {
		state = filepath.Join(state, "aval", "lessons")
	}
	return lessons.Options{Dir: dir, StateDir: state}
}

// rateGuardHosts reads only the rate-guard hosts file and env, never the aval
// config: the hook runs before every Bash call.
func rateGuardHosts(getenv func(string) string) ([]string, error) {
	home, _ := userHomeDir(getenv)
	return rateguard.Hosts(getenv, os.ReadFile, runtime.GOOS, home)
}

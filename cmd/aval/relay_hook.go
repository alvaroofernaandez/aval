package main

import (
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay/adapter/claudehook"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/facts"
)

// relayHookRecent is how recent a journal entry must be for SessionStart to
// take it as the mod's relay of this compaction. Spike #63 S5 measured the
// SessionStart hooks 67 to 312 ms after the mod's answer.
const relayHookRecent = 10 * time.Second

// relayHook is `aval hook relay-precompact|relay-sessionstart`, ladder level 2
// of #65: the classic hooks that carry the capsule when the relay mod does
// not run. A hot path (ADR 0007): with AVAL_RELAY other than 1 it reads
// nothing and prints nothing; it never fails the compaction.
func relayHook(name string, c cmdIO) {
	if c.env("AVAL_RELAY") != "1" {
		return
	}
	opt := claudehook.Options{Repo: facts.Repo, CapsuleTokens: relayPolicy(c.env).CapsuleTokens}
	if store, ok := relayStore(c.env); ok {
		opt.Paused = func(dir string) bool { return store.Paused(repoRootOr(dir)) }
		opt.Relayed = name == "relay-sessionstart" && store.RelayedSince(time.Now().Add(-relayHookRecent))
	}
	if name == "relay-precompact" {
		claudehook.PreCompact(c.stdin, c.stdout, opt)
		return
	}
	claudehook.SessionStart(c.stdin, c.stdout, opt)
}

// repoRootOr is the repository root of dir, or dir outside a repository.
func repoRootOr(dir string) string {
	if root := facts.Root(dir); root != "" {
		return root
	}
	return dir
}

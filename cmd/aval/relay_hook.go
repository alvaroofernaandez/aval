package main

import (
	"time"

	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/relay"

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
	if name == "relay-prompt" {
		relayPrompt(c)
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

// relayCacheTTL is the prompt cache lifetime the prompt hook assumes: the
// hook input does not carry the runtime's, and the one hour cache is what
// the relay cost model measures against.
const relayCacheTTL = time.Hour

// relayPrompt is `aval hook relay-prompt` (UserPromptSubmit): it blocks a
// prompt that would re-bill a big context on a cold cache, and warns when a
// warm context is big. Messages follow AVAL_LANG, never the config file.
func relayPrompt(c cmdIO) {
	tr := i18n.New(i18n.Select(c.env("AVAL_LANG"), c.env))
	say := func(key string) func(int, time.Duration) string {
		return func(tokens int, idle time.Duration) string {
			return tr.T(key, i18n.Args{"tokens": relay.Tokens(tokens), "idle": idle.Round(time.Minute).String()})
		}
	}
	opt := claudehook.PromptOptions{
		Policy: relayPolicy(c.env), Now: time.Now(), CacheTTL: relayCacheTTL,
		Block: say(i18n.Key("cli.relay.prompt.block")), Warn: say(i18n.Key("cli.relay.prompt.warn")),
	}
	if store, ok := relayStore(c.env); ok {
		opt.Blocks = store
		opt.Paused = func(dir string) bool { return store.Paused(repoRootOr(dir)) }
	}
	claudehook.Prompt(c.stdin, c.stdout, opt)
}

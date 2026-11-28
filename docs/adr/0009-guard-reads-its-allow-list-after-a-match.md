# 0009. The guard reads its allow-list only after a rule matched

Status: accepted (2026-11-28)

## Context

`AGENTS.md` says hooks "never load config": they are hot paths with no config
read, no exec and no network. `aval hook guard` (issue #24) denies destructive
Bash commands in bypass mode, and it needs an escape hatch: an explicit
allow-list in the private overlay (`~/.config/aval/private/guard.allow`),
never in the public repository. Reading that file on every Bash call would
put a disk read on every command, for a list that is empty for nearly all of
them.

## Decision

The guard reads `guard.allow` only after a rule has matched. A command that
breaks no rule does no read at all, which is almost every command, so the
no-op path stays pure string work. The file is not the aval config: it is one
full command per line, matched against the whole normalised command, never a
prefix.

An unreadable allow-list leaves a matched command denied (the rule matched,
and an allow could not be proved). A panic is an internal error and lets the
command through, so a bug in the guard cannot freeze a session. The hook
starts nothing and uses no network.

## Rejected alternatives

- **Load the allow-list on every call.** Costs a stat and a read for every
  Bash command to serve the rare one that is denied.
- **Cache the allow-list in memory.** The hook is a new process per call; there
  is nothing to keep it in.
- **Put the allow-list in the public repo or the aval config.** The entries
  name private paths and hosts, and the config is a hot-path no-go.
- **Prefix or glob entries.** An entry such as `rm -rf /opt` would allow
  `rm -rf /opt/../etc`; exact commands are the only safe unit.

## Consequences

- `TestAllowListIsReadOnlyAfterARuleMatched` and
  `TestRunNeverTouchesTheDiskWhenNothingMatches` pin that the no-op path reads
  nothing.
- `aval budget` holds the guard hook to 10 ms, tighter than the generic 20 ms.
- `AGENTS.md` carries a dated amendment that points here.

# 0005. The context relay is cost-aware

Status: accepted with amendment (proposed 2026-11-23, amended 2026-11-26)

## Context

The context relay (issues #63 to #71, and #74) was specified with thresholds
as a percentage of the context window. `odd/tasks/relay-cost-model.md`,
approved by the owner on 2026-11-23, amends those specs with data from one
real week of transcripts (273 worker sessions, about $1,843 API-equivalent).
It measured, among other things: a cold prompt cache makes clearing always
win; with a warm cache clearing pays only past a request-count break-even;
the best compaction point is about 200 to 230k tokens; and the session
floor is about 58k tokens. The data covers one person and Claude Code only;
quality versus context length is unmeasured.

None of this is implemented: there is no `internal/relay` package yet.

## Decision

When built, the relay will:

- trigger on absolute tokens (`SoftTokens`, `HardTokens`, starting at 160k and
  200k, to be calibrated), with the percentage kept only as a cap for small
  windows;
- read cache age and TTL from the runtime and relay at the next turn boundary
  when the cache is cold and the context is at least 100k tokens;
- relay at the hard threshold only when the cache is warm;
- count benefit net of the startup floor, and show tokens before and after
  rather than a percentage;
- claim cost savings only for the cold-cache trigger, and only once measured;
- ship a replay simulator (#74) so users calibrate thresholds on their own data.

## Rejected alternatives

- **Percentage-of-window thresholds.** On a 1M window, 60% is 600k, almost
  three times the measured optimum.
- **Forced early relay with a warm cache.** Measured between -4.6% and +1.1%,
  no reliable gain.
- **Claiming in-session replace is cheaper.** It is an inference, not a
  measurement; spike #63 decides it.

## Consequences

- The relay specs in #63 to #71 are amended by the cost model.
- Thresholds are starting values until the replay tool and quality pilot (#74)
  run.
- This ADR moves to accepted, with an amendment, when the relay ships.

## Amendment (2026-11-26): cold trigger as a hook, capsule across /clear

The relay shipped (rc.6 to rc.9), so this ADR is accepted. Two changes:

- **The cold-cache trigger advises instead of compacting.** `UserPromptSubmit`
  cannot compact (P3), but it can block. `aval hook relay-prompt` reads exact
  tokens and the last turn time from the transcript: cold cache (idle > 1 h)
  and >= `ColdMinTokens` blocks the prompt and asks for `/clear`; warm and
  >= `SoftTokens` adds a warning; resending the same prompt within 2 minutes
  passes. It fails open and is silent unless `AVAL_RELAY=1`. The 1 h TTL is
  assumed: the hook input carries no TTL.
- **The capsule survives /clear.** `SessionEnd(reason=clear)` stores the
  capsule plus the last assistant reply per working directory;
  `SessionStart(clear)` injects it once, within 10 minutes. Two sessions
  clearing in the same directory overwrite each other.
- **Exact tokens come from the transcript** (`input + cache_creation +
  cache_read` per assistant turn), also exposed as `aval context floor`.

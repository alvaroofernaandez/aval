# Relay auto-clear + leaner floor (sumi ecosystem, phase 0b)

Goal: the user never types `/clear`. Relay clears on its own at natural
boundaries and the new session continues from a capsule that is good enough
to keep working. In the same pass, trim the last user-owned parts of the
context floor. Builds on `context-floor.md` (PR #117); branch
`feat/relay-autoclear` stacks on `feat/context-floor`.

Measurement: as in `context-floor.md` (first main turn, input + cache_creation
+ cache_read). Baseline 2026-11-26: headless 29.1k, interactive 39k (before
T9 of context-floor).

Facts this plan relies on (verified 2026-11-26):
- Hooks cannot run `/clear`. The mod can: `$.command.run({command:"clear"})`
  then `$.prompt.submit`, proven headless from `turn.complete` only (relay.md
  S6). Ladder level 3 already does this, but only at the hard threshold.
- `/clear` fires SessionEnd(reason=clear) → capsule written; SessionStart
  (source=clear) injects it (context-floor T6, observed live).
- Capsule today: first/latest/recent requests (raw, uncapped), edited files,
  branch. Handoff keyed by sha256(cwd), expires after 10 min, deleted on read.
  An image-only prompt becomes the "Latest request" (`relay.IsRequest`,
  `internal/relay/capsule.go:169`).

## T0 — spike: clear from a blocked or finished turn (no code kept)
- [ ] Does a UserPromptSubmit block end the turn with `turn.complete` in the
      mod? If yes, level 3 can clear and resubmit the blocked prompt
- [ ] Does `/clear` from the mod wipe the visible last reply (scrollback)?
- [ ] Record results in this file; T4 picks its trigger from them

## T1 — floor trim (config only)
- [x] Engram protocol appears 3× (CLAUDE.md, SessionStart hook, MCP
      instructions): CLAUDE.md copy cut to one paragraph. The SessionStart
      dump is not configurable (third-party plugin script); left as is
- [x] CLAUDE.md 7.3 → 5.2 KB: TDD detail moved to lesson `strict-tdd.md`
      (source-file triggers), skills rule condensed; the shared block keeps
      the blocking keywords (`aval rules sync`, AGENTS.md mirrored)
- [x] Deny `chrome-devtools` (duplicates Claude in Chrome)
- [x] Measure: headless 29.1k → 27.8k (−1.3k)

## T2 — handoff robustness (TDD, internal/relay)
- [x] Image-only and attachment-only prompts are not requests; keep the last
      text request
- [x] Per-prompt cap in the capsule (long pastes truncated with a marker)
- [x] Handoff key = cwd + Claude Code process (the PID that survives
      /clear), so two sessions in one directory do not collide
- [x] Handoff survives until consumed or 24 h, not 10 min

## T3 — capsule quality (TDD)
- [x] Open plan tasks: unchecked boxes of the `odd/tasks/*.md` touched in
      the session (title + next unchecked items)
- [x] Working tree: uncommitted files and commits made in the session
- [x] Recent requests de-duplicated and capped; edited files keep order
- [x] Golden test on a real transcript fixture; size stays under budget

## T4 — auto-clear at boundaries (Go policy + mod plumbing)
- [x] `aval relay boundary` (Go, TDD): given usage and the last turn's
      signals (successful git commit, plan box checked), answers clear | none.
      Threshold provisional: ≥ 100k tokens (`AVAL_RELAY_BOUNDARY_TOKENS`)
- [x] Mod `turn.complete`: ask `aval relay boundary`; on clear run
      `$.command.run clear` then submit the last reply again (capsule via
      SessionEnd/SessionStart). Cold cache path per T0 result: keep advisor
      (a blocked prompt never reaches turn.complete)
- [x] AVAL_RELAY != 1 → no-op; any error → no clear
- [ ] Live check: a session crosses a commit boundary above threshold and
      continues without the user typing `/clear`

## T5 — closing
- [ ] ADR 0005 amendment (boundary trigger, handoff key, capsule fields)
- [ ] Results table; PR stacked on #117

# Phase 0 — Claude Code context floor + relay (sumi ecosystem)

Goal: lower the fixed context cost of every session and sub-agent, and make
relay the single context policy (compaction, cold-cache clear, handoff). One
change at a time, each with a before/after measurement. Source of truth:
`config/claude/` in this repo; live `~/.claude/` matches it after each task.

Measurement: first main-thread assistant turn of a fresh session, sum of
`input + cache_creation + cache_read` from the transcript JSONL. Sub-agents:
first sidechain turn. Baseline (2026-11-26, sumi-panel, after /clear): 68.7k.

Verified keys (code.claude.com docs, 2026-11-26): `autoCompactWindow` /
`CLAUDE_CODE_AUTO_COMPACT_WINDOW` (>=100k), subagent `omitClaudeMd` (v2.1.271+),
`skillOverrides` (on | name-only | user-invocable-only | off; not plugin
skills), `syncClaudeAiSkills: false` (user settings only), UserPromptSubmit
block (`decision: block` or exit 2), SessionStart matcher `clear`, SessionEnd
reason `clear` (1.5 s default timeout).

Dropped from the first draft: a standalone `autoCompactWindow 250k`. Relay
already owns the window (`aval relay enable` → hard 200k + 33k = 233k, inside
the measured 200-230k optimum); two owners would conflict.

## T0 — transcript usage reader
- [x] `internal/relay` reader: exact tokens per assistant turn
      (input + cache_creation + cache_read), last turn timestamp, main vs
      sidechain. TDD on a fixture JSONL
- [x] `aval context floor [--session <id>]` prints main and sub-agent floors

## T1 — settings: union of live and aval
- [x] aval gains the 5 live `skillOverrides` "off" (design-refactor,
      design-shotgun, one company skill (private overlay), land-and-deploy,
      remotion-best-practices); live gains the relay hooks and
      `skipAutoPermissionPrompt`; drop empty `pluginConfigs`
- [x] Sync live == aval; diff is empty

## T2 — relay on, as the one compaction owner
- [x] `aval relay enable` live (AVAL_RELAY=1, window 233k); `aval relay
      status` healthy; mod loaded

## T3 — general-purpose sub-agent without CLAUDE.md
- [x] `config/claude/agents/general-purpose.md`: `omitClaudeMd: true`,
      `model: sonnet`, ~1k-token worker core (absolute paths, rtk tools,
      English artifacts, no commits, report format)
- [x] Measure sub-agent floor before/after; confirm the override took effect

## T4 — skill listing trim
- [x] `syncClaudeAiSkills: false` if no claude.ai skill is needed locally
- [x] Rarely used user skills → `name-only`; never used → `off`
      (history audit decides). Measure floor before/after

## T5 — cold-cache advisor (relay T12 cold trigger, hook channel)
- [x] `aval hook relay-prompt` on UserPromptSubmit, reusing `Policy.Decide`
      with T0 tokens and idle time from the last turn timestamp: block when
      cold and >= 100k ("/clear, the capsule carries over"), warn when warm
      >= 160k, fail-open on any error, silent when AVAL_RELAY is off. TDD

## T6 — capsule across /clear
- [x] SessionEnd(reason=clear) writes the capsule (plus last full exchange)
      to statefs keyed by cwd; SessionStart(matcher clear) injects it once
- [x] Engram summary stays with the Engram plugin (no duplicate hook)

## T7 — closing
- [x] ADR 0005 amended (status, hook channel for the cold trigger, /clear
      capsule); `relay.md` T12 cold-trigger box updated
- [x] Results table filled

## Results
| Task | Metric | Before | After |
|------|--------|--------|-------|
| floor | main first turn, interactive | 68.7k | next fresh session (`aval context floor`) |
| all | main first turn (headless, all hooks on) | 49.4k | 45.0k (−8.8%) |
| T3 | general-purpose sub-agent first turn (headless probe) | 30.7k | 27.2k (−3.5k) |
| T4 | main first turn (headless `claude -p`) | 49.4k | 45.0k (−4.3k) |

T4 detail: 19 rarely used user skills → `name-only` (history audit of 2167
transcripts: ≤1 use or self-explanatory name); remote-ops and Kiri skills stay
`on` for their trigger phrases. claude.ai skills moved to `~/.claude/skills/.trash`.
T2: live aval was 0.2.0 without `relay`; rebuilt 0.3.0-rc.9 into `~/.local/bin`
(old binary kept as `aval.0.2.0.bak`).
Finding: `aval sync` merges only hooks, so top-level keys drifted silently (T1b).

Closing notes:
- T6 capsule is keyed by cwd: two sessions clearing in the same directory
  overwrite each other. Cold TTL assumed 1 h (hook input has no TTL).
- Not yet observed live: a real Claude Code UserPromptSubmit block and a real
  SessionEnd(clear) → SessionStart(clear) round trip (unit + e2e on copies only).
- Found while executing: `aval context` was dispatched but rejected by main's
  `knownCommand` gate; the test now pins every standalone command.

# v0.3.0 relay: #64 core, #65 Claude Code in-place relay, #66 visibility

Why: the relay issues are the last large block of "v0.3.0 · Native harness core".
Planned by a read-only advisor pass (Fable) on 2026-11-24, at main 90cb33f with
v0.3.0-rc.5 as the last tag. The executor is a fresh session: everything it needs
is in this file. "Open owner decisions" holds questions, not answers; T0 turns them
into answers before the first edit.

## What #63 proves, and what it does not

Source: `~/Documents/aval/spikes/63-relay/RESULTS.md`. Every cell is n=1, haiku,
headless: observations, not distributions.

| Fact | RESULTS row | Status | Consequence |
| :-- | :-- | :-- | :-- |
| A mod answering `session.compact` with `{messages}` replaces the context with no summariser call, for `manual` and `auto` | S1 | proven | T0 exists; adapter RED tests can use the S1 fixtures |
| A hand-built message needs `toolUses: []`; kept messages carry a stable `handle`; consecutive user messages and an assistant-first tail are accepted | S1 shapes | proven | Protocol v1 `keep` is a list of handles; the capsule message always carries `toolUses: []` |
| `$.session.compact()` from the mod (`plugin` trigger) | S1 plugin | **not run**: refused headless and from a `command.run` hook | The turn-boundary trigger of #65 has no proven delivery |
| Subagent compaction (`agentId`) | S1 agentId | not run | Subagent capsule (AC-4 of #65) is blocked |
| `context.percent` and `context.window` are the model window (1M here), never the compaction window | S3 | proven | Never trigger on `percent`; use `context.tokens` |
| `CLAUDE_CODE_AUTO_COMPACT_WINDOW=100000` gives threshold 67,000 (a 33k buffer) | S3 | proven at 100k; 250k not run | The mid-turn trigger sets the window, not the percentage override |
| `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` is a percentage of the model window, fired about 3.7k early, and the breakdown does not reflect it | S3 | partly run | Model-dependent; do not use it |
| Hook entry to return 2 ms (manual) and 46 ms (auto); `SessionStart:compact` hooks still fire after a mod answer (67 to 312 ms) | S5 | measured, n=1 | Third-party SessionStart hooks inject after a relay; doctor flags slow ones |
| `clear` then `prompt.submit` works from `turn.complete` only | S6 | proven headless | Ladder level 3 exists, at a turn boundary only |
| `$.process.run(["aval","version"])`: exit 0, 68 ms on macOS | S7 | Windows not run | About 68 ms spawn floor per call; no Windows claim |
| Exact reading: `session.measure.context.tokens` equals the transcript's input + cache_read + cache_creation | E1 | proven | Claude Code needs no estimator; `ErrPct` matters for other runtimes |
| chars/4 underestimates by 13 to 43%; D = 1.75 gives pooled p95 13.7%, in-sample | E1 | partial | The issue's "±25% uncalibrated" is wrong for chars/4 (measured p95 51%) |
| First request after a replace reads the same cache as native compaction (14,576 read; 28,051 vs 28,825 created) | E2 | measured, n=1 | "Prefix stays cached, so T0 is cheaper" is not supported |
| Floor: about 42k haiku, 62 to 71k across 10 real Opus sessions; `--tools ""` inflates to about 195k | E3 | measured | `FloorTokens` is measured per session, never a constant |

Not covered by any RESULTS row, so unverified:

- Steered compaction through `next({...e, instructions})` (ladder level 1).
- `$.ui.log`, `$.ui.toast`, `$.ui.notify` and its "sent" result (#66).
- A cache TTL reading from the mods API. The `session.measure` payload in the
  fixtures holds only `context`, `rateLimits`, `cost`, `changed`.
- Loading a plugin persistently (`CLAUDE_CODE_PLUGIN_DIRS` from the settings `env`
  block, a `.js` module). The spike only used `--plugin-dir` with `register.ts`.
- Whether `$.session.compact()` is accepted inside a `prompt.submit` hook (needed by
  the cold-cache trigger).
- The shipped type declarations were not found on this machine; nothing here is
  checked against them.

Hand-over gap: `odd/tasks/v0.3.0-batch-2.md` lists S1 `plugin` as an agent-run probe,
but it is blocked headless. It is owner-pending.

## Blocked versus unblocked

| Work | State | Waits on |
| :-- | :-- | :-- |
| #64 core, T1 to T7 | can start now | only D1, D2, D8 |
| #65 statefs and `aval relay compact`, T8 and T9 | after T7 | D3, D7 |
| #65 mod and install, T10 | after T9 | agent probe P2, D4, D5, D6 |
| #65 ladder levels 1 to 4, T11 | after T10 | agent probe P1 for level 1 |
| #65 turn-boundary and cold-cache triggers, T12 | **blocked** | owner: S1 `plugin` interactive, P3 |
| #65 subagent capsule, T12 | **blocked** | owner: S1 `agentId` |
| #65 Windows variant, T12 | **blocked** | owner: S7 on Windows |
| #64 ledger scope on Claude Code | state machine unblocked; what it tracks is **blocked** | owner: S4 |
| #66 texts, OS notifier, metrics, status, log, T13 and T14 | after T8 | nothing |
| #66 statusline segment, T15 | after T8 | nothing |
| #66 doctor, T16 | after T10 | mod heartbeat from T10 |
| #66 in-app marker, toast, notify | **blocked** | owner: S2 extended with `$.ui.*` |
| "On by default", the word "invisible", any cost claim | **blocked** | S2, E2 repeats, #74 |

## Release rule

Same as batch 2. Each `feat` with something a user can run ships as the next rc;
`aval release rc --dry-run` is the source of truth. A fix ships in the next rc
(batch 2, D16). #64 has no user surface, so T1 to T7 cut no rc (D2).

| Task | Issue | Version |
| :-- | :-- | :-- |
| T1 to T9 | #64, #65 | none, ride with rc.6 |
| T10 | #65 | v0.3.0-rc.6 |
| T11 | #65 | v0.3.0-rc.7 |
| T13 | #66 | v0.3.0-rc.8 |
| T14 | #66 | none, rides with rc.9 |
| T15 | #66 | v0.3.0-rc.9 |
| T16 | #66 | v0.3.0-rc.10 |
| T12 | #65 | v0.3.0-rc.11, when the owner probes land |

## Flow for every task

The flow of `odd/tasks/v0.3.0-batch-2.md`, steps 1 to 11, unchanged. Additions:

- Skills: `aval-architecture` for T1, T8 and T10; `design-pipeline` and
  `aval design ack` for every CLI output in T13 to T16; `aval-docs` for Markdown.
- Every task runs `go test ./tests/ -run 'Quality|Architecture'`.
- Ratchets at their ceilings: `cmd/aval/main.go` (764 lines, `dispatch` complexity
  42 and 189 lines), `internal/core/sync/sync.go` (716 lines), `statusline.Render`
  (complexity 16). New code goes in new files and helpers; no ceiling is raised.
- `aval relay` is dispatched from `runStandalone` in `cmd/aval/core_dispatch.go`.

## T0 — before the first edit

- [x] Owner answers D1 to D10; record them here as answers.
- [x] Plan and deck built (`plan-deck`); commit both on the T1 branch:
      `docs(plan): relay plan and deck`.
- [x] Hand the owner the runbook for S1 `plugin`, P3, S2 (plus `$.ui.*`), S4,
      S1 `agentId`, S7, E1 live. Nothing in T1 to T11 waits on them.
- [x] Agent probes in the spike directory, outside the repo, appended to RESULTS.md:
  - P1: steered compaction. A mod returning `next({...e, instructions})`; record
    whether the summary follows the instructions and the `usage`.
  - P2: persistent loading. Does `CLAUDE_CODE_PLUGIN_DIRS` in a settings `env` block
    load the plugin, and does a `.js` module load? Scratch settings file only.
- [x] Sanitise the S1 fixtures for the repo (D7). `s3-*` and `e2-native` contain a
      home path; the others hold real session text.

## T1 — #64 ADR amendment, package rule, cost-aware `Decide` (no release)

Branch `feat/relay-policy`. Skills: aval-architecture, aval-testing, aval-docs.
A package with no rule in `archRules` is a violation (`tests/architecture_test.go`).

Files: `docs/adr/0002-internal-layering.md` (dated amendment),
`tests/architecture_test.go`, `docs/architecture/model.md` (regenerated),
`ARCHITECTURE.md`, `internal/relay/policy.go`, `internal/relay/policy_test.go`.

- [x] Amendment: `internal/relay` is a leaf; `internal/relay/*` may import
      `internal/relay` only; `cmd/aval` wires them; `core/*`, `ui`, `agent/*` do not
      import relay.
- [x] `archRules`: `{pkg: "internal/relay"}` and
      `{pkg: "internal/relay/*", allow: ["internal/relay"]}`; regenerate the model.
- [x] Stub first: `Decide(Snapshot) Decision` returning none.
- [x] RED `TestDecide_HardTokensCapsLargeWindow`: a 1M window at 200k tokens with a
      warm cache returns relay. Expected failure: got none.
- [x] Then `_BelowSoft_None`, `_AtSoft_Precompute`, `_SmallWindowUsesPctCap`,
      `_ColdCacheAboveColdMin_Relay`, `_ColdCacheBelowColdMin_None`,
      `_WarmCacheBelowHardTokens_None`, `_NetBenefitBelowFloor_None`,
      `_UnknownCache_TreatedAsWarm`, `_UsesUpperBound`.
- [x] Defaults as provisional constants: soft 160k, hard 200k, cold minimum 100k.
- [x] The issue's `TestDecide_AtHard_Relay` (61%) is superseded by the cost model;
      say so in the PR.
- [x] Commits: `docs(adr): internal/relay is a leaf with adapters below it`,
      `feat(relay): cost-aware Decide on absolute tokens and cache age`. PR `Refs #64`.

## T2 — #64 brakes and chain (no release)

Branch `feat/relay-brakes`. Files: `internal/relay/brake.go`, `chain.go`, tests.

- [x] RED `TestBrake_SameFingerprintTwice`: same progress fingerprint twice returns
      brake with a typed reason. Expected failure: got go.
- [x] Then `_ThrashWithinMinTurns`, `_MaxChain`, `_CostCap`,
      `TestChain_EpochIncrements`, `_SessionIDMapAfterClear`.
- [x] Reasons are a closed type, asserted by value.
- [x] Commit `feat(relay): brakes and the relay chain`.

## T3 — #64 capabilities, tier, estimator (no release)

Branch `feat/relay-capabilities`. Files: `capability.go`, `estimator.go`, tests.

- [x] RED `TestTier_ClaudeNativeUI_T0`. Expected failure: got T4.
- [x] Table per vector and `RunMode`; `TestDegrade_ReplaceProbeFailed_T1` and down.
- [x] `TestEstimator_UpperBoundTriggersEarly`, `_CalibratesOnCompaction`,
      `_UnknownModelWidensBound`, `_TranscriptDivisorIsProvisional`.
- [x] Bounds from E1: exact 0 to 2%; transcript D = 1.75, `ErrPct` 15 ("in-sample,
      one person, Opus"); 50% for an unknown model.
- [x] `Capabilities` defined here as closed types (D8).
- [x] Commits: `feat(relay): closed capability model with Tier and Degrade`,
      `feat(relay): context estimator that compares its upper bound`.

## T4 — #64 message model and pair-safe tail (no release)

Branch `feat/relay-tail`. Files: `message.go`, `tail.go`, tests.

- [x] RED `TestSelectTail_NeverSplitsToolPair`. Expected failure: stub cuts by count.
- [x] Then `_RespectsBudget`, `_MidTurnKeepsInflightPair`,
      `_OversizedLastPairKeptAlone`, `_ReturnsHandlesNotCopies`.
- [x] Commit `feat(relay): pair-safe tail selection by handle`.

## T5 — #64 capsule and redaction (no release)

Branch `feat/relay-capsule`. Files: `capsule.go`, `redact.go`, `testdata/*.golden`.

- [x] RED `TestCapsuleRender_Golden`. Expected failure: empty output.
- [x] Then `_ShrinksInOrder_NoteTailObjective`, `_NoRepoOmitsGit`, `_NeverAboveBudget`.
- [x] Baseline: up to 15 previous prompts, 25 most recently edited files, last exchange.
- [x] RED `TestRedact_MasksCredentialShapes`. The relay owns this table (D1).
- [x] Commits: `feat(relay): deterministic capsule with a fixed shrink order`,
      `feat(relay): credential redaction for capsule text`.

## T6 — #64 work ledger (no release)

Branch `feat/relay-ledger`. Files: `ledger.go`, tests.

- [x] RED `TestLedger_RunningJobIsNeverRestartable`. Expected failure: restartable.
- [x] Then `_Replay`, `_Merge`, `_Reconcile_LostAndOrphaned`, `_EpochCarriesLedger`.
- [x] Job kinds are an open string until S4; state that in the PR.
- [x] Commit `feat(relay): work ledger state machine`.

## T7 — #64 protocol v1, entry point, budget (no release; closes #64)

Branch `feat/relay-protocol`. Files: `protocol.go`, `ports.go`, `handle.go`,
`bench_test.go`, `imports_test.go`, `testdata/protocol/*.json`.

- [x] RED `TestProtocol_GoldenIO`. Expected failure: passthrough.
- [x] Then `_VersionMismatch_Passthrough`, `_UnknownEvent_Passthrough`,
      `_MalformedInput_Passthrough`.
- [x] `ports.go` declares only `Store`, `Clock`, `FactsProvider`, `Notifier`,
      `Redactor`; the rest arrive with their first implementation.
- [x] RED `TestRelayImportsNoIO`: fails on `os`, `net`, `os/exec`.
- [x] `BenchmarkHandle2000Messages` against the 20 ms target; no wall-clock assertion.
- [x] Commits: `feat(relay): adapter protocol v1`,
      `test(relay): the core imports no I/O and meets its budget`. PR `Closes #64`.

## T8 — #65 state files (no release)

Branch `feat/relay-statefs`. Package `internal/relay/adapter/statefs`. State dir
`$XDG_STATE_HOME/aval/relay` (default `~/.local/state/aval/relay`),
`%LOCALAPPDATA%\aval\relay` on Windows, as `lessonsOptions` in
`cmd/aval/agent_hook.go`. Files 0600.

- [x] RED `TestJournal_PreparedThenCommitted`. Expected failure: stub reads nothing.
- [x] Then `_VoidOnResume`, `TestLock_StaleReclaimed`, `TestLock_SecondRelaySkips`,
      `TestAtomicWrite_NoPartialFile`, `TestStateFilesAre0600` (skipped on Windows).
- [x] Per-session summary `sessions/<session_id>`: one pinned line (relay count,
      tokens before and after, floor, trigger). T15 reads it.
- [x] Commit `feat(relay): journal, locks and atomic state files`.

## T9 — #65 `aval relay compact`, pause and resume (no release)

Branch `feat/relay-compact`. Files: `cmd/aval/relay.go`, `cmd/aval/relay_test.go`,
`internal/relay/adapter/claudemod`, `internal/relay/adapter/facts`,
`cmd/aval/ui_hotpaths_test.go`, `cmd/aval/testdata/relay/`.

- [x] RED `TestCompact_AnswersWithCapsuleAndHandles` on the sanitised
      `s1-auto-normal` payload. Expected failure: passthrough.
- [x] Then `_ManualTrigger`, `_ProtocolVersionMismatch_Passthrough`,
      `_PausedPassthrough`, `_SecondRelayInSessionSkips`, `_BadJSONPassthroughExitZero`.
- [x] Hot path (D3): add `{"relay","compact"}` to `TestHotPathsNeverReadConfig` and
      `TestHooksIgnoreTheLanguage`. Thresholds from env only.
- [x] `facts` reads `.git` with stat and file reads; no `git` process, no network.
- [x] `CacheTTL` is unknown to the mods API, so `Decide` gets "unknown" (warm).
- [x] `aval relay pause|resume`; text pinned in `tests/messages_test.go`.
- [x] Fuzz `FuzzRelayCompactInput` plus its step in the `fuzz` job.
- [x] Commits: `feat(relay): aval relay compact answers a compaction with a capsule and a tail`,
      `feat(relay): aval relay pause and resume`.

## T10 — #65 the mod and its install (feat → v0.3.0-rc.6)

Progress 2026-11-24: P1 and P2 passed (RESULTS.md); mod, manifest link,
`aval relay enable|disable|commit` and the ADR 0001 amendment are on branch
`feat/relay-claude-mod`. Deviation: the mod reads usage with `$.session.usage()`
inside the compact handler and makes a per-process session id (the mods API
exposes none). Merged in #111; checked in `aval dev` (PR comment), interactive TUI not covered (S2). Shipped as v0.3.0-rc.6.

Branch `feat/relay-claude-mod`. Starts only after P2 and D4, D5, D6.

Files: `config/claude/plugins/aval-relay/` (plugin.json, hooks.json, register.js,
logic.js, logic.test.js), `install.manifest`, `.github/workflows/ci.yml`
(`node --test`), `cmd/aval/relay.go` (`enable`, `disable`),
`docs/adr/0001-go-only.md` (amendment, D5), `docs/guide.md`, release notes.

- [x] Shim rules: no policy; `$` never passed to a helper declared inside
      `register`; hand-built messages have `toolUses: []`; `$.process.run` has a
      timeout; any error returns `next(e)`.
- [x] No-op path (ADR 0007): when `AVAL_RELAY` is not `1`, return `next(e)`.
- [x] Triggers: `auto` and `manual` only. (Ticked with T10 by mistake; the filter landed in T11 after the sandbox run showed a compaction during a clear.)
- [x] RED `logic.test.js`: `shapeSnapshot` builds the protocol v1 request.
- [x] RED `TestRelayEnableSetsEnvKeys`: `AVAL_RELAY=1` and
      `CLAUDE_CODE_AUTO_COMPACT_WINDOW = HardTokens + 33000`; `disable` removes them.
- [x] RED in `tests/manifest_test.go` (plugin linked) and `tests/agent_hooks_test.go`
      (repo settings hold no relay env key: off by default).
- [x] Commit-on-next-measure: `aval relay commit` on the first `session.measure`
      after an answer; appends `metrics.jsonl`.
- [x] One real interactive session by hand before merge, result in the PR.
- [x] Notes: experimental, off by default, engine-fired trigger only, macOS and
      Linux, no cost claim.
- [x] Release: dry run shows v0.3.0-rc.6, then `aval release rc --yes`.

## T11 — #65 fallback ladder, levels 1 to 4 (feat → v0.3.0-rc.7)

Branch `feat/relay-ladder`.

Progress 2026-11-24: levels 1 to 3 on the branch. Owner decisions: level 1 with
aval down uses a fixed `STEER` text in `logic.js` (aval answers `steer` itself on
`no_gain`); level 3 fires when the `max_chain` brake refuses a replace (aval answers
`new_session`, the mod compacts natively, then clears and submits the capsule at
`turn.complete` and takes a new session id). Level 2 hooks live in
`config/claude/settings.json` (the mod's plugin does not load when the mod does
not); `relay-sessionstart` is silent when a journal entry is under 10 s old. Checked in
`aval dev` (sandbox `relay11`, headless haiku, n=1): level 2 end to end, level 1 steer
and aval-down fallback, level 3 clear plus submit in one stream-json process with
`AVAL_RELAY_MAX_CHAIN=1`. The runs found and fixed: no trigger filter, the native
summary, slash-command markers and the plugin-wrapped capsule taken as requests, the
request in flight missing, and a stale capsule submitted after a queued clear. Known
limit: input already queued runs before the clear. Not run: the interactive TUI.

- [x] RED `TestLadder_BinarySlow_Level1`. Expected failure: level 0.
- [x] Then `_ModNotLoaded_Level2`, `_PolicyWantsNewSession_Level3`,
      `_AllFailed_Level4`, `_NeverSilent`.
- [x] Level 1 only if P1 passed; otherwise skipped and the PR says so.
- [x] Level 2: `aval hook relay-precompact` and `aval hook relay-sessionstart`, in
      the hot-path tests and `FuzzHookInput`; no-ops unless `AVAL_RELAY=1`.
- [x] Level 3: `clear` then `prompt.submit` from `turn.complete` only (S6).
- [x] Level 4: the lowered window already set by `enable`.
- [ ] Release: dry run shows v0.3.0-rc.7.

## T12 — #65 blocked remainder (feat → v0.3.0-rc.11 when unblocked)

- [ ] Turn-boundary trigger: needs S1 `plugin`.
- [ ] Cold-cache trigger: needs P3; if it fails, record it as not deliverable on
      Claude Code.
- [ ] Subagent capsule: needs S1 `agentId`.
- [ ] Windows exec form in `internal/core/sync/winhooks.go`: needs S7.
- [ ] PR `Closes #65` only when the surviving acceptance criteria are met.

## T13 — #66 texts, metrics, status and log (feat → v0.3.0-rc.8)

Branch `feat/relay-status`.

Progress 2026-11-24: T10 ticked "appends `metrics.jsonl`" but nothing wrote it; `aval relay
commit` now appends one metric per prepared relay. The mod sends no floor, so the texts
omit it on Claude Code; only replace answers (level 0) are journaled, so levels 1 and 3
are not in the log yet.

- [x] RED `TestNotifyText_T0_TokensBeforeAfterFloor`:
      `Relayed 212k → 61k (floor 58k) · cold cache`. Expected failure: empty string.
- [x] One golden per tier; `TestMetrics_AppendAndReadBack`,
      `TestRelayStatus_PrintsTierSourceAndBound`, `TestRelayLog_ReadsMetrics`,
      `TestRelayStatus_NoStateIsNotAnError`.
- [x] Tokens, never percentages (ADR 0005). `--json` stays English.
- [ ] Release: dry run shows v0.3.0-rc.8.

## T14 — #66 OS notifier (rides with rc.9)

Branch `feat/relay-notify`. Package `internal/relay/adapter/notify`.

Progress 2026-11-24: notify runs after the commit, so it shows the before and after figures;
the mod does not wait for it (5 s timeout, aval bounds the notifier at 3 s). macOS argv passes
the text through `on run argv`; Windows and WSL through `AVAL_NOTIFY_TEXT` (WSLENV). Checked
on macOS only: exit 0 in 0.7 s; Windows, WSL and Linux are argv-tested, not run.

- [x] RED `TestNotifier_Darwin_Command`. Expected failure: empty argv.
- [x] Then `_Linux_NotifySend`, `_Linux_FallsBackToOSC`, `_Windows_PowerShellArgs`,
      `_WSL_PowerShellExe`, `_TextIsNeverShellInterpreted`,
      `TestNotify_FailureNeverFailsTheRelay`.
- [x] `aval relay notify` is started by the shim after the answer, never inside
      `aval relay compact`. In-app `$.ui.*` waits for S2.

## T15 — #66 statusline segment (feat → v0.3.0-rc.9)

Branch `feat/relay-statusline`.

- [ ] RED `TestRender_RelaySegment`: gives `relay 3`. Expected failure: no segment.
- [ ] `TestRender_NoRelayStateIsByteIdentical`.
- [ ] The segment is a helper (`Render` at complexity 16). `statusline` reads the
      one-line file itself; `cmd/aval/relay_contract_test.go` pins the format.
- [ ] `session_id` in the statusline input only after a recorded payload shows it.
- [ ] `BenchmarkRenderWithRelay` against 1 ms; `aval budget` row in the PR.

## T16 — #66 `aval relay doctor` (feat → v0.3.0-rc.10)

Branch `feat/relay-doctor`.

- [ ] RED `TestDoctor_FlagsModNotLoaded`. Expected failure: no finding.
- [ ] Then `_FlagsWindowMismatch`, `_FlagsSlowSessionStartHooks`,
      `_PrintsCapabilityVectorAndTier`, `_ExitCodeForScripts`.
- [ ] Prints proven and unverified tiers against the probed 2.1.330.
- [ ] PR `Closes #66` only if the in-app criterion is struck by an issue comment.

## T17 — closing

- [ ] Report per task per the AGENTS.md output contract.
- [ ] Dated amendment to ADR 0005: E2 and E3 results, window instead of percentage.
- [ ] Comment on #63 with what is still not run.
- [ ] Engram: `mem_save` the outcome.

## Owner decisions, 2026-11-24

The owner accepted every recommendation below as written ("the issues were already
designed, proceed").


- **D1. `internal/relay` is a leaf with its own redaction table.** Recommended.
  `internal/agent/prepush` holds employer names, not credentials, and a leaf cannot
  import it. Tradeoff: a second pattern table.
- **D2. #64 cuts no rc.** Recommended: T1 to T7 ride with rc.6.
- **D3. Relay knobs from env only** (`AVAL_RELAY*`), as rate-guard does. Reading the
  config on a hook path is forbidden.
- **D4. Enable through `aval relay enable`, using the compaction window**
  (`CLAUDE_CODE_AUTO_COMPACT_WINDOW = HardTokens + 33000`), not
  `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` via `aval sync` (S3: it is a percentage of the
  model window). The 33k buffer is proven at 100k only.
- **D5. Dated amendment to ADR 0001 for the JS shim**: plumbing-only runtime shims a
  host's plugin API requires, tested with `node --test` in CI.
- **D6. Ship rc.6 with the `auto` trigger only.** The cold-cache trigger waits for
  the owner probes.
- **D7. Sanitised fixtures in the repo**: synthetic text in the recorded shape, no
  home path; the originals stay in the spike directory.
- **D8. `Capabilities` is defined in `internal/relay`; #8 adopts it.**
- **D9. Owner probes in one sitting**: S1 `plugin`, P3, S2 with `$.ui.*`, S4; then
  S1 `agentId`, E1 live, S7. Without a Windows machine, no Windows relay claim.
- **D10. Standing authorisation to tag v0.3.0-rc.6 to rc.11**, each after its PR is
  merged with CI green on main.

## Risks and contradictions

- **Early-access API.** All results are from 2.1.330; the doctor compares versions
  and the shim falls back to `next(e)`.
- **#65 promises more than is proven**: "background shells, subagents, monitors,
  crons and task list are untouched" has no evidence (S4 not run).
- **#65 triggers on `percent`**, which S3 shows is of the model window. Use tokens.
- **#65 cold-cache trigger on `UserPromptSubmit`**: no proven way to compact there
  and no TTL reading; ADR 0005 needs an amendment if P3 fails.
- **#64 body defaults** (soft 50%, hard 60%) are replaced by its update and ADR 0005.
- **#66 "asynchronous dispatch"**: the shim starts the notifier after the answer.
- **#66 example text** `relay 3 · 61% → 6%` contradicts ADR 0005.
- **Cost claims**: E2 shows no first-request cache advantage for T0; its measured
  value is no summariser call and no 8.5 to 10.8 s wait.
- **Third-party SessionStart hooks** fire after a mod answer; the floor includes them.
- **Spawn cost** about 68 ms per `aval` start: never on every `session.measure`.
- **Hub rule**: nothing in this plan adds a hub surface.
- **Fixtures are private data** in a public repo until sanitised (D7).

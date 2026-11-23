# Relay cost model: what measured data changes in the relay plan

Status: approved by the owner on 2026-11-23. It amends the relay specs in issues #63 to #71. It is a planning document; no code is involved.

Source: a teammate's write-up, "El relevo de contexto, visto con nuestros datos" (2026-11-23). It describes a cost-driven relay already running for Claude Code (clear advisor, deterministic handoff, autocompaction tuning). It is backed by simulations over one real week of transcripts: 2026-11-08 to 2026-11-15, 273 worker sessions plus about 290 interactive ones, about $1,843 API-equivalent, 22k requests.

## What the data says

| Topic | Measured result | Status in the source |
| :--- | :--- | :--- |
| Deterministic note | Built from the transcript with no model call: 38 ms on the first pass over 6.6 MB, about 20 ms after that, read incrementally from the last byte read | measured |
| Cold cache | The prompt cache lives 1 h. A read costs 0.1x input; a write costs 2x. After a long pause, continuing rewrites the whole context, so clearing always wins with a cold cache: 150k context on an 85k floor saves 130k input tokens in one request | measured |
| Warm cache | Clearing pays only if more than `N > 19 · T_base / (T_ctx − T_base)` requests remain, where `T_base` is the startup floor and `T_ctx` the current context. On the old 85k floor that is N > 108 at 100k and N > 10 at 250k | measured |
| Optimal compaction point | About 200 to 230k tokens (Monte Carlo, 1,000 bootstrap resamples). An autoCompactWindow of 250k actually compacts at about 217k, roughly 33k early | measured |
| Startup floor | Every session starts at about 58k (73k before CLAUDE.md was trimmed). The floor is about 25% of total spend; long sessions over 150k are about 18% | measured |
| Relay to a new agent | Costs the same as compacting: $1,512 versus $1,502 at 217k. It is cheaper only if the new agent re-reads nothing, which real tasks never manage | measured |
| Forced clears with a warm cache | Between −4.6% and +1.1% at a realistic continuity weight (λ = 10). Native autocompaction is already a relay: it adds only about 20k over the session floor | measured |
| In-session replace (aval T0) | The system and tool prefix stays cached, so it should be cheaper than `/clear` | **inference**, not measured |
| Quality versus context length | Not measured. A pilot was proposed and never run | unknown |

### Limits of the data

- One person, one week, Claude Code only.
- Opus 5.5 prices and an 8k summary are assumed.
- The MDP is only approximately Markov: lag-1 autocorrelation is 0.19 to 0.28.
- The simulation scripts (`mc.py`, `relay.py`, `mdp.py`) lived in `/tmp` and are lost.

## What changes in the plan

1. **Trigger on absolute tokens, not a percentage of the window.**
   - With a 1M window, 60% is 600k, almost three times the measured optimum.
   - `Policy` gains `SoftTokens` and `HardTokens`, with initial defaults of 160k soft and 200k hard. These are starting values, to be calibrated (item 7).
   - `SoftPct` and `HardPct` remain as a cap for small windows. The effective threshold is `min(HardTokens, HardPct × Window)`.
2. **Cache-aware decision.**
   - `Snapshot` gains `CacheAge` (time since the last request) and `CacheTTL`. `CacheTTL` is read from the runtime, never assumed: it is 1 h or 5 min depending on the account and overage state.
   - Cold cache (`CacheAge > CacheTTL`) and context of at least `ColdMinTokens` (100k): relay at the next turn boundary, before the request is sent. This is the largest measured saving.
   - Warm cache: relay at `HardTokens` only. Never force an early relay with a warm cache.
3. **Startup floor in the decision.**
   - `Reading` gains `FloorTokens`, the context left right after a relay. It is measured per runtime and model on the first relay, and estimated from the system prompt plus tools until then.
   - Net benefit is `T_ctx − FloorTokens`, not `T_ctx`.
   - Notifications and docs stop promising "from 61% to 6%". They show tokens before and after, and the floor.
4. **The cost value is the cold-cache case.** The pitch for the relay is quality, coordination of running work and reach across 17 runtimes. Cost savings are claimed only for the cold-cache trigger, and only once measured.
5. **Precomputed note only with a warm cache (#71).** A model-written note requested on a cold cache costs as much as continuing. With a cold cache the capsule is deterministic only.
   - The deterministic capsule adopts the measured baseline content: up to 15 previous prompts, the 25 most recently edited files, and the last full exchange.
   - The transcript is read incrementally from the last byte read.
6. **Measure in-session replace before claiming it (#63, spike E2).** On the same transcripts, compare a T0 relay, `/clear` plus injection, and native compaction. Record `cache_read_input_tokens` and `cache_creation_input_tokens` of the first request after each. This decides whether T0 is cheaper, and by how much.
7. **Replay simulator and quality pilot (#74).**
   - Rebuild the lost simulations as a versioned replay tool in aval. It replays real transcripts against a policy and reports cost, so any user can calibrate the thresholds on their own data.
   - Run the quality pilot: long tasks at several thresholds, scoring task success, not only cost.
8. **Claude autocompaction offset (#65).** The native window compacts about 33k before its nominal size. The T2 mid-turn fallback sets the override with that offset in mind, so it does not fire before aval's own hard threshold.

## Issue changes

| Issue | Change |
| :--- | :--- |
| All of #63 to #71 | The shared "Context relay in brief" line: absolute token threshold plus a cold-cache trigger, instead of 60/50% |
| #63 spike | Add E2 (in-session replace versus `/clear` versus compaction cost) and E3 (startup floor per runtime) |
| #64 core | `Policy` and `Snapshot` fields, cache-aware `Decide`, floor in the net benefit; new RED tests |
| #65 Claude T0 | Cold-cache trigger on `UserPromptSubmit`; autocompaction offset |
| #66 notifications | Tokens before and after plus the floor, instead of percentages |
| #71 tuning | Note only with a warm cache; baseline capsule content; incremental transcript reader |
| #74 (new) | Replay simulator and quality pilot |

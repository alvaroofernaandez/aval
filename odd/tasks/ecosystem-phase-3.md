# Ecosystem phase 3: one config model, a Pi adapter, generations, usage

Why: aval is the single harness of the sumi ecosystem (decision 2026-11-26).
Pen (a daemon agent driving Pi over RPC) and the factories (headless runs in
containers) must boot from the same configuration aval gives Claude Code, pinned
to a known state, and their cost must be measured. Phase 3 of the approved
ecosystem design adds to aval:

- (a) a single configuration model and an adapter interface, with Claude Code
  and Pi (earendil-works/pi-coding-agent, MIT) as adapters;
- (b) immutable generations: build, activate, rollback and pin, reusing
  `txsync` and `backup` (#120);
- (c) the HCS-0008 invariants as tests: projected or reported, one generation
  per run, host settings win, no ambient config. HCS-style dotted ids and a
  contentHash only, never HCS conformance (the spec is a single-author draft);
- (d) usage measurement reconciled with ccusage (#29): cost is not quota.

Billing rule: every agent runs on the owner's Claude subscription. Nothing
aval renders sets, reads or forwards `ANTHROPIC_API_KEY`; the Pi adapter
asserts it.

Delivery: stacked PRs to main, one per issue, each with `Closes #N`. PRs are
opened, never merged; review is the owner's.

## T1 — #119 flaky treeHash test helper
- [x] RED: a vanished entry (Info fails) is skipped, not a panic
- [x] RED: churn under `.git` does not change the hash
- [x] RED: gitEnv disables gc.auto and maintenance.auto
- [x] GREEN, `go test -count=50 ./internal/devmode`; siblings in cmd/aval/hub_test.go, tests/devmode_test.go, tests/install_test.go
- [x] PR

## T2 — #8 single configuration model and adapter interface
- [x] `internal/core/agentcfg`: Config (rules, skills, commands, agents, hooks, permissions, MCP), loaded from the repo as it is
- [x] Adapter interface: Name, Capabilities, Render, Apply, Diff; loss warnings for unsupported capabilities
- [x] Validation: broken references (missing skill, hook without command, duplicates, no description)
- [x] Reusable contract suite and a fake adapter; render budget benchmark (2.3 ms Load+Render)
- [x] Under `core/*` (sync imports it), so no ADR 0002 change; ARCHITECTURE.md rows
- [x] PR

## T3 — #9 Claude Code adapter on the single model
- [x] Golden of the current `aval sync` output captured before the refactor (darwin, linux, windows)
- [x] `internal/core/agentcfg/claude` renders the managed settings; parity test against the repo and the goldens
- [x] `aval sync` goes through the adapter; personal keys and overlay kept; merge and ForOS moved, sync's duplicate rules reader and JSON decoder removed
- [x] Rules, skills, commands, agents as `Rendered.Linked` (owner: option A); contract suite green on the whole repo, PR

## T4 — Pi adapter (#68 Pi slice, #10 support matrix row)
- [x] `internal/core/agentcfg/pi`: settings, AGENTS.md rules (shared block only, owner: option A), skills, prompts, mcp.json; isolated agent dir (`Env` sets PI_CODING_AGENT_DIR)
- [x] Capabilities: MCP native via mcp.json (Pi v1.7; owner: P2 = A), no subagents, no permission model, hooks via extension only; loss warnings, partial prompts warned
- [x] Billing guard: nothing rendered carries an API key (ErrAPIKey); `Env` drops ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN and ANTHROPIC_OAUTH_TOKEN
- [x] Subscription login inside the isolated dir: Apply links auth.json to ~/.pi/agent/auth.json (owner: P1 = A)
- [x] Contract suite green on a sample and on the whole repo, PR

## T5 — #120 immutable generations
- [x] Build: content-addressed id gen-<16 hex> (owner G2 = A), writes only under ~/.aval/generations, read-only, atomic rename; `aval generations build|list|diff --json` (PR 1 of 3, owner G3 = A)
- [x] Activate (owner G1, G4, G5, G6 = A): atomic swap of ~/.aval/generations/current (symlink, pointer file on Windows), Claude Code through the txsync merge, doctor after activation, restore on failure, crash recovery; sync = build + activate; the active generation is the doctor's desired state; files 0444 + contentHash check (G8 = A) (PR 2 of 3)
- [x] Rollback (G7 = A): append-only history, bare rollback toggles, `rollback <id>`
- [x] Pi consumers get pi/ applied into ~/.aval/consumers/<name>/pi with the login link
- [x] Pin and retention like backup
- [x] Consumer pin file so Pen and factories boot from a pinned generation, PR (3 of 3)

## T6 — HCS-0008 invariants as tests
- [ ] Projected or reported: every Config element is rendered or warned about, per adapter
- [ ] One generation per run: a run reads exactly one generation id
- [ ] Host settings win: the overlay never removes a host deny
- [ ] No ambient config: adapters read only their inputs, never the real HOME, PR

## T7 — #29 `aval usage`
- [x] Transcript reader, dedupe across files, per project, session, day and consumer
- [x] Price table with an effective date; unknown model shows n/a; billing api or subscription
- [x] 5 h block and 7 d window, `--check` budgets, `--json`
- [x] Reconciliation fixture matching ccusage totals (ccusage 20.1.2), PR

## T8 — #55 `aval dev check`
- [x] One step table that ci.yml calls; pinned tools; per-OS rules in the table
- [x] `--list`, `--fast`, `--only`, `--keep-going`; a test that ci.yml has no quality steps outside the table, PR

## After phase 3
Continue the open issues in milestone order (v0.3.0, v0.4.0, v0.5.0, ...),
skipping those that need an owner decision or hardware not available here
(Windows, desktop spikes); list them with the exact question.

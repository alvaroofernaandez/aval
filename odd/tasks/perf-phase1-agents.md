# Performance phase 1 — agents (hooks, MCP, SDD removal)

Decision 2026-11-21: the whole environment is tuned for speed, lightness and
portability, measured before and after. SDD is dropped entirely.

Baseline (M4, 16 GB): sdd-preflight hook 657 ms per subagent; code-review-graph
update 154 ms per edit; chrome-devtools MCP via `npx @latest` 535 ms start and
~220 MB; turing MCP ~177 MB loaded in every project; wakatime plugin 112 ms
twice per tool call; warp plugin 6–8 ms per hook with Warp not installed.

- [x] T1 Hooks: drop sdd-preflight and skill-registry refresh; code-review-graph async (tests/agent-hooks.test.sh)
- [x] T2 MCP: chrome-devtools pinned global binary (258 ms vs 535 ms); turing moved to project scope (~/.claude.json, not versioned)
- [x] T3 SDD outside the repo: copies in ~/.agents, ~/.codex, ~/.gemini moved to ~/.Trash/sdd-retired-*; SDD orchestrator block stripped from GEMINI.md and Codex AGENTS.md (82–99 KB → 33 KB)
- [x] T4 SDD inside the repo: skills retired, agents/prompts removed, instructions cleaned (tests/no-sdd.test.sh)
- [x] T5 Plugins: warp disabled (no Warp installed); wakatime kept — the company tracks time with it (112 ms x2 per tool call, hooks owned by the plugin)
- [ ] T6 Projects: .atl/ (29 repos), openspec/ (7), project CLAUDE.md/AGENTS.md SDD mentions, stale worktrees (~4 GB) — needs owner decisions per repo

Note: gentle-ai 4.6.0 reports the `sdd` component as retired, so `sync` does
not reinstall it.

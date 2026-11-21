# Claude Code efficiency — settings, design pipeline, skills

Inputs: history audit of 174 sessions (2026-09-22..2026-11-21) and ecosystem research.

## T1 — settings (live + aval)
- [x] Commit attribution off at the settings level, not only by rule
- [x] Hooks: every command guarded, no dead or SDD hooks, live == aval
- [x] Plugins: disable those not installed / not used (warp, seo guard noise)
- [x] CLAUDE.md: rules for recurring model failures (absolute paths,
      AskUserQuestion >= 2 options, fix the whole class of a bug, visual
      "fixed" needs a screenshot, mem_judge_many)

## T2 — design pipeline audit → simplify
- [ ] Gate survives compaction and sub-agents (229 denials / 53 sessions)
- [ ] Fewer skills, fewer tokens, same guarantees

## T3 — skills audit (global, per project, per agent)
- [ ] Inventory, duplicates, dead, unused; always-loaded token cost
- [ ] Apply: remove/disable what adds cost without value

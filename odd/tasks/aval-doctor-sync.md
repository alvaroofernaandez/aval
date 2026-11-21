# aval doctor, sync and the private overlay

Why: on 2026-11-21 every one of these was found and fixed by hand — 14 broken
links, hooks calling retired commands, agents pinned to retired models, skills
without SKILL.md, live settings drifting from the repo, SDD leftovers in Codex
and Gemini instructions, employer material that can't live in the public repo.

## T1 — `aval doctor [--fix] [--json]` (Go, < 1 s, offline)
- [x] Checks: manifest links, broken links in agent dirs, live hooks resolvable
      and not retired, live hooks == repo hooks, agent model aliases, skills
      without SKILL.md / frontmatter, enabled plugins that are not installed
- [x] `--fix`: remove broken links, run sync; exit 1 on any failure

## T2 — `aval sync [--dry-run]`
- [x] Merge repo-managed keys into ~/.claude/settings.json, keep personal keys,
      back up first
- [x] Shared agent-rules block into ~/.codex/AGENTS.md and ~/.gemini/GEMINI.md

## T3 — private overlay `~/.config/aval/private`
- [x] settings.json fragment merged by sync; links manifest applied by sync
- [ ] Employer material moves there; no-proprietary goes green

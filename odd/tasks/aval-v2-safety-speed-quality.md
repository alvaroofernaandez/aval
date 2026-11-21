# aval v2 — safety, fewer tokens, guarantees

Why: research of gentle-ai, FullFran/hcs + precedent, ECC, rulesync, codeburn
and others (2026-11-21). Three parallel worktrees, merged by the coordinator.

## A — safety (branch feat/safety)
- [x] `aval restore [--list|latest|<id>]`, backup manifest + retention (5, pin)
- [x] Transactional `sync`: backup → apply → doctor → auto-restore, one attempt
- [x] `aval uninstall [--dry-run]`: only repo links, restore previous files
- [x] Deny-list coverage test; private overlay can narrow, never widen denies

## B — fewer tokens, faster (branch feat/lean)
- [ ] Lessons hook (`aval hook lessons`): path-triggered lessons instead of
      always-on rules; 0 tokens on no match, < 10 ms
- [ ] `aval budget`: always-on tokens per agent + hook latency vs budget
- [ ] `aval review assess`: git-only risk tier → how many reviewers

## C — guarantees (branch feat/quality)
- [x] `aval sync --check` wired into the pre-push hook
- [x] `aval doctor --security`: broad permissions, remote-exec hooks, inline MCP tokens
- [x] Fake-agent E2E for the hooks (no API keys)
- [x] `aval update --dry-run` + sha256 verification of downloaded binaries

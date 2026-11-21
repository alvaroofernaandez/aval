# aval v2 — safety, fewer tokens, guarantees

Why: research of gentle-ai, FullFran/hcs + precedent, ECC, rulesync, codeburn
and others (2026-11-21). Three parallel worktrees, merged by the coordinator.

## A — safety (branch feat/safety)
- [ ] `aval restore [--list|latest|<id>]`, backup manifest + retention (5, pin)
- [ ] Transactional `sync`: backup → apply → doctor → auto-restore, one attempt
- [ ] `aval uninstall [--dry-run]`: only repo links, restore previous files
- [ ] Deny-list coverage test; private overlay can narrow, never widen denies

## B — fewer tokens, faster (branch feat/lean)
- [x] Lessons hook (`aval hook lessons`): path-triggered lessons instead of
      always-on rules; 0 tokens on no match, < 10 ms
- [x] `aval budget`: always-on tokens per agent + hook latency vs budget
- [x] `aval review assess`: git-only risk tier → how many reviewers

## C — guarantees (branch feat/quality)
- [ ] `aval sync --check` wired into the pre-push hook
- [ ] `aval doctor --security`: broad permissions, remote-exec hooks, inline MCP tokens
- [ ] Fake-agent E2E for the hooks (no API keys)
- [ ] `aval update --dry-run` + sha256 verification of downloaded binaries

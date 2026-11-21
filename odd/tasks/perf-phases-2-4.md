# Performance phases 2–4 — shell, portability, system

Continues odd/tasks/perf-phase1-agents.md. Every change measured before/after.

## Phase 2 — shell
- [x] zsh interactive startup 113.7 ms ± 0.8 → 38.6 ms ± 0.4 (hyperfine, M4).
      oh-my-zsh dropped (one plugin, duplicate compinit); tool init scripts
      cached and regenerated when the binary is newer; no uname/go env forks;
      nvm hook removed (nvm not installed); fzf Alt-C variable fixed.
      Guard: tests/shell-startup.test.sh (80 ms budget + behaviour probes).

## Phase 3 — portability
- [x] tmux status bar and paste router on Linux, fixture-tested on macOS (/proc loadavg+meminfo; wl-paste/xclip/xsel). GPU omitted on Linux: no cheap vendor-neutral unprivileged source, and a fixed value would be false. macOS refresh 10.1 → 10.2 ms.
- [x] Brewfile (leaves, casks, taps) — new machine is one `brew bundle` away
- [x] Paths: no hard-coded home directory in zshrc; dead PATH entries pruned
- [ ] Not done on purpose: dedupe "Contextual Skill Loading" in OpenCode
      AGENTS.md (~190 tokens) — the copy lives in a gentle-ai managed block
      that `gentle-ai sync` rewrites.

## Phase 4 — system
- [x] brew cleanup -s --prune=all: 41 MB
- [x] ~/.oh-my-zsh (22 MB) → Trash, unused after phase 2
- [x] 80 clean, integrated worktrees removed: 31 GB (phase 1, T6)
- [ ] Left for the owner: postgresql@17 brew service (24 MB, a project may
      depend on localhost:5432), Kiri/Docker prune (rule: show and confirm),
      ~/.gentle-ai/backups (58 MB), Steam launch agent.

## Safety
- Pre-push hook (.githooks/pre-push) refuses employer material and secrets in
  outgoing commits; added after two leaks reached commits on 2026-11-21.

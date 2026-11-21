# aval go-native

Goal: aval is a single Go binary. No shell or Python of our own; vendored
upstream code removed; clean, declarative repo layout; OSS hygiene files
modelled on gentle-ai.

Out of scope by nature: `home/zshrc`, `home/zprofile`, `home/p10k.zsh`,
`home/zsh/*` — configuration sourced by zsh, not programs.

Decisions
- 2026-11-21: vendored upstream (gstack, p87, ui-ux-pro-max) is deleted, not
  ported. `design-gates` (p87) is re-implemented as aval's own Go command
  because the design pipeline depends on it.
- Bootstrap install: `go install`, Homebrew tap and the existing npm wrapper.
  No `curl | sh` installer.

## Tasks

- [x] T1 Remove vendored upstream: `third_party/`, `pins/`, skills `browse`,
      `design-shotgun`, `land-and-deploy`, `gstack-policy`, `ui-ux-pro-max`,
      `design-gates`; `scripts/{gstack-sync,gstack-patch,p87-sync,uiux-sync}.sh`;
      lesson `gstack-vendored.md`; their tests; every reference.
- [x] T2 OSS hygiene: `LICENSE` (MIT), `CONTRIBUTING.md`, `SECURITY.md`,
      `CODE_OF_CONDUCT.md`, `.github/ISSUE_TEMPLATE/*`,
      `.github/PULL_REQUEST_TEMPLATE.md`, `.gitattributes`.
- [x] T3 `aval design gate` (contrast, hardcodes, tokens) in Go, replacing the
      deleted design-gates scripts; `design-ack` as `aval design ack`.
- [x] T4 Claude hooks (`config/claude/hooks/*.sh`) → `aval hook <name>`;
      settings.json points at the binary.
- [x] T5 tmux helpers (`config/tmux/*.sh`) → `aval tmux <name>`; tmux.conf
      calls the binary.
- [x] T6 Remaining scripts → Go: `install.sh`, `scripts/install.sh`,
      `scripts/sync-agent-rules.sh`, `scripts/reap-orphan-tests.sh`,
      `scripts/macos/*`, skill scripts (`kiri-up.sh`, `pdf-report/*.py`,
      docker/design-refactor helpers), `docs/*.py`.
- [x] T7 Shell test suite (`tests/*.test.sh`, `config/tmux/tests`) → `go test`;
      CI runs `go test ./...` only.
- [ ] T8 Layout: final folder tree, root cleanup, README/docs updated.

Each task: strict TDD, `go test ./...` green, one conventional commit. Never push.

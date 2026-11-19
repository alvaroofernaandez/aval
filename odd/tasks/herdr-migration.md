# Herdr migration (replace Orca)

Decision 2026-11-19: Herdr (herdr-app + herdr-core `hc`) replaces Orca as the
terminal/agent workspace. Claude Code reports its status to Herdr natively
(OSC 7501), so no agent hooks are needed.

- [x] T1 Stop Orca (app, daemon, orphan shells and MCP servers) by exact PID
- [x] T2 Remove Orca worktrees with `git worktree remove` (branches kept in ~/Proyectos repos)
- [x] T3 Uninstall Orca: app, ~/orca, ~/.orca, Application Support, caches, prefs, agent leftovers
- [x] T4 Dotfiles: drop the 16 Orca hooks, config/orca, tests/orca-config.test.sh
- [x] T5 Skills: `herdr` skill (TDD), replace Orca `orchestration` skill, hc templates
- [x] T6 Version herdr-core config (templates) via install.manifest
- [x] T7 Memories + run-all.sh green

Done 2026-11-19: Orca uninstalled (~2 GB freed). Full suite green except yazi-sidebar-config, which times out only inside an hc pty (passes from a normal shell).

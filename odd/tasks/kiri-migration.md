# Kiri migration (replace OrbStack)

Decision 2026-11-17: Kiri (github.com/libracoder/kiri, v0.2.x beta) replaces OrbStack.
OrbStack is uninstalled ONLY after everything is migrated and verified.

- [ ] T1 Backup every named/anonymous data volume (tar.gz) to ~/Backups/orbstack-volumes-2026-11-17, skip buildkit cache
- [ ] T2 Install Kiri.app (signed DMG) and run `kiri setup` (docker context `kiri`)
- [ ] T3 `kiri import` from OrbStack; verify volumes (checksums / Postgres row counts)
- [x] T4 Make `kiri` the default docker context; fix shell init (zprofile)
- [x] T5 Rewrite skills: orbstack-docker -> kiri-docker, orbstack-health -> kiri-health, update docker-space-optimizer, docker-build-optimized, mac-disk-cleanup
- [x] T6 Update CLAUDE.md, opencode AGENTS.md, tests/docker-runtime-skills.test.sh (TDD), memories
- [ ] T7 Fix project coupling (orb.local, orbstack context, socket paths)
- [ ] T8 Smoke-test projects' compose stacks on Kiri
- [ ] T9 Uninstall OrbStack (after user-visible verification)

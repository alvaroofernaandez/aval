# Versioning and Windows

Roadmap issue: #34 "[M8.3] feat(windows)". T1 (versioning) is tracked by its
own branch; this file carries both so the two stay visible together.

Rule for T2: one cross-platform code path (filepath, os APIs) wherever
possible; an OS branch only inside one small helper with a test; Unix-only
features skip cleanly (runtime check plus a stated reason), never emulated.

## T2 - Windows support (branch feat/windows)
- [x] Installer copy mode is a first-class path: a destination still equal to
      the repo is "already installed" (no new backup), doctor accepts an
      untouched copy, uninstall removes only untouched copies
- [x] Atomic writes: `renameReplace` retries transient sharing violations on
      Windows (settings.json read by Claude Code while sync writes it)
- [x] `aval sync` writes exec-form hooks (`command` + `args`, no shell) and a
      forward-slash status-line command on Windows; doctor and budget
      understand the exec form
- [x] Unix-only commands (tmux, macos, reap-orphan-tests, kiri, docker) print
      "not supported on windows" and exit 1
- [x] Design gate understands native Windows paths; plan labels always use `/`
- [x] Tests: Unix-only suites skip via `testenv.SkipWindows` with a reason;
      HOME/USERPROFILE/LocalAppData are set together (`testenv`)
- [x] CI: `windows-latest` in the matrix (copy-mode install, no -race)
- [ ] First green Windows CI run (cannot be verified locally)

# aval dev: a sandbox for developing aval without touching the machine

Why: testing aval means running install, sync and doctor, which write to the
real $HOME, ~/.claude and ~/.config. Dev mode gives every command a throwaway
HOME so the real system, config and projects stay untouched.

## T1 — internal/devmode (sandbox layout, env, safety)
- [ ] RED tests: name validation, base dir, root safety, env isolation, shell spec, nested guard
- [ ] GREEN: layout.go, env.go, shell.go

## T2 — repo copy, build, bootstrap, projects, ls/rm
- [ ] RED tests: CopyRepo, git identity, Project clone, List/Remove
- [ ] GREEN: prepare.go, project.go, manage.go

## T3 — `aval dev` command (one dispatch case + cmd/aval/dev.go)
- [ ] RED e2e (tests/devmode_test.go): fake real HOME stays byte-identical
- [ ] GREEN: shell, run, reset, ls, rm, diff, project

## T4 — docs/devmode.md, README and guide links, smoke run on the real Mac

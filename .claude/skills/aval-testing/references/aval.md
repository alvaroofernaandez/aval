# aval test runners, environment and pitfalls

Detail for `../SKILL.md`. Paths are relative to the repository root.

## Runners

| Suite | Command | Notes |
| --- | --- | --- |
| Everything | `go test ./...` | What a PR must keep green |
| CI | `go test ./... -race` on ubuntu, macOS and windows | `.github/workflows/ci.yml`, guarded by `tests/workflows_test.go` |
| One package or test | `go test ./internal/core/sync/ -run '^TestX$'` | While iterating |
| Repo-level guards | `go test ./tests/` | Builds the binary lazily, once per run, only if a test needs it |
| Architecture gate | `go test ./tests/ -run Architecture` | Edges, ratchet, model freshness (`tests/architecture_test.go`) |
| tmux e2e | `go test ./internal/term/tmux/e2e/` | `//go:build !windows`; needs tmux, skips without it |
| Static | `go vet ./...`, `gofmt -l .` | gofmt must print nothing |

## Environment and flags

| Name | Effect |
| --- | --- |
| `AVAL_TEST_REPO` | Points `tests/` at another checkout or a mutated copy of this one. This is how a guard is shown to fail when what it protects breaks (`tests/helpers_test.go`, `findRepo`). |
| `-update` | Rewrites generated files instead of comparing. In `tests/` it regenerates `docs/architecture/model.md`; in `internal/ui` it rewrites `testdata/*.golden`. Run it on one package with `-run`, then review the diff. |
| `AVAL_NO_UPDATE_CHECK=1` | Disables the update check in a spawned binary. |
| `-race`, `-count=N`, `-p 1` | Race detector, defeat the cache and repeat, serial packages (flake classification). |

## Helpers

- `internal/testenv` (test imports only, enforced by the architecture gate): `SkipWindows(t, reason)`, `NeedSymlinks(t)`, `HomeEnv(home)` for a spawned process's env, `SetHome(t, home)` for the current process.
- `tests/helpers_test.go`: `p(rel...)` resolves repo paths, `read`, `exists`, `isDir`, `subdirs`, `mustHave` (line-anchored regexp), `mustNotHave`, `mustContain`, `eq`, `isTrue`, `needTool` (skips when an external tool is absent), `avalBin` (the binary built once).

## Pitfalls

- **One `-update` flag per package.** `tests/` declares it once, in `tests/architecture_test.go` (`archUpdate`). A second `flag.Bool("update", ...)` in the same package panics with "flag redefined". Reuse `archUpdate` or the existing flag; do not declare another.
- `go test ./tests/ -update` without `-run` rewrites every generated file the package owns. Scope it.
- Tests that need an external tool (`gitleaks`, `tmux`, `vhs`) skip when it is missing: report that as skipped, not passed.
- A test that sets HOME without `t.TempDir()` can write into the real HOME of whoever runs it.
- The shell, hook and statusline latency tests are sensitive to machine load; see the SKILL.
- Windows: no symlinks without privilege (`testenv.NeedSymlinks`), `.exe` suffix on built binaries, CRLF in files read back.

---
name: aval-testing
description: "Use when writing or changing a test, doing TDD (RED/GREEN), running any suite, or chasing a flaky or slow test in aval. Test levels, error assertions, determinism, the flake protocol, the differential check and the output contract, for Go."
---

## Activation contract

Load before writing or changing a test, or before running any suite, in this repository. It carries the general discipline and the aval specifics; where it disagrees with a global testing skill (including the distributed `strict-tdd`), this one wins. Runners, env vars and pitfalls: [references/aval.md](references/aval.md). The why behind each rule: [references/field-notes.md](references/field-notes.md).

## Hard rules

- **Test first.** One RED test per rule; it fails on its assertion, not on a compile error, a missing file or setup. Then GREEN with the minimum code, then refactor while green.
- **Never invent evidence.** Report the command you ran and what you saw.
- **Lowest level that catches the regression.** Assert through the public interface: output, exit code, returned values, files written. Not private helpers.
- **Regression guard.** For every existing command or option sharing code you touched (flags, parsers, helpers), one test proves its previous behaviour holds.
- Committed tests never read git history or a fixed commit.

## Levels

| What you verify | Level | Where |
| --- | --- | --- |
| A parser, planner, mapping, renderer | Unit, pure, scratch via `t.TempDir()` | `_test.go` next to the code in `internal/` |
| A subcommand's behaviour | In process through the `run` seam | `cmd/aval` (target state, see below) |
| Process startup: argv, exit code, PATH, Windows `.exe` | One real-binary smoke test per subcommand | `tests/` (builds the binary once) |
| Repository rules: budgets, docs, workflows, architecture, ADRs | Repo-level guard | `tests/` |
| Real tmux, terminals, long flows | e2e behind a build tag | e.g. `internal/term/tmux/e2e` (`//go:build !windows`) |

**The `run` seam is pending (S6 of `odd/tasks/engineering-standards.md`).** Target: `run(args, stdin, stdout, stderr, env) int` in `cmd/aval`, `main` a thin wrapper, every subcommand tested in process and one real-binary smoke test each. Until S6 lands many tests spawn the binary; do not add new binary-spawning tests where an in-process one is possible today (`cmd/aval/hub_test.go` drives the hub in process).

## Assertions

- Errors: `errors.Is` for sentinels, `errors.As` for typed errors. Never compare `err.Error()` or `strings.Contains(err.Error(), ...)`: that is a change detector. If the code returns no sentinel, add one (RED first).
- User-facing text is pinned in exactly one place (S7 makes that a single messages test). Logic tests assert data, not sentences.
- One reason to fail per test; a parameterised table in one file instead of the same invariant across many files.
- Prefer in-memory fakes over mocks that assert call sequences. A fake behaves like the real thing: what the real one stores, the fake stores.

## Determinism

- Scratch only through `t.TempDir()` (removed automatically). Never `os.MkdirTemp` or `ioutil.TempDir` in a test.
- No `time.Sleep`. Wait on a condition with a deadline, or inject a clock. No wall-clock assertions in unit tests.
- `t.Setenv` for env; `testenv.SetHome` for HOME. Never touch the real HOME.
- The S5 hygiene guard, `tests/test_hygiene_test.go`, fails on `MkdirTemp`, `TempDir` and `Sleep` in `_test.go` files outside a reasoned exemption map.
- Goldens: regenerate with `-update`, never by hand; mask times, durations, paths and versions before comparing. Example: `internal/ui/golden_test.go` renders with a fixed theme and colour profile so output is identical on every OS. Review every golden diff.

## Flake protocol

1. Reproduce: `go test -race -count=20 -run '^TestName$' ./pkg/`.
2. Classify: run it alone and serially, `go test -race -count=20 -p 1 -run '^TestName$' ./pkg/`, then with the whole package. A test that passes alone and fails in parallel is a finding (shared HOME, env, ports, files, global state), not noise.
3. Find the mechanism. A flake is often a real race in the code: make it deterministic (hold an event back, a fake that answers late) to get a RED that fails every run, then fix the code.
4. Never loosen an assertion, add a sleep or retry blindly until it passes.

**Load-sensitive tests.** `TestLessonsHookLatency` (`tests/lessons_test.go`), `TestRateGuardHookLatency` (`tests/rateguard_test.go`) and `TestShellStartupBudget` (`tests/shell_startup_test.go`) measure wall time. Before calling a failure a regression, run the same test on `main` under the same load (a worktree, below) and compare.

## Differential check

Before finishing a change to a command, compare it with the base:

```bash
git worktree add /tmp/aval-base main
(cd /tmp/aval-base && go build -o /tmp/aval-base.bin ./cmd/aval)
go build -o /tmp/aval-new.bin ./cmd/aval
# same args, same throwaway HOME contents for both; compare stdout, stderr and exit code
git worktree remove /tmp/aval-base
```

Every difference you did not intend is a regression: fix it and pin it with a test.

## Mutation testing

Optional and on demand, over the code you touched, when a test looks weak (lines covered, nothing asserted). Never a gate and never a dependency (ADR 0001). A surviving mutant becomes a RED test.

## Commands

```bash
go test ./pkg/ -run '^TestName$'      # while iterating
go test ./...                          # once, at the end
go test -race -count=1 ./...           # what CI runs (plus vet and gofmt)
go vet ./... && gofmt -l .
```

## Output contract

- **What ran:** exact command, exit code, pass, fail and skip counts.
- **What did not run, and why:** a build tag not set, a tool missing (`needTool` skips), Windows-only paths. Skipped is not passed.
- **Failures:** the test name and the output tail, not a paraphrase. A known load-sensitive failure is named with the evidence it also fails on `main` under the same load.
- **TDD evidence:** the RED line (the failing assertion) and the GREEN output per rule; the differential check result.

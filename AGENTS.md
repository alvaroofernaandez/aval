# Agent instructions for aval

Read by any coding agent working in this repository (Claude Code loads it through `CLAUDE.md`). The project skills in `.claude/skills/` are the standard here: where one disagrees with a global or personal skill, the project skill wins.

## Load the project skill before the task

| Task | Skill |
| --- | --- |
| Package boundaries, a new package, interface or seam, an import between `internal/` packages, before and after a refactor | `aval-architecture` |
| Writing or changing a test, TDD, running any suite, flaky or slow tests | `aval-testing` |
| Any Markdown file, README, guide, ADR or documentation image | `aval-docs` |
| Versioning, the changelog, tagging or publishing a release | `aval-release` |
| Executing a plan from `odd/tasks`, or slides of a plan | `plan-deck` |
| Any visible surface: a TUI screen, CLI output, layout, style, copy, or `.agents/DESIGN.md` | `design-pipeline` |
| Any feature, bug fix or behavioural refactor | `strict-tdd` |

`plan-deck`, `design-pipeline` and `strict-tdd` are the skills aval distributes from `skills/`; they apply here as well.

## Hard rules

- **Go only.** New behaviour is Go in `cmd/` or `internal/`. No shell or Python; dotfiles sourced by zsh (`home/`) are the only exception.
- **Strict TDD.** RED first: a test that fails on its assertion, not on a compile or import error. Then GREEN with the minimum code, then refactor.
- **Assert errors, not text.** Use `errors.Is` or `errors.As`, never `err.Error()` comparisons. User-facing text is pinned in one place.
- **Tests are deterministic.** Temp dirs only through `t.TempDir()`; no fixed sleeps.
- **Commits.** Conventional Commits, one reviewable unit each, with its tests. No `Co-Authored-By` or AI attribution.
- **Plan before execution.** Substantial work gets a plan in `odd/tasks/<feature>.md` and a deck (`plan-deck`) before the first source edit.
- **Visible surfaces.** Run the design pipeline and `aval design ack <surface>` before editing one; prove it with `aval design gate`.
- **Never push unasked.**
- **Hooks and the statusline never load config.** They are hot paths: pure work on their input, no config read, no exec, no network.
- **Opening the hub writes nothing and never touches the network.** It reads local state only (the update badge uses refs already fetched).
- **Decisions are recorded, never rewritten.** A changed decision gets a dated amendment or a new ADR.

## Commands

```bash
go test ./...                          # the full suite
go test ./tests/ -run Architecture     # the import-boundary gate
go vet ./...
gofmt -l .                             # must print nothing
go test -race -count=1 ./path/...      # chasing a flake: race detector, no cache
```

Run the focused package or `-run` pattern while iterating and the full suite once at the end.

## Output contract

Every report of work, by a person or an agent, states:

- **What ran:** the exact command, its exit code, and the pass, fail and skip counts.
- **What did not run, and why:** a suite skipped, a build tag not set, a tool missing. Skipped is not passed.
- **Failures:** the test name and the tail of its output, not a paraphrase.
- **TDD evidence:** the RED line (the failing assertion) and the GREEN output.

Report only what ran and what you observed. Never invent evidence.

## Where things live

- `cmd/aval`: the entry point, a thin wrapper.
- `internal/`: leaves (`agent/*`, `devops/*`, `term/*`, `deck`, `design/*`, `release`, `docs/*`, `devmode`) import nothing internal; `core/*` imports only `core/*` and `release`; `ui` sits on `core/*`; `ui/cli` on `ui`. The model and the allowlist are in `docs/architecture/model.md` and ADR 0002; `go test ./tests/ -run Architecture` enforces them.
- `tests/`: repo-level guards (budgets, workflows, docs, architecture).
- `odd/tasks/`: plans, one file per feature.
- `docs/adr/`: architecture decision records.
- `.agents/DESIGN.md`: the design contract for every visible surface.
- `install.manifest`: what `aval install` links; code reads it, never hard-codes paths.
- `config/claude/CLAUDE.md`: the global file aval installs for its users, not this project's instructions.

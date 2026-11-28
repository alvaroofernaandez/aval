# Contributing to aval

Thanks for your interest in **aval** — a single Go binary that installs,
syncs and verifies a terminal and AI-agent environment.

## Issue first

No pull request without an issue.

1. Open an issue with the [bug report](.github/ISSUE_TEMPLATE/bug_report.yml)
   or [feature request](.github/ISSUE_TEMPLATE/feature_request.yml) template.
2. Wait for the `status:approved` label before starting work.
3. Comment on the issue to claim it, then open a PR that references it
   (`Closes #<n>`).

## Principles

- **Go only.** New behaviour ships as Go code in `cmd/` or `internal/`. Shell
  and Python are not accepted; dotfiles sourced by zsh (`home/`) are the only
  exception.
- **Declarative.** What gets installed lives in `install.manifest`; code reads
  data, it does not hard-code paths.
- **No vendored upstream.** Depend on Go modules, not copied trees.
- **Small surface.** Prefer deleting code to adding flags.

## Development setup

Requirements: Go (version in `go.mod`) and git.

```bash
git clone https://github.com/alvaroofernaandez/aval.git
cd aval
go build ./cmd/aval
go run ./cmd/aval --help
```

## Testing

Strict TDD: write the failing test first, make it pass with the minimum code,
then refactor.

```bash
aval dev check --fast   # the edit loop: gofmt, vet, tests without -race, npm tests, actionlint
aval dev check          # exactly what CI runs, on your machine
```

`aval dev check` is the single definition of "green": CI calls it too, so
there is no second list of steps to remember. It needs `actionlint` and
`gitleaks` at the pinned versions and says the exact install command when one
is missing (`aval dev check --list` shows every step and its pin). Underneath
it runs what you would type by hand:

```bash
go test ./...
go vet ./...
gofmt -l .        # must print nothing
```

## Commit convention

[Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `ci`, `perf`.
One reviewable unit per commit, with its tests. No `Co-Authored-By` or AI
attribution trailers.

## Pull requests

- Linked to an approved issue.
- Stay under ~400 changed lines; split larger work into chained PRs.
- `aval dev check` green (it is what CI runs: tests, `go vet`, `gofmt`, actionlint, gitleaks).
- Docs updated in the same PR when behaviour changes.

## AI-assisted contributions

Allowed, but you own every line: read the full diff, remove anything you
cannot explain, run the tests and report the real results.

## Code of conduct

This project follows the [Code of Conduct](CODE_OF_CONDUCT.md).

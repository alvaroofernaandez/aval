# 0001. Go only

Status: accepted (2026-11-23)

## Context

aval is a single binary that installs, syncs and verifies an environment on
macOS, Linux and Windows. `CONTRIBUTING.md` states the rule: "Go only. New
behaviour ships as Go code in `cmd/` or `internal/`. Shell and Python are not
accepted; dotfiles sourced by zsh (`home/`) are the only exception." It adds
"No vendored upstream. Depend on Go modules, not copied trees."

The architecture gate (see 0002) was evaluated against archsieve, a Node tool.
In Go it only detects import cycles, which the compiler already rejects, so it
adds no enforcement.

## Decision

All behaviour is Go in `cmd/` and `internal/`. `home/` dotfiles are the only
non-Go code that ships. Upstream code is a Go module dependency, never copied in.
Node tooling is never a dependency or a gate. archsieve may be run through
`npx` at a pinned version to produce reports (diagrams, affected packages,
churn); no test or CI step depends on it.

## Rejected alternatives

- **Shell scripts.** They do not run on Windows, and CI covers Windows.
- **Python helpers.** They need an interpreter the user may not have; the
  product is one binary.
- **Node tooling as a dependency.** It adds a runtime to every contributor and
  to CI, and archsieve gives nothing the compiler and a Go test do not.

## Consequences

- Contributors need only Go and git.
- Checks that other ecosystems do with scripts are Go tests under `tests/`.
- Report-only tools stay optional and outside the gate.

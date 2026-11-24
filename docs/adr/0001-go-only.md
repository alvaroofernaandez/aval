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

## Amendments

### 2026-11-24

A runtime shim that a host's plugin API requires may be written in that
host's language, when it is plumbing only: it forwards events to the `aval`
binary and applies the answer, and every decision stays in Go. Its pure
logic lives in a file with no host imports and is tested with `node --test`,
run from a Go test that skips without `node` (as
`internal/herdr/mods/diffreview_test.go` does); CI already sets up node for
`npm/cli`.

This records what the repository already ships, the herdr `diff-review` mod
and the OpenCode plugins, and adds the Claude Code relay mod
(`config/claude/plugins/aval-relay`, `odd/tasks/relay.md` T10, decision D5):
Claude Code's mods API only loads JavaScript or TypeScript modules (spike
#63, probe P2). Node stays out of the product: the shims run inside the host,
and `aval` remains one Go binary.

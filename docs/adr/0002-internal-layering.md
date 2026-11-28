# 0002. Layering of `internal/`

Status: accepted (2026-11-23)

## Context

`internal/` holds leaf packages (`agent/*`, `devops/*`, `term/*`, `deck`,
`design/*`, `release`, `docs/*`, `devmode`), `core/*` (`backup`, `config`,
`doctor`, `install`, `manifest`, `plan`, `platform`, `repoloc`, `sync`,
`txsync`, `uninstall`, `update`), the `ui` package with `ui/caps` and `ui/cli`,
and `cmd/aval` on top. The Go compiler rejects import cycles but not a leaf
importing the UI or core importing a screen. Without a rule the graph drifts.

## Decision

Imports among internal packages follow these edges only:

- Leaves import nothing internal.
- `core/*` imports only `core/*` and `release`.
- `ui` imports only `core/*`, `devmode`, `release` and `ui/caps`.
- `ui/cli` imports only `ui` and `ui/caps`.
- Nothing imports `cmd/aval`.
- `internal/testenv` appears only in test imports.

`tests/architecture_test.go` checks every edge against a declared allowlist,
and `docs/architecture/` holds the generated model. Exceptions are counted and
may shrink, never grow. Changing the layering requires an amendment to this
ADR first, then the allowlist.

## Rejected alternatives

- **No rule, rely on review.** Reviewers miss edges; the compiler does not
  catch layering.
- **archsieve as the gate.** In Go it only sees import cycles (see 0001).

## Consequences

- A forbidden import fails `go test ./tests/`, reported as `file -> import`.
- Moving a package between layers is an ADR amendment, not a refactor detail.

## Amendments

### 2026-11-23

The decision above lists `design/*` among the leaves and does not name every
package. The allowlist that runs, `archRules` in `tests/architecture_test.go`,
is the precise rule:

- `design/hook` may import `design/ack`: the PreToolUse gate reads the ack it
  checks. `design/ack` and `design/gate` stay leaves.
- `ui` may import `design/gate` from test files only.
- `herdr/*` and `tools/*` are leaves.
- `ui/caps` is a leaf.

`docs/architecture/model.md` is generated from that table.

### 2026-11-23

`internal/i18n` is a new leaf (ADR 0006): it holds the message catalog and
the language selection, and imports nothing internal. Who may import it:

- `cmd/aval`, already allowed everything: it resolves the language and
  renders the config warnings that need it.
- `ui`, which carries the resolved language in `Options` and its frame for
  the TUI strings (task I2 of `odd/tasks/i18n.md`).
- `ui/cli`, whose `Printer` carries the language for the human CLI output
  (task I3).

`core/*` may not import it. Core returns data and typed errors
(`config.InvalidValueError`, `config.Warning`), and the front ends turn them
into text. The language is itself a config key, so `config.Load` reports a bad
value before any language is known; only the caller can render that warning in
the language that wins. Core results also feed `--json`, which stays English.

### 2026-11-24

`internal/relay` is a new leaf: the I/O-free core of the context relay
(issue #64, plan `odd/tasks/relay.md`). It imports nothing internal. Its
adapters live below it, in `internal/relay/*`, and may import
`internal/relay` only. `cmd/aval` wires them.

`core/*`, `ui` and `agent/*` do not import relay: the hooks and the status
line reach it only through `cmd/aval`, and the relay redacts credentials with
its own table instead of importing `agent/prepush`, whose table holds
employer names, not credential shapes.

### 2026-11-28

`internal/devmode/*` is a new family of leaves: the first member is
`internal/devmode/check`, the step table and runner behind `aval dev check`
(issue #55, task T8 of `odd/tasks/ecosystem-phase-3.md`). It imports nothing
internal and nothing imports it but `cmd/aval`, which wires it to the command
line. `internal/devmode` itself stays a leaf with no members of its own to
share: the sandbox and the check do not import each other.

The family is a rule of its own because `archRules` matches `internal/devmode`
exactly; a subpackage would otherwise be a package no rule matches.

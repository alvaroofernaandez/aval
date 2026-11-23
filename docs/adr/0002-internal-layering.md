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

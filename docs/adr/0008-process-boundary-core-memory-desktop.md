# 0008. A process boundary between core, memory and desktop

Status: accepted (2026-11-24)

## Context

The roadmap in `README.md` plans two products beside the harness core: **aval
memory** and **aval context**, native ports that turn Engram, RTK and
code-review-graph into adapters (v0.3.0), and **aval desktop**, a native window
over `aval core` (v0.9.0). The owner wants each to be able to live in its own
repository.

Today the module `github.com/alvaroofernaandez/aval` (`go.mod`) exposes no
public package: everything is under `internal/`, and there is no `pkg/`
directory. The Go toolchain refuses to import an `internal/` package from
outside the tree rooted at its parent, so a second repository cannot import
core's code as it stands.

Engram is already reached across a process boundary: it is an external binary
(`internal/core/deps/deps.manifest`, `[engram]`, `probe = bin engram`) wired as
a Claude Code plugin and MCP server (`config/claude/settings.json`). Nothing in
`internal/` or `cmd/` imports it.

ADR 0002 layers `internal/` and `tests/architecture_test.go` enforces the
edges; any new package has to enter that allowlist first.

## Decision

Core, memory and desktop talk across a **process boundary**: a versioned
JSON-RPC protocol over stdio (MCP-compatible where an agent is the client).
No repository imports another's Go code.

- **Core owns the contract.** The port interfaces, the protocol specification
  and a conformance suite live in this repository. A memory or desktop
  implementation is correct when it passes the suite, not when it shares code.
- **Memory is born inside core and leaves later.** It starts as a leaf package
  `internal/memory`, added to the ADR 0002 allowlist. When it needs its own
  release cadence it becomes a nested module (`memory/go.mod`) in this
  repository. It moves to its own repository, history included
  (`git filter-repo`), only after the protocol has shipped one minor release
  without a breaking change.
- **Desktop is born in its own repository** as a client of `aval core`: it
  starts `aval` and speaks the protocol. It never links core.
- **Versioning.** The protocol carries its own version, negotiated at
  start-up. A breaking change bumps it and core keeps the previous version
  working for one minor release.

## Rejected alternatives

- **A public `pkg/` API.** It freezes core's internals as a Go API that two
  other repositories compile against; every refactor in core becomes a
  breaking change for them, and the API covers Go clients only.
- **Split memory into its own repository now.** The protocol does not exist
  yet; designing it across two repositories doubles every change while it is
  still moving. A nested module gives the same separation once it is needed.
- **One repository forever.** Desktop pulls a GUI toolchain and a release
  cadence that the CLI does not need, and keeping it out of core keeps core's
  dependency budget (`docs/dependencies.md`) intact.

## Consequences

- Core gains a protocol package and a conformance suite before any second
  implementation exists; the existing Engram integration becomes the first
  adapter measured against it.
- A memory or desktop change that needs a core change is two pull requests
  linked by protocol version, never a shared commit.
- `internal/memory` enters `archRules` in `tests/architecture_test.go` as a
  leaf; `go test ./tests/ -run Architecture` keeps it from importing `core/*`
  or `ui`.
- The extraction criteria (one minor release with no protocol break) are
  checked when the move is proposed; the move itself is an amendment here.

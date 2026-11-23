# Architecture decision records

An ADR records one decision that shapes the codebase: the context it was made
in, what was decided, what was rejected and what follows from it. Read the
relevant ADR before changing the thing it governs.

## Template

Copy this into `NNNN-kebab-title.md`:

```markdown
# NNNN. Title

Status: proposed | accepted (YYYY-MM-DD) | superseded by NNNN

## Context

What forces the decision. Cite evidence: files, commits, issues.

## Decision

What aval does, in the present tense.

## Rejected alternatives

Each option and the reason it lost.

## Consequences

What gets easier, what gets harder, and how the decision is enforced.

## Amendments

Optional. Dated entries, newest last.

### YYYY-MM-DD

What changed and why.
```

## Rules

- **Never rewrite an accepted ADR.** If a decision changes, add a dated entry
  under `## Amendments` or write a new ADR that supersedes it and mark the old
  one `Status: superseded by NNNN`.
- Numbers are contiguous from 0001 and never reused. File names are
  `NNNN-kebab-case.md`.
- Every claim cites evidence from the repo, and is verified before it is written.
- Every ADR is listed in the index below.
- `tests/adr_test.go` enforces the sections, the `Status:` line, the numbering,
  the index and dated amendments.

## Index

| ADR | Status | Decision |
| --- | --- | --- |
| [0001](0001-go-only.md) | Accepted (2026-11-23) | aval is written in Go only; no shell, Python or Node dependency. |
| [0002](0002-internal-layering.md) | Accepted (2026-11-23) | `internal/` is layered as leaves, `core`, `ui`, `ui/cli`, `cmd`, and a test enforces the edges. |
| [0003](0003-pre-push-scans-ci-tests.md) | Accepted (2026-11-23) | Pre-push scans outgoing commits; CI runs the tests on three operating systems. |
| [0004](0004-hub-writes-nothing-offline.md) | Accepted (2026-11-23) | Opening the hub writes nothing and makes no network call. |
| [0005](0005-cost-aware-context-relay.md) | Proposed (2026-11-23) | The context relay triggers on absolute tokens and cache state, not a window percentage. |
| [0006](0006-i18n-human-output.md) | Accepted (2026-11-23) | Human output is English or Spanish; machine output stays English. |
| [0007](0007-hooks-may-start-a-resolved-tool.md) | Accepted (2026-11-23) | A hook may start an already resolved tool under a timeout, never on its no-op path. |

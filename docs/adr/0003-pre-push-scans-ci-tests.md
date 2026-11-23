# 0003. Pre-push scans while CI tests

Status: accepted (2026-11-23)

## Context

The header of `internal/agent/prepush/prepush.go` defines the pre-push hook as
a gate that "refuses outgoing commits that carry employer material or
secrets". It scans only the commits between the remote's tip and the local
one, so uncommitted work never blocks a push and old commits are not re-judged.
`.githooks/pre-push` delegates to `aval hook pre-push`, and
`tests/pre_push_test.go` covers the shim, the versioned hooks and a real
`git push` through the hook.

`.github/workflows/ci.yml` runs `go test ./... -race` on ubuntu, macOS and
windows (a matrix with `fail-fast: false`) on pushes to main and pull
requests, with full history so gitleaks can scan every commit.

## Decision

Pre-push scans and never runs the test suite. CI runs the tests, on three
operating systems. `git push --no-verify` is the deliberate bypass.

## Rejected alternatives

- **Running tests in pre-push** (the hagaharness repo does this). The suite
  needs tools and a built binary, makes every push slow, and covers only the
  developer's own OS; CI covers three.
- **No hosted CI.** Nothing would test Windows or macOS, and the hook alone
  cannot prove the suite passes.

## Consequences

- A push is fast and fails only for a finding in the outgoing commits.
- A failing test is caught in CI, not locally at push time.
- `tests/workflows_test.go` keeps the workflow itself honest.

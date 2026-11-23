# 0004. The hub writes nothing and stays offline

Status: accepted (2026-11-23)

## Context

Bare `aval` opens the hub on a terminal. `.agents/DESIGN.md`, section 10
(Installer TUI and CLI output), subsection Hub, says: "Opening the hub writes
nothing and makes no network calls. The doctor summary is read-only, and the
update line is read from the cache that the last update check left, never
fetched."

In code, `cmd/aval/hub.go` wires `CachedNotice` to `update.CachedWith`, which
reads the cache. In `internal/ui/services.go`, `RepoStatus` is documented as
reading "locally ... how many commits the repo is behind its last fetched
upstream and whether its tree has uncommitted changes. ... It never fetches
and never writes."

## Decision

Opening the hub and quitting changes no file under HOME or the cache
directory and makes no network request. The update badge comes from the cached
notice; the repo badge comes from local git state only. Anything that writes
or fetches is an explicit action the user chooses inside a view, never a
side effect of opening.

## Rejected alternatives

- **Refresh the update check on open.** It adds latency and a network call to
  a screen that must open instantly and work offline.
- **Fetch the upstream to show an exact behind count.** It writes to `.git`
  and hits the network; the count from the last fetch is enough for a badge.

## Consequences

- Badges can be stale; they reflect the last check or fetch.
- `TestOpeningTheHubWritesNothingAndUsesNoNetwork` in `cmd/aval/hub_test.go`
  enforces it: it hashes HOME and the cache before and after, and replaces
  the HTTP transport with one that fails the test on any request.

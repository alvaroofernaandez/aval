---
name: aval-release
description: "Use when cutting, checking or publishing an aval release: versioning, the CHANGELOG, an rc or beta, promote, tags, GoReleaser. An ordered checklist that stops at the first failure; publishing only with explicit authorization."
---

## Activation contract

Load before `aval release` or any tag, version or CHANGELOG work. Maintainer reference: `docs/releasing.md`. Where it disagrees with a global skill, this one wins (`AGENTS.md`).

## Hard rules

- **Never push unasked.** `aval release rc|beta|patch|minor|major|promote` creates an annotated tag and runs `git push origin <tag>`. That is a remote, public action: run a cutting command only after the owner's explicit authorization for that exact version. Without it, stop at `--dry-run`.
- The `release` environment approval in GitHub is the owner's alone. Never ask for or simulate it.
- Prerelease (`-rc.N`, `-beta.N`) or stable is the owner's decision.
- Never print or commit tokens (`NPM_TOKEN`, `GH_TOKEN`).
- Conventional Commits, no AI attribution: the release notes are generated from commit subjects.

## Checklist (in order; stop at the first failure and report it)

1. **Clean `main`.** `git status` clean, on `main`, up to date with `origin/main`. `aval release` refuses otherwise.
2. **CHANGELOG.** `CHANGELOG.md` has its `## Unreleased` section describing what ships (`tests/hub_docs_test.go` pins parts of it). From v0.1.0 on, the notes come from the GitHub release that `aval release` and GoReleaser generate from conventional commits; do not hand-write past entries. Read the grouped notes `aval release status` prints (Breaking, Features, Fixes, Performance); if a subject reads badly, it is fixed in history only by the owner, never by a rewrite you start.
3. **Tests with the race detector.** `go test -race ./...` passes locally (CI runs the same on three OSes; `aval release` itself runs `go test ./...` without `-race`). Also `go vet ./...` and `gofmt -l .` empty.
4. **GoReleaser pin.** `.github/workflows/release.yml` pins `GORELEASER_VERSION` and `GORELEASER_SHA256` (the hash of `goreleaser_Linux_x86_64.tar.gz` from that release's `checksums.txt`). Upgrade only deliberately, in its own commit, after `goreleaser release --snapshot --clean --skip=publish`.
5. **Status and dry run.** `aval release status`, then the chosen command with `--dry-run`: check the version (before 1.0, `feat` or breaking bumps the minor, `fix`/`perf` the patch; `docs`, `test`, `ci`, `chore` alone mean nothing to publish).
6. **Tag (authorized only).** The cutting command: it checks clean tree, `main`, up to date, `go test ./...`, green CI on `HEAD` when `gh` is available, shows the notes, asks `y/N`, tags and pushes the tag. `promote` re-tags the rc's commit; if `main` moved, cut another rc.
7. **Verify.** The workflow URL it prints: `preflight`, `publish` (after the owner approves the environment), `verify`, `npm`. Then by hand:

```bash
gh release download vX.Y.Z -R alvaroofernaandez/aval
shasum -a 256 -c SHA256SUMS --ignore-missing      # Linux: sha256sum -c
gh attestation verify aval_X.Y.Z_<os>_<arch>.tar.gz --repo alvaroofernaandez/aval
tar -xzf aval_X.Y.Z_<os>_<arch>.tar.gz && ./aval version
npx @alvaroofernaandez/aval@next version          # rc/beta; @latest for stable
```

## If something fails

Follow `docs/releasing.md`, "If something fails". Deleting a remote tag (`git push origin :refs/tags/<tag>`) is a remote action: only with the owner's authorization, and only before anything was approved.

## Mechanisms behind this checklist

- `internal/release` (`internal/release/git_test.go`): the preflight checks, the version arithmetic and the notes grouping.
- `cmd/aval/release_test.go`: the command wiring.
- ADR 0003: CI, not pre-push, runs the tests; a release requires CI green on the tagged commit.

## Report

The version, each checklist step with its command and result, what was not run and why, and the exact point you stopped for authorization.

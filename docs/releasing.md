# How to publish a release

aval uses [SemVer](https://semver.org): `vX.Y.Z` for stable releases and `vX.Y.Z-rc.N` / `vX.Y.Z-beta.N`
for prereleases. A release is an **annotated tag** on `main`; pushing it makes `release.yml` do
the rest. A summarised history is in [CHANGELOG.md](../CHANGELOG.md); the full notes of each
release are written by hand in [docs/releases/](releases/) in English and Spanish, and the GitHub
release body is those notes followed by GoReleaser's generated commit list.

## Release notes

Before cutting, write `docs/releases/vX.Y.Z.md` (English) and `docs/releases/vX.Y.Z.es.md` (Spanish;
a prerelease `vX.Y.Z-rc.N` uses the base version's files), and give the `CHANGELOG.md` section
`## vX.Y.Z-rc.N (date)` a line linking both. `tests/release_notes_test.go` fails if a section from
v0.2.0 on lacks either link or file. Every number in the notes is measured from the repo.

After the publish succeeds (GoReleaser has created the release with its generated list), set the
body once:

```sh
gh release view vX.Y.Z-rc.N -R alvaroofernaandez/aval --json body -q .body > generated.md
cat docs/releases/vX.Y.Z.md docs/releases/vX.Y.Z.es.md generated.md > body.md   # English, Spanish, generated
gh release edit vX.Y.Z-rc.N -R alvaroofernaandez/aval --notes-file body.md
```

The release page does not resolve repo-relative paths, so rewrite them in `body.md` first: links
(`../adr/x.md`, `../../CHANGELOG.md`, `v0.2.0.es.md`) to
`https://github.com/alvaroofernaandez/aval/blob/<tag>/<path from the repo root>`, and images
(`../assets/x.gif`, `../assets/x.svg`) to `https://raw.githubusercontent.com/alvaroofernaandez/aval/<tag>/<path>`.
Resolve each against `docs/releases/`. Mermaid does not render in a release body (checked on
v0.2.0-rc.1: the page shows it as a code block), so replace each diagram in `body.md` with a
table that says the same; the files in the repo keep the diagrams. Put the generated list inside
`<details><summary>All commits · Todos los commits</summary>` so the notes lead. Keep `body.md`
out of the repo.

## The flow (3 commands)

```sh
aval release status    # latest tag, unpublished commits, suggested version
aval release rc        # tag v0.2.0-rc.1 (the next rc adds 1)
aval release promote   # the rc becomes v0.2.0, on the same commit
```

Each cutting command (`rc`, `beta`, `patch`, `minor`, `major`, `promote`):

1. checks: clean tree, `main` branch, up to date with `origin/main`, `go test ./...` and green CI on
   `HEAD` (the latter only if `gh` is available);
2. shows the version, the commits and the grouped notes (Breaking, Features, Fixes, Performance;
   `docs`, `test`, `ci` and `chore` do not appear);
3. asks `y/N` (`--yes` skips it; `--dry-run` only shows and touches nothing);
4. creates the annotated tag and pushes it with `git push origin <tag>`;
5. prints the workflow URL: **the `release` environment still needs approval** (only the owner can do it).

Shortcuts: `aval release patch|minor|major` publishes a stable release without going through an rc.

### Which version comes out

Before 1.0: `feat` and a breaking change (`!` or `BREAKING CHANGE:`) bump the minor;
`fix` and `perf` bump the patch. From 1.0 it is standard SemVer. `docs`, `test`, `ci` and `chore` do not
justify a version: if that is all there is, `aval release` says there is nothing to publish.

`promote` only tags the rc's commit, so the stable binary is the one you already tested.
If `main` has moved on since the rc, it refuses: cut another rc.

## What each job does

| Job | Permissions | Checks / does |
| :--- | :--- | :--- |
| `preflight` | read-only | the tag has the form `vX.Y.Z[-rc.N\|-beta.N]`, is annotated and is on `origin/main`; `ci.yml` finished successfully on that commit; `goreleaser check` and a `release --snapshot` build all 6 targets |
| `publish` | `contents: write`, `id-token`, `attestations`; **`release` environment** | GoReleaser builds, creates the release (a prerelease if it contains `-`) and uploads archives + `SHA256SUMS`; provenance attestation on each archive |
| `verify` | read-only | downloads what was published: `sha256sum -c SHA256SUMS`, `gh attestation verify` on each archive, and that the binary prints the tag's version |
| `npm` | **`release` environment**, `id-token` | reuses the same already-verified binaries; publishes with `--tag next` if the version contains `-`, `latest` otherwise |

Commits on `main` only run `ci.yml`; they never publish.

## Trying an rc

```sh
npx @alvaroofernaandez/aval@next version     # from npm
gh release download v0.2.0-rc.1 -R alvaroofernaandez/aval --pattern '*linux_amd64*'
```

Stable builds notify about news on `latest`; prerelease builds, on `next` (and on
`latest`, to learn that their stable release is out). A development build (`dev`) never notifies.
`AVAL_NO_UPDATE_CHECK=1` disables it.

## Verifying what you download

```sh
gh release download v0.2.0 -R alvaroofernaandez/aval
sha256sum -c SHA256SUMS --ignore-missing          # macOS: shasum -a 256 -c SHA256SUMS --ignore-missing
gh attestation verify aval_0.2.0_linux_amd64.tar.gz --repo alvaroofernaandez/aval
tar -xzf aval_0.2.0_linux_amd64.tar.gz && ./aval version
```

## If something fails

- **The tag is wrong and nothing has been approved yet:** `git push origin :refs/tags/<tag>` and `git tag -d <tag>`.
- **`verify` or `npm` failed:** re-run the workflow from Actions (*Run workflow* with the tag);
  the asset upload replaces existing ones and `npm` skips what is already published.
- **Upgrading GoReleaser:** change `GORELEASER_VERSION` and `GORELEASER_SHA256` in `release.yml` (the hash
  is in the `checksums.txt` of its release, line `goreleaser_Linux_x86_64.tar.gz`) and test with
  `goreleaser release --snapshot --clean --skip=publish`.

## One-time requirements

A `release` environment with owner approval and the `NPM_TOKEN` secret (an automation token with
publish permission on `@alvaroofernaandez`). The moving `latest` tag and its "Latest build"
release from before this system were deleted when v0.1.0 shipped.

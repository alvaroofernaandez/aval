# Release v0.2.0-rc.1: the hub, the new TUI and Spanish

Status: approved by the owner on 2026-11-23 ("si, procede").

**Goal:** merge `feat/tui-experience` into `main` and cut `v0.2.0-rc.1`, with release notes the owner asked for: detailed, explanatory, in English and Spanish, and visual.

**Why a minor rc:** the branch adds features (`feat` commits), so before 1.0 the minor goes up. It is an rc because it is a large change (112 commits) and the Spanish TUI has only been checked by tests, not by hand.

## Decisions

- **Hand-written notes per release.** `docs/releases/vX.Y.Z.md` (English) and `docs/releases/vX.Y.Z.es.md` (Spanish) explain the release. The GitHub release body is those notes followed by GoReleaser's generated commit list. This amends the "CHANGELOG is not edited by hand" rule in `CHANGELOG.md` and `docs/releasing.md`. The rejected alternative was keeping commit subjects only, because they do not explain anything to a user.
- **`CHANGELOG.md` gets a dated section per release** with a short summary and links to both notes. It does not repeat them.
- **Visuals are real.** The GIFs are recorded with vhs from this checkout, including a new Spanish hub GIF. Every number in the notes is measured. Diagrams are Mermaid in the repo notes only, because the release body may not render it.
- **The workflow is unchanged.** GoReleaser and `release.yml` stay as they are. The bilingual body is applied after publish with `gh release edit`.

## Tasks

- [x] **R1: notes and changelog.**
  - Write `docs/releases/v0.2.0.md` and `docs/releases/v0.2.0.es.md`.
  - Rename the CHANGELOG section to `v0.2.0-rc.1`, linking both notes.
  - Amend `docs/releasing.md` and the `aval-release` skill.
  - Add a docs test that each release section links to notes that exist in both languages (RED first).
- [x] **R2: Spanish hub GIF.**
  - Add `docs/hub-demo-es.tape`, a copy of `docs/hub-demo.tape` with `AVAL_LANG=es`, producing `docs/assets/hub-es.gif`.
  - The Spanish notes show it.
- [x] **R3: PR and merge.**
  - Open a PR from `feat/tui-experience` into `main` and wait for CI on ubuntu, macos and windows.
  - Merge with a merge commit, so the conventional commit subjects survive for GoReleaser.
- [x] **R4: cut `v0.2.0-rc.1`.**
  - On a clean `main`, run `aval release status`, then `aval release rc --dry-run`, then the cut.
  - The `release` environment approval is the owner's.
- [x] **R5: bilingual release body and verification.**
  - After publish, set the body with `gh release edit`.
  - Verify the checksums, the attestation, `aval version` and `npx @alvaroofernaandez/aval@next version`.
  - Clean up the merged branches.

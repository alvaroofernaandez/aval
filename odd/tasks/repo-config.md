# Repo location config and explicit `aval install`

Why: a release binary (npm, GitHub) never lives inside the repository, so
`resolveRepo` only worked when run from the checkout. And a bare `aval`
installed dotfiles, a write to $HOME as the default action.

## T1 — internal/core/repoloc
- [x] RED tests: precedence flag > AVAL_REPO > config > cwd > executable; explicit
      sources that point nowhere fail loudly; config path honours XDG_CONFIG_HOME;
      Save/Load round trip
- [x] GREEN: repoloc.go; cmd/aval resolveRepo delegates to it

## T2 — install records the repo location
- [x] RED: finishInstall writes the config; --dry-run does not
- [x] GREEN: core_seed.go

## T3 — bare `aval` prints help; `aval install` is the explicit command
- [x] RED: no args → usage on stdout; `install` parses --dry-run/--yes/--copy;
      legacy `aval --yes` still installs with a deprecation notice
- [x] GREEN: main.go dispatch, devmode bootstrap uses `install --yes`

## T4 — docs: README, guide, npm README, devmode, CHANGELOG breaking note

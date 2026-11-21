#!/usr/bin/env bash
# Link this repository's configs into $HOME.
#
# Existing files and directories are never deleted: anything in the way is moved
# aside to <path>.bak.<timestamp> before the symlink is created. Re-running is
# safe — links already pointing at this repo are left alone.
#
#   ./install.sh            link everything
#   ./install.sh --dry-run  show what would happen, change nothing
set -uo pipefail

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"

DRY_RUN=0
for arg in "$@"; do
  case "$arg" in
    --dry-run | -n) DRY_RUN=1 ;;
    -h | --help) sed -n '2,10p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

# What gets linked lives in install.manifest, not here. The cross-platform
# installer reads the same file, and a second copy of the list would diverge the
# first time an entry was added to one and not the other — silently, because a
# missing entry just means the config never gets linked.
#
# tests/manifest.test.sh fails if this script and the manifest ever disagree.
MANIFEST="${MANIFEST:-$REPO_ROOT/install.manifest}"

if [ ! -f "$MANIFEST" ]; then
  echo "install: manifest not found: $MANIFEST" >&2
  exit 1
fi

LINKS=()
fanout_source=""
fanout_dests=""

# Section headers and group metadata are read but not acted on here: install.sh
# installs every group. Platform filtering is the TUI's job, since this script
# only ever runs on Unix.
while IFS= read -r line; do
  line="${line%%#*}"                       # strip comments
  line="${line#"${line%%[![:space:]]*}"}"  # trim leading space
  line="${line%"${line##*[![:space:]]}"}"  # trim trailing space
  [ -n "$line" ] || continue

  case "$line" in
    \[*\]) continue ;;
    fanout-source*=*) fanout_source="${line#*=}"; fanout_source="${fanout_source## }" ;;
    fanout-dests*=*)  fanout_dests="${line#*=}" ;;
    *=*) continue ;;                       # label / detail / platforms
    *)
      src="${line%%[[:space:]]*}"
      dest="${line##*[[:space:]]}"
      [ "$src" != "$dest" ] && LINKS+=("$src:$dest")
      ;;
  esac
done < "$MANIFEST"

# The fan-out is expanded here rather than listed, so adding a skill directory
# needs no manifest edit at all.
if [ -n "$fanout_source" ]; then
  for skill_path in "$REPO_ROOT/$fanout_source"/*/; do
    [ -d "$skill_path" ] || continue
    skill="$(basename "$skill_path")"
    for skill_dir in $fanout_dests; do
      LINKS+=("$fanout_source/$skill:$skill_dir/$skill")
    done
  done
fi

if [ "${#LINKS[@]}" -eq 0 ]; then
  echo "install: manifest declared no entries: $MANIFEST" >&2
  exit 1
fi

stamp="$(date +%Y%m%d%H%M%S)"

# Backups are quarantined here rather than left beside the original. Tools scan
# their config directories wholesale — Claude Code loads every entry under
# ~/.claude/skills — so a "ship.bak.<stamp>" sitting next to "ship" would be
# picked up as a second, duplicate skill.
BACKUP_ROOT="$HOME/.aval-backup/$stamp"

failed=0

say() { printf '  %-24s %s\n' "$1" "$2"; }

for entry in "${LINKS[@]}"; do
  src="$REPO_ROOT/${entry%%:*}"
  dest="$HOME/${entry##*:}"
  name="${entry##*:}"

  if [ ! -e "$src" ]; then
    echo "install: missing source: $src" >&2
    failed=1
    continue
  fi

  # Already pointing where it should — nothing to do.
  if [ -L "$dest" ] && [ "$(readlink "$dest")" = "$src" ]; then
    say "$name" "already linked"
    continue
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    if [ -e "$dest" ] || [ -L "$dest" ]; then
      say "$name" "would back up, then link"
    else
      say "$name" "would link"
    fi
    continue
  fi

  mkdir -p "$(dirname "$dest")"

  # -e is false for a broken symlink, so test -L too or it would survive.
  if [ -e "$dest" ] || [ -L "$dest" ]; then
    backup="$BACKUP_ROOT/$name"
    mkdir -p "$(dirname "$backup")"
    mv "$dest" "$backup"
    say "$name" "backed up -> ~/.aval-backup/$stamp/$name"
  fi

  ln -s "$src" "$dest"
  say "$name" "linked"
done

# ~/.claude/settings.json is owned by `aval sync`, never linked: a link would
# make sync write personal keys and the private overlay into this repo. Seed it
# from the repo when missing (or when an older install left a link), then let
# `aval sync` merge the repo-managed keys if the binary is available.
settings="$HOME/.claude/settings.json"
if [ "${DRY_RUN:-0}" != 1 ]; then
  if [ -L "$settings" ] || [ ! -e "$settings" ]; then
    mkdir -p "$HOME/.claude"
    rm -f "$settings"
    cp "$REPO_ROOT/config/claude/settings.json" "$settings"
    say "claude settings (aval sync)" "seeded" >&2
  fi
  for bin in "$HOME/.local/bin/aval" "$(command -v aval 2>/dev/null || true)"; do
    if [ -n "$bin" ] && [ -x "$bin" ]; then "$bin" sync --repo "$REPO_ROOT" >/dev/null 2>&1 || true; break; fi
  done
else
  say "claude settings (aval sync)" "would seed" >&2
fi

# Versioned git hooks: pre-push refuses employer material and secrets in
# outgoing commits. Skipped on --dry-run and outside a git checkout.
if [ "${DRY_RUN:-0}" != 1 ] && git -C "$REPO_ROOT" rev-parse --git-dir >/dev/null 2>&1; then
  git -C "$REPO_ROOT" config core.hooksPath .githooks
fi

[ "$failed" -eq 0 ] || exit 1

#!/usr/bin/env bash
# Tests for install.manifest — the single source of truth for what gets
# installed and where.
#
# The manifest exists so there is exactly one list of links: the Go installer
# under cmd/aval (TUI and `--yes`) reads it, and nothing else carries a copy
# that could diverge the first time someone adds an entry to one and not the
# other. The failure would be silent: the config simply never gets linked.
#
# So the contract these tests defend is narrow and specific:
#   1. the manifest parses, and every group declares a platform
#   2. every source it names actually exists in the repo
#   3. the Go installer installs exactly what the manifest says — no more, no less
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="$REPO/install.manifest"

pass=0
fail=0

ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "manifest"

# --- it exists ---------------------------------------------------------------
assert_eq "install.manifest exists" "yes" "$([ -f "$MANIFEST" ] && echo yes || echo no)"
[ -f "$MANIFEST" ] || { printf '\n%d passed, %d failed\n' "$pass" $((fail + 1)); exit 1; }

# --- every group declares a valid platform -----------------------------------
# A group with no platform would be silently skipped everywhere, or installed
# everywhere: both are wrong, and neither shows up as an error.
groups="$(rg -o '^\[([a-z-]+)\]' -r '$1' "$MANIFEST")"
group_count="$(printf '%s\n' "$groups" | rg -c . || echo 0)"
assert_eq "declares at least one group" "yes" \
  "$([ "$group_count" -ge 1 ] && echo yes || echo no)"

bad_platform=0
for g in $groups; do
  p="$(rg -A6 "^\[$g\]" "$MANIFEST" | rg -o '^platforms\s*=\s*(\S+)' -r '$1' | head -1)"
  case "$p" in
    unix | all) ;;
    *) echo "     group [$g] has platform '$p'"; bad_platform=1 ;;
  esac
done
assert_eq "every group declares platforms as unix or all" "0" "$bad_platform"

bad_label=0
for g in $groups; do
  rg -A6 "^\[$g\]" "$MANIFEST" | rg -q '^label\s*=\s*\S' || { echo "     group [$g] has no label"; bad_label=1; }
done
assert_eq "every group carries a label for the TUI" "0" "$bad_label"

# --- every source exists -----------------------------------------------------
# A manifest entry naming a path that was renamed or deleted makes the installer
# fail at run time, on the user's machine, halfway through installing.
missing=0
while read -r src _dest; do
  [ -n "$src" ] || continue
  [ -e "$REPO/$src" ] || { echo "     missing source: $src"; missing=1; }
done < <(rg '^\S+\s+\S+$' "$MANIFEST" | rg -v '^\s*#' | rg -v '=')
assert_eq "every source named in the manifest exists" "0" "$missing"

# --- the fan-out target exists ----------------------------------------------
fanout_src="$(rg -o '^fanout-source\s*=\s*(\S+)' -r '$1' "$MANIFEST" | head -1)"
assert_eq "the fan-out source directory exists" "yes" \
  "$([ -n "$fanout_src" ] && [ -d "$REPO/$fanout_src" ] && echo yes || echo no)"

# --- the Go installer and the manifest agree ---------------------------------
# The heart of it. `aval --yes --dry-run` prints one destination per line; the
# manifest plus its fan-out must expand to exactly that set. Any drift in
# either direction fails here rather than on someone's machine.
. "$REPO/tests/lib/aval-bin.sh"
BIN_DIR="$(mktemp -d)"
trap 'rm -rf "$BIN_DIR"' EXIT
AVAL="$(build_aval "$BIN_DIR")"
assert_eq "the Go installer builds" "yes" "$([ -n "$AVAL" ] && echo yes || echo no)"

actual_dests="$(HOME="$BIN_DIR/home" "$AVAL" --yes --dry-run --repo "$REPO" 2>/dev/null \
  | rg -o '~/\S+' | sed 's|^~/||' | sort -u)"

expected_dests="$(
  rg '^\S+\s+\S+$' "$MANIFEST" | rg -v '^\s*#' | rg -v '=' | awk '{print $2}'
  if [ -n "$fanout_src" ]; then
    for d in $(rg -o '^fanout-dests\s*=\s*(.+)$' -r '$1' "$MANIFEST" | head -1); do
      for s in "$REPO/$fanout_src"/*/; do
        [ -d "$s" ] && echo "$d/$(basename "$s")"
      done
    done
  fi
)"
expected_dests="$(printf '%s\n' "$expected_dests" | sort -u)"

assert_eq "the Go installer installs exactly what the manifest declares" \
  "" "$(diff <(printf '%s\n' "$expected_dests") <(printf '%s\n' "$actual_dests") | head -20)"

assert_eq "the count is non-trivial" "yes" \
  "$([ "$(printf '%s\n' "$actual_dests" | rg -c .)" -ge 40 ] && echo yes || echo no)"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

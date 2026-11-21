#!/usr/bin/env bash
# Lint every agent config in the repo (skills, CLAUDE.md, hooks, MCP, plugins)
# with agnix, pinned. Errors fail the suite; warnings and info are reported by
# `npx agnix validate .` on demand. Skipped when node is not installed.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGNIX_VERSION="0.61.0"

pass=0
fail=0
ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "agnix"

if ! command -v npx >/dev/null 2>&1; then
  echo "  (node not installed; agnix skipped)"
  printf '\n%d passed, %d failed\n' 1 0
  exit 0
fi

out="$(cd "$REPO" && npx -y "agnix@$AGNIX_VERSION" validate . 2>&1)"
errors="$(printf '%s\n' "$out" | rg -c ' error: ' || true)"
assert_eq "agnix reports no errors in any agent config" "0" "${errors:-0}"
[ "${errors:-0}" = "0" ] || printf '%s\n' "$out" | rg ' error: ' | head -10

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

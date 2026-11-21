#!/usr/bin/env bash
# Guards the cost of Claude Code hooks.
#
# A hook runs on every event it matches, so its cost is paid over and over:
# measured 2026-11-21, `gentle-ai sdd-preflight-hook` took 657 ms on every
# subagent launch and `code-review-graph update` 154 ms on every edit. SDD has
# been dropped from this harness, so its hooks must not come back, and work
# that only needs to happen eventually must not block the agent.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SETTINGS="$REPO/config/claude/settings.json"

pass=0
fail=0

ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "agent-hooks"

assert_eq "settings.json is valid JSON" "0" "$(jq empty "$SETTINGS" >/dev/null 2>&1; echo $?)"

commands="$(jq -r '.hooks // {} | .[][] | .hooks[] | .command' "$SETTINGS" 2>/dev/null)"

assert_eq "no hook runs SDD" "" \
  "$(printf '%s\n' "$commands" | rg -i 'sdd' | tr '\n' ' ')"
assert_eq "no hook refreshes the SDD skill registry" "" \
  "$(printf '%s\n' "$commands" | rg 'skill-registry' | tr '\n' ' ')"

# Re-indexing the code graph is bookkeeping: it must not hold up an edit.
sync_graph="$(jq -r '.hooks.PostToolUse // [] | .[].hooks[]
  | select(.command | test("code-review-graph update")) | (.async // false)' "$SETTINGS")"
assert_eq "code-review-graph update runs async" "true" "${sync_graph:-missing}"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

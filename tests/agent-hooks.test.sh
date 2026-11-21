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

# The live settings file is a separate copy, not a symlink: gentle-ai rewrites
# it, and it holds personal keys that cannot be published. Hooks are the part
# that must never drift — on 2026-11-21 the live copy still called a retired
# `gentle-ai sdd-preflight-hook`, and every Agent call failed with
# "unknown command". So aval owns the hooks and the live copy must match them.
# 21 commits in 60 days carried Co-Authored-By despite the rule: Claude Code
# itself injects the trailer unless the setting hides it. A rule the harness
# contradicts on every commit is a coin flip; the setting is not.
attr() { jq -r '[.attribution.commit, .attribution.pr] | map(. // "unset") | join(",")' "$1"; }
assert_eq "aval hides commit and PR attribution" "," "$(attr "$SETTINGS")"

# Terax is not installed; its three hooks still forked a shell on every
# prompt, notification and stop just to test an empty variable.
assert_eq "no hook serves the uninstalled Terax terminal" "0" \
  "$(jq '[.hooks // {} | .[][] | .hooks[] | select(.command | test("TERAX"))] | length' "$SETTINGS")"

# Agents pinned retired IDs (claude-opus-4-6) or OpenCode's provider/model
# form (anthropic/claude-sonnet-4-5), which Claude Code does not resolve.
# Aliases always track the current model.
bad_models="$(rg -H -N '^model:' "$REPO/config/claude/agents/" 2>/dev/null \
  | rg -v 'model: *(opus|sonnet|haiku|inherit) *$' | sd '.*/' '' | tr '\n' ' ')"
assert_eq "every Claude agent uses a model alias" "" "$bad_models"

LIVE="${AVAL_LIVE_SETTINGS:-$HOME/.claude/settings.json}"
if [ -f "$LIVE" ]; then
  hooks_of() { jq -S '[.hooks // {} | to_entries[] | .key as $e | .value[] | .hooks[] | "\($e) \(.command)"] | sort' "$1"; }
  assert_eq "the live settings run exactly aval's hooks" "$(hooks_of "$SETTINGS")" "$(hooks_of "$LIVE")"
  retired="$(jq -r '.hooks // {} | .[][] | .hooks[] | .command' "$LIVE" | rg 'sdd-|skill-registry' || true)"
  assert_eq "no live hook calls a retired SDD command" "" "$retired"
  assert_eq "the live settings hide commit and PR attribution" "," "$(attr "$LIVE")"
fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

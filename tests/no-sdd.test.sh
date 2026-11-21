#!/usr/bin/env bash
# SDD (Spec-Driven Development) is dropped from this harness, 2026-11-21.
#
# It cost on every turn: instruction text loaded whether or not a session
# touched it, a 657 ms hook on every subagent launch, eleven skills and eleven
# agents competing for triggers. gentle-ai 4.6.0 reports the component as
# retired, so nothing reinstalls it — which also means nothing removes the
# leftovers but this repo. This suite keeps them from creeping back.
#
# Retired material lives only in git history; this suite and odd/ task logs
# are the only places the word may still appear.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

pass=0
fail=0

ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "no-sdd"

leftovers="$(cd "$REPO" && fd -H -i '^(sdd([-_.].*)?|skill-registry|openspec.*|sdd)$' \
  shared/skills config/claude config/opencode 2>/dev/null | tr '\n' ' ')"
assert_eq "no SDD skills, agents, prompts or profiles remain" "" "$leftovers"

assert_eq "no _shared SDD/openspec contracts remain" "" \
  "$(cd "$REPO" && fd -i '^(sdd-|openspec-)' shared/skills/_shared 2>/dev/null | tr '\n' ' ')"

# Active instructions, skills, hooks, plugins and scripts must not mention it.
mentions="$(cd "$REPO" && rg -l -i '\bsdd\b|sdd-|openspec|skill-registry' \
  config shared/skills scripts docs install.manifest README.md \
  -g '!shared/skills-retired/**' -g '!*.bak-*' 2>/dev/null | tr '\n' ' ')"
assert_eq "no active file mentions SDD, openspec or skill-registry" "" "$mentions"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

#!/usr/bin/env bash
# Tests for the Herdr workspace setup.
#
# Herdr (herdr-app window + herdr-core `hc`) replaced Orca on 2026-11-19.
# Claude Code reports its status to Herdr natively (OSC 7501), so no agent
# hooks may come back: Orca's 16 hooks ran a shell on every single tool call.
# The skill is a stub that loads the version-matched manuals (`hc --skill`,
# `herdr-app --skill`) so it never drifts from the installed release.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKILL="$REPO/shared/skills/herdr/SKILL.md"
TEMPLATES="$REPO/config/herdr-core/templates"
SETTINGS="$REPO/config/claude/settings.json"
CLAUDE_MD="$REPO/config/claude/CLAUDE.md"
AGENTS_MD="$REPO/config/opencode/AGENTS.md"

pass=0
fail=0
ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }
has() { rg -q "$1" "$2" 2>/dev/null && echo yes || echo no; }

shared_block() {
  awk '$0=="<!-- shared:agent-rules -->"{f=1;next} $0=="<!-- /shared:agent-rules -->"{f=0} f' "$1" 2>/dev/null
}

echo "herdr"

# --- skill ---------------------------------------------------------------------
assert_eq "skill: SKILL.md exists" "yes" "$([ -f "$SKILL" ] && echo yes || echo no)"
assert_eq "skill: frontmatter name matches dir" "yes" "$(has '^name: herdr$' "$SKILL")"
assert_eq "skill: description is one quoted line" "yes" "$(has '^description: ".+"$' "$SKILL")"
assert_eq "skill: own skills carry no license field" "no" "$(has '^license:' "$SKILL")"
assert_eq "skill: loads the version-matched hc manual" "yes" "$(has 'hc --skill' "$SKILL")"
assert_eq "skill: loads the version-matched window manual" "yes" "$(has 'herdr-app --skill' "$SKILL")"
assert_eq "skill: agents always start with --bg" "yes" "$(has 'hc run --bg' "$SKILL")"
assert_eq "skill: waits with hc wait, never sleep" "yes" "$(has 'hc wait' "$SKILL")"
assert_eq "skill: never stops or restarts the user's core" "yes" "$(has '(?i)never .*hc server (stop|restart)' "$SKILL")"
assert_eq "skill: forbids broad process kills" "yes" "$(has '(?i)never .*pkill' "$SKILL")"
assert_eq "skill: states status needs no hooks" "yes" "$(has '(?i)no (agent )?hooks' "$SKILL")"
assert_eq "skill: Codex runs with --no-daemon" "yes" "$(has 'no-daemon' "$SKILL")"
assert_eq "skill: wraps one-off commands in an interactive zsh for .zshrc PATH" "yes" "$(has "zsh -ic" "$SKILL")"
assert_eq "skill: does not drive Orca" "no" "$(has '(?i)orca' "$SKILL")"

# --- hc templates ---------------------------------------------------------------
for t in reviewer implementer; do
  f="$TEMPLATES/$t.md"
  assert_eq "template $t: exists" "yes" "$([ -f "$f" ] && echo yes || echo no)"
  assert_eq "template $t: frontmatter name" "yes" "$(has "^name: $t\$" "$f")"
  assert_eq "template $t: has description" "yes" "$(has '^description: .+' "$f")"
  assert_eq "template $t: has command" "yes" "$(has '^command: .+' "$f")"
  assert_eq "template $t: reports back through hc send" "yes" "$(has 'hc send' "$f")"
done
assert_eq "template reviewer: is read-only" "yes" "$(has '(?i)read-only' "$TEMPLATES/reviewer.md")"

# --- install manifest -------------------------------------------------------------
assert_eq "manifest links hc templates" "yes" \
  "$(has '^config/herdr-core/templates +\.config/herdr-core/templates$' "$REPO/install.manifest")"

# --- no Orca left behind ------------------------------------------------------------
assert_eq "claude settings: no Orca hooks" "no" "$(has '(?i)orca' "$SETTINGS")"
assert_eq "old Orca snapshot is gone" "no" "$([ -e "$REPO/config/orca" ] && echo yes || echo no)"

# --- always-on rule, mirrored to OpenCode ----------------------------------------
block="$(shared_block "$CLAUDE_MD")"
# The rule is path/command-scoped, so it lives in a lesson (injected when `hc` or
# `herdr-app` runs) and the shared block only points at the lessons directory.
LESSON="$REPO/config/claude/lessons/herdr-workspace.md"
assert_eq "the herdr lesson points to the herdr skill" "yes" \
  "$(rg -q '`herdr`' "$LESSON" && echo yes || echo no)"
assert_eq "the herdr lesson triggers on hc and herdr-app" "yes" \
  "$(rg -q "commands:" "$LESSON" && rg -q 'hc\|herdr-app' "$LESSON" && echo yes || echo no)"
assert_eq "CLAUDE.md: shared block points to the lessons" "yes" \
  "$(printf '%s' "$block" | rg -q 'Herdr' && printf '%s' "$block" | rg -q 'lessons' && echo yes || echo no)"
assert_eq "AGENTS.md mirrors the pointer" "yes" \
  "$(shared_block "$AGENTS_MD" | rg -q 'Herdr' && echo yes || echo no)"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

#!/usr/bin/env bash
# Tests for the hybrid headless-browser policy.
#
# Lightpanda is the default headless browser for automation (Playwright/Puppeteer
# over CDP, E2E flows, scraping, DOM/markdown extraction). It has NO graphical
# rendering engine, so anything that needs pixels — screenshots, visual
# regression, layout/contrast gates, PDF generation — must stay on Chromium.
# These tests pin both halves so neither drifts into "Lightpanda for everything".
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKILL="$REPO/shared/skills/lightpanda-browser/SKILL.md"
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

echo "lightpanda-browser"

# --- skill ---------------------------------------------------------------------
assert_eq "skill: SKILL.md exists" "yes" "$([ -f "$SKILL" ] && echo yes || echo no)"
assert_eq "skill: frontmatter name matches dir" "yes" "$(has '^name: lightpanda-browser$' "$SKILL")"
assert_eq "skill: description is one quoted line" "yes" "$(has '^description: ".+"$' "$SKILL")"
assert_eq "skill: own skills carry no license field" "no" "$(has '^license:' "$SKILL")"
assert_eq "skill: documents CDP connection" "yes" "$(has 'connectOverCDP' "$SKILL")"
assert_eq "skill: disables telemetry" "yes" "$(has 'LIGHTPANDA_DISABLE_TELEMETRY=true' "$SKILL")"
assert_eq "skill: states there is no rendering engine" "yes" "$(has '(?i)no (graphical )?rendering engine' "$SKILL")"
assert_eq "skill: keeps screenshots on Chromium" "yes" "$(has '(?i)screenshot' "$SKILL")"
assert_eq "skill: keeps PDF generation on Chromium" "yes" "$(has '(?i)pdf' "$SKILL")"
assert_eq "skill: never uses Lightpanda png dump as a screenshot" "yes" "$(has 'dump png' "$SKILL")"
assert_eq "skill: agent flows use the Lightpanda MCP server" "yes" "$(has 'lightpanda mcp' "$SKILL")"
assert_eq "skill: installs playwright-core without browser download" "yes" "$(has 'PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1' "$SKILL")"
assert_eq "skill: keeps a single Chromium headless shell" "yes" "$(has '(?i)one .*headless shell' "$SKILL")"
assert_eq "skill: documents the Playwright Test cdpEndpoint fixture" "yes" "$(has 'cdpEndpoint' "$SKILL")"
assert_eq "skill: warns Service Workers (MSW) are unsupported" "yes" "$(has 'serviceWorker' "$SKILL")"
assert_eq "skill: requires a hydration probe before moving specs" "yes" "$(has '(?i)hydration probe' "$SKILL")"
assert_eq "skill: warns Clerk browser sign-up fails" "yes" "$(has '(?i)clerk' "$SKILL")"
assert_eq "skill: forbids broad process kills" "yes" "$(has '(?i)never .*pkill' "$SKILL")"

# --- always-on rule, mirrored to OpenCode ----------------------------------------
block="$(shared_block "$CLAUDE_MD")"
# The rule moved to a lesson injected when a headless browser is about to run;
# the shared block keeps only a pointer to the lessons directory.
LESSON="$REPO/config/claude/lessons/headless-browser.md"
assert_eq "headless-browser lesson names Lightpanda as default" "yes" \
  "$(rg -q 'Lightpanda is the default' "$LESSON" && echo yes || echo no)"
assert_eq "headless-browser lesson keeps Chromium for pixels" "yes" \
  "$(rg -q 'Chromium' "$LESSON" && echo yes || echo no)"
assert_eq "headless-browser lesson points to the skill" "yes" \
  "$(rg -q 'lightpanda-browser' "$LESSON" && echo yes || echo no)"
assert_eq "CLAUDE.md: shared block points to the lessons" "yes" \
  "$(printf '%s' "$block" | rg -q 'headless browser' && printf '%s' "$block" | rg -q 'lessons' && echo yes || echo no)"
assert_eq "AGENTS.md mirrors the pointer" "yes" \
  "$(shared_block "$AGENTS_MD" | rg -q 'headless browser' && echo yes || echo no)"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

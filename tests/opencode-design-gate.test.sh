#!/usr/bin/env bash
# Tests for the OpenCode port of the design-pipeline gate.
#
# Why a second implementation exists
# ----------------------------------
# The Claude Code gate is a PreToolUse shell hook (config/claude/hooks/
# design-pipeline-gate.sh). OpenCode has no JSON hook system, so the shell hook
# cannot be reused — only its contract can, and the contract is now a MARKER:
#
#   `design-ack <surface>` writes ${XDG_CACHE_HOME:-~/.cache}/design-gate/<sha1
#   of project root>; a UI edit is allowed while that marker is younger than
#   DESIGN_ACK_TTL seconds (default 4 h) and .agents/DESIGN.md still hashes the
#   same.
#
# This plugin reads the same file with the same rules (no in-memory checklist
# matching, no event stream), so one `design-ack` opens both agents, and a
# sub-agent or a post-compaction turn needs nothing special.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLUGIN="$REPO/config/opencode/plugin/design-gate.ts"
ACK="$REPO/shared/skills/design-pipeline/scripts/design-ack"

pass=0
fail=0
ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "opencode-design-gate"

assert_eq "the plugin exists" "yes" "$([ -f "$PLUGIN" ] && echo yes || echo no)"

if ! command -v bun >/dev/null 2>&1; then
  echo "  bun not installed — logic assertions skipped"
  printf '\n%d passed, %d failed\n' "$pass" "$fail"
  [ "$fail" -eq 0 ]; exit $?
fi
[ -f "$PLUGIN" ] || { printf '\n%d passed, %d failed\n' "$pass" "$((fail))"; exit 1; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
export XDG_CACHE_HOME="$TMP/cache"
unset DESIGN_PIPELINE_OFF DESIGN_ACK_TTL

mkdir -p "$REPO/tests/fixtures"
cat > "$REPO/tests/fixtures/opencode-gate-probe.ts" <<'PROBE'
// Exercises the gate's pure predicate plus the deny path, so the bash suite can
// assert on them without standing up an OpenCode session.
//   probe ui <path>                     -> isUISurface
//   probe edit <path> <dir> [session]   -> "deny" | "allow" for an edit call
import { isUISurface, DesignGate } from "../../config/opencode/plugin/design-gate.ts"

const [, , mode, file, dir, session] = process.argv

if (mode === "ui") {
  console.log(String(isUISurface(file)))
} else {
  const hooks: any = await (DesignGate as any)({ directory: dir })
  try {
    await hooks["tool.execute.before"]({ tool: "edit", sessionID: session ?? "s1", callID: "c1" },
      { args: { filePath: file } })
    console.log("allow")
  } catch (e: any) {
    console.log("deny " + String(e?.message ?? e).split("\n").length)
  }
}
PROBE

ui() { bun run "$REPO/tests/fixtures/opencode-gate-probe.ts" ui "$1" 2>/dev/null; }
edit() { bun run "$REPO/tests/fixtures/opencode-gate-probe.ts" edit "$1" "$2" "${3:-}" 2>/dev/null; }
ack() { (cd "$1" && "$ACK" "${2:-component}" >/dev/null 2>&1); }
marker_for() {
  local o; o="$(printf '%s' "$(cd "$1" && pwd -P)" | shasum -a 1)"
  printf '%s/design-gate/%s' "$XDG_CACHE_HOME" "${o%% *}"
}

assert_eq "a .tsx is a UI surface"  "true"  "$(ui src/Button.tsx)"
assert_eq "a .css is a UI surface"  "true"  "$(ui app/main.css)"
assert_eq "a .go file is not"       "false" "$(ui cmd/main.go)"
assert_eq "a component test is not" "false" "$(ui src/Button.test.tsx)"

P="$TMP/proj"; mkdir -p "$P/src" "$P/.agents"
printf 'tokens v1\n' > "$P/.agents/DESIGN.md"

assert_eq "a UI write with no marker is denied, in one line" "deny 1" "$(edit "$P/src/Button.tsx" "$P")"

ack "$P"
assert_eq "a UI write with a fresh marker is allowed"   "allow" "$(edit "$P/src/Button.tsx" "$P")"
assert_eq "the session id is irrelevant (sub-agents)"   "allow" "$(edit "$P/src/Button.tsx" "$P" other-session)"

sd '^epoch=.*$' "epoch=$(( $(date +%s) - 18000 ))" "$(marker_for "$P")"
assert_eq "a marker older than 4 hours is denied"       "deny 1" "$(edit "$P/src/Button.tsx" "$P")"
assert_eq "DESIGN_ACK_TTL lengthens the window" "allow" \
  "$(DESIGN_ACK_TTL=86400 edit "$P/src/Button.tsx" "$P")"

ack "$P"
printf 'tokens v2\n' > "$P/.agents/DESIGN.md"
assert_eq "DESIGN.md changed after the ack is denied"   "deny 1" "$(edit "$P/src/Button.tsx" "$P")"

assert_eq "a non-UI write needs no marker"      "allow" "$(edit "$P/main.go" "$P")"
assert_eq "a component test needs no marker"    "allow" "$(edit "$P/src/Button.test.tsx" "$P")"
assert_eq "DESIGN_PIPELINE_OFF bypasses"        "allow" \
  "$(DESIGN_PIPELINE_OFF=1 edit "$P/src/Button.tsx" "$P")"

# Same marker file as the shell hook: the project root is the git toplevel found
# by walking up from the file, so a nested file resolves to the repo's marker.
G="$TMP/repo"; mkdir -p "$G/src/deep"; git -C "$G" init -q
ack "$G"
assert_eq "a nested file resolves to the git toplevel marker" "allow" "$(edit "$G/src/deep/Card.tsx" "$G/src/deep")"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

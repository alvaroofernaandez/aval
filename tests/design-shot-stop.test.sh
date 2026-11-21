#!/usr/bin/env bash
# Tests for the Stop hook (`aval hook design-shot-stop`) that asks for a render
# before a UI turn ends. The Go suites in internal/designhook cover the same cases.
#
# Measured over 60 days: about one user message in five was a screenshot saying
# a "fixed" visual bug was not fixed. Verification was done by reading code, not
# by rendering. The gate hook stamps ${cache}/design-gate/ui-edited.<session_id>
# (content: project root) whenever it allows a UI edit; this Stop hook blocks
# ONCE per stamp when .design/shots of that project holds nothing newer, asking
# for shot.mjs or an honest "not visually verified".
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
. "$REPO/tests/lib/aval-bin.sh"
SETTINGS="$REPO/config/claude/settings.json"

pass=0
fail=0
ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "design-shot-stop"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
AVAL="$(build_aval "$TMP")"
export XDG_CACHE_HOME="$TMP/cache"
unset DESIGN_PIPELINE_OFF
GATE="$XDG_CACHE_HOME/design-gate"
mkdir -p "$GATE"

assert_eq "aval builds" "yes" "$([ -x "$AVAL" ] && echo yes || echo no)"
if [ ! -x "$AVAL" ]; then
  printf '\n%d passed, %d failed\n' "$pass" "$((fail + 1))"; exit 1
fi

OLD=200001010000
PROJ="$TMP/proj"; mkdir -p "$PROJ/.design/shots"

stop() { # $1 session  [$2 stop_hook_active]
  jq -cn --arg sid "$1" --argjson active "${2:-false}" \
    '{session_id:$sid, hook_event_name:"Stop", stop_hook_active:$active, cwd:"/x"}' | "$AVAL" hook design-shot-stop 2>/dev/null
}
stamp() { printf '%s' "$PROJ" >"$GATE/ui-edited.$1"; touch -t "${2:-$OLD}" "$GATE/ui-edited.$1"; }
decision() { [ -z "$1" ] && { printf none; return; }; printf '%s' "$1" | jq -r '.decision // "none"'; }

assert_eq "no UI edit this session: silent" "none" "$(decision "$(stop s-none)")"

stamp s1
OUT="$(stop s1)"
assert_eq "UI edited and no shot: block" "block" "$(decision "$OUT")"
REASON="$(printf '%s' "$OUT" | jq -r '.reason')"
assert_eq "the reason names shot.mjs" "yes" "$(printf '%s' "$REASON" | rg -q 'shot\.mjs' && echo yes || echo no)"
assert_eq "the reason offers the honest alternative" "yes" \
  "$(printf '%s' "$REASON" | rg -q 'not visually verified' && echo yes || echo no)"

assert_eq "it blocks only ONCE per stamp" "none" "$(decision "$(stop s1)")"

# A later UI edit re-stamps; the block is allowed again, once.
touch "$GATE/ui-edited.s1"
touch -t "$OLD" "$GATE/ui-edited.s1.blocked"
assert_eq "a newer stamp blocks again" "block" "$(decision "$(stop s1)")"
assert_eq "...and only once again" "none" "$(decision "$(stop s1)")"

# A shot newer than the stamp satisfies it.
stamp s2
touch "$PROJ/.design/shots/home-390.png"
assert_eq "a render newer than the stamp: silent" "none" "$(decision "$(stop s2)")"

# A shot older than the stamp does not.
rm -f "$PROJ/.design/shots/"*
touch -t "$OLD" "$PROJ/.design/shots/old-390.png"
stamp s3 200001020000
assert_eq "a render older than the stamp still blocks" "block" "$(decision "$(stop s3)")"

stamp s4
assert_eq "stop_hook_active never blocks (no loops)" "none" "$(decision "$(stop s4 true)")"

stamp s5
assert_eq "DESIGN_PIPELINE_OFF is silent" "none" "$(decision "$(DESIGN_PIPELINE_OFF=1 stop s5)")"

for bad in '' 'garbage' '{"hook_event_name":"Stop"}'; do
  out="$(printf '%s' "$bad" | "$AVAL" hook design-shot-stop 2>/dev/null; echo "rc=$?")"
  assert_eq "malformed stdin is silent and exits 0: ${bad:-<empty>}" "rc=0" "$out"
done

# The common case (no UI edit this session) needs nothing but the binary.
out="$(printf '%s' '{"session_id":"s-none","stop_hook_active":false}' | PATH=/nonexistent "$AVAL" hook design-shot-stop 2>&1; echo "rc=$?")"
assert_eq "the no-edit path runs with an empty PATH" "rc=0" "$out"

# --- wiring ------------------------------------------------------------------
assert_eq "settings.json runs the stop hook on Stop" "yes" \
  "$(jq -e '.hooks.Stop[]?.hooks[]?.command | select(test("hook design-shot-stop"))' "$SETTINGS" >/dev/null 2>&1 && echo yes || echo no)"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

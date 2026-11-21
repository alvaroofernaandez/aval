#!/usr/bin/env bash
# Tests for the OmniWM tiling window manager config.
#
# OmniWM's defaults put every action on Option. On a Spanish ISO keyboard
# Option+1/2/3 type | @ # and Option+arrows move by word, so the defaults would
# swallow characters needed to write code. Caps Lock becomes the Hyper key
# (Control+Option+Command, without Shift so Shift still tells "move window"
# apart from "switch workspace"), and no binding may start with plain Option.
#
# OmniWM rejects the WHOLE file on a missing key, an unknown enum or a
# duplicate hotkey id, so the file is the app's own export, edited in place.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONF="$REPO/config/omniwm/settings.toml"

pass=0
fail=0
ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "omniwm"

assert_eq "the OmniWM config is versioned" "yes" "$([ -f "$CONF" ] && echo yes || echo no)"
assert_eq "the manifest links it into ~/.config/omniwm" "yes" \
  "$(rg -q '^config/omniwm/settings\.toml\s+\.config/omniwm/settings\.toml$' "$REPO/install.manifest" && echo yes || echo no)"
assert_eq "schema version is 5" "yes" "$(rg -q '^schemaVersion = 5$' "$CONF" 2>/dev/null && echo yes || echo no)"
assert_eq "Caps Lock triggers Hyper" "yes" \
  "$(rg -q '^systemHyperTrigger = "Caps Lock"$' "$CONF" 2>/dev/null && echo yes || echo no)"
assert_eq "Hyper is Control+Option+Command, leaving Shift free" "yes" \
  "$(rg -q '^hyperKeyModifiers = "Control\+Option\+Command"$' "$CONF" 2>/dev/null && echo yes || echo no)"
assert_eq "no binding starts with plain Option" "0" \
  "$(rg -c '^binding = "(Left |Right )?Option\+' "$CONF" 2>/dev/null || echo 0)"
assert_eq "workspace 1 is on Hyper+1" "yes" \
  "$(rg -U -q 'binding = "Hyper\+1"\nid = "switchWorkspace\.0"' "$CONF" 2>/dev/null && echo yes || echo no)"
assert_eq "moving a window to workspace 1 is Hyper+Shift+1" "yes" \
  "$(rg -U -q 'binding = "Hyper\+Shift\+1"\nid = "moveToWorkspace\.0"' "$CONF" 2>/dev/null && echo yes || echo no)"

dups="$(rg -o '^binding = "(.*)"' -r '$1' "$CONF" 2>/dev/null | rg -v '^Unassigned$' | sort | uniq -d)"
assert_eq "no two actions share a binding" "" "$dups"
dup_ids="$(rg -o '^id = "(.*)"' -r '$1' "$CONF" 2>/dev/null | sort | uniq -d | rg -v '^[0-9A-F-]{36}$')"
assert_eq "no hotkey id appears twice" "" "$dup_ids"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

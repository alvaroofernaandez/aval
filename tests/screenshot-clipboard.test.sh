#!/usr/bin/env bash
# Every screenshot lands on the clipboard as well as on disk.
#
# 29% of the prompts sent to Claude Code carry a pasted image, almost all of
# them screenshots. macOS saves a screenshot to a file OR copies it, never
# both, so each one cost a trip to Finder. A LaunchAgent watches the screenshot
# folder and copies the newest image, keeping the file.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
. "$REPO/tests/lib/aval-bin.sh"

pass=0
fail=0
ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "screenshot-clipboard"

T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
AVAL="$(build_aval "$T")"
mkdir -p "$T/shots" "$T/state"
LOG="$T/clip.log"
# A fake clipboard: records which file it was asked to copy.
printf '#!/bin/sh\nprintf "%%s\\n" "$1" >> "%s"\n' "$LOG" > "$T/fakeclip"; chmod +x "$T/fakeclip"
run() { CLIP_CMD="$T/fakeclip" XDG_STATE_HOME="$T/state" SETTLE=0 "$AVAL" macos screenshot-clipboard copy "$T/shots"; }

assert_eq "the Go binary builds" "yes" "$([ -n "$AVAL" ] && echo yes || echo no)"

if [ -n "$AVAL" ]; then
  printf 'old' > "$T/shots/Captura 1.png"; touch -t 202601010000 "$T/shots/Captura 1.png"
  printf 'new' > "$T/shots/Captura 2.png"
  printf 'txt' > "$T/shots/notes.txt"
  run
  assert_eq "it copies the newest screenshot" "$T/shots/Captura 2.png" "$(tail -1 "$LOG" 2>/dev/null)"

  run
  assert_eq "it does not copy the same screenshot twice" "1" "$(wc -l < "$LOG" | tr -d ' ')"

  printf 'tmp' > "$T/shots/.Captura 3.png"
  run
  assert_eq "it ignores the hidden file macOS writes before renaming" "1" "$(wc -l < "$LOG" | tr -d ' ')"

  sleep 1; printf 'jpg' > "$T/shots/Captura 4.jpg"
  run
  assert_eq "it copies jpg screenshots too" "$T/shots/Captura 4.jpg" "$(tail -1 "$LOG")"
fi

if [ -n "$AVAL" ]; then
  HOME="$T/home" SKIP_LAUNCHCTL=1 SKIP_DEFAULTS=1 "$AVAL" macos screenshot-clipboard install >/dev/null 2>&1
  PLIST="$T/home/Library/LaunchAgents/dev.aval.screenshot-clipboard.plist"
  assert_eq "it writes the LaunchAgent" "yes" "$([ -f "$PLIST" ] && echo yes || echo no)"
  assert_eq "the agent watches the screenshot folder" "yes" \
    "$(rg -q "<string>$T/home/Pictures/Screenshots</string>" "$PLIST" 2>/dev/null && rg -q 'WatchPaths' "$PLIST" && echo yes || echo no)"
  assert_eq "the agent runs the installed aval binary" "yes" \
    "$(rg -q "<string>$T/home/.local/bin/aval</string>" "$PLIST" 2>/dev/null && rg -q '<string>copy</string>' "$PLIST" && echo yes || echo no)"
  assert_eq "it creates the screenshot folder" "yes" "$([ -d "$T/home/Pictures/Screenshots" ] && echo yes || echo no)"
  if command -v plutil >/dev/null 2>&1; then
    assert_eq "the plist is valid" "0" "$(plutil -lint "$PLIST" >/dev/null 2>&1; echo $?)"
  fi
fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

#!/usr/bin/env bash
# Shell helpers born from the habits audit (14,391 atuin commands, 2026-11-21):
# `clear` is 30% of everything typed, with ~150 typos; `docker-compose up
# --build` ~260 times; `sleep 600` 157 times to wait on a job; `git pull` 499
# plus `git status` 261; the same 50-char react-doctor command 108 times.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HELPERS="$REPO/home/zsh/helpers.zsh"

pass=0
fail=0
ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "shell-helpers"

assert_eq "helpers.zsh exists" "yes" "$([ -f "$HELPERS" ] && echo yes || echo no)"
assert_eq "zshrc sources the helpers" "yes" \
  "$(rg -q 'zsh/helpers\.zsh' "$REPO/home/zshrc" && echo yes || echo no)"

T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
mkdir -p "$T/bin"
# Stubs record how they were called instead of starting containers.
for c in make docker npm git npx; do
  printf '#!/bin/sh\necho "%s $*" >> "%s/calls"\n' "$c" "$T" > "$T/bin/$c"; chmod +x "$T/bin/$c"
done
z() { (cd "$1" && PATH="$T/bin:/usr/bin:/bin" zsh -f -c "source '$HELPERS'; $2" 2>&1); }

if [ -f "$HELPERS" ]; then
  mkdir -p "$T/mk" "$T/dc" "$T/np"
  printf 'up-build:\n\techo\n' > "$T/mk/Makefile"
  printf 'services: {}\n' > "$T/dc/compose.yaml"
  printf '{"scripts":{"dev:stack:up":"x","dev:stack:down":"y"}}\n' > "$T/np/package.json"

  : > "$T/calls"; z "$T/mk" up >/dev/null
  assert_eq "up runs make up-build when the Makefile has it" "make up-build" "$(tail -1 "$T/calls")"
  : > "$T/calls"; z "$T/dc" up >/dev/null
  assert_eq "up runs docker compose up --build -d for a compose project" "docker compose up --build -d" "$(tail -1 "$T/calls")"
  : > "$T/calls"; z "$T/np" up >/dev/null
  assert_eq "up runs the npm stack script otherwise" "npm run dev:stack:up" "$(tail -1 "$T/calls")"
  : > "$T/calls"; z "$T/dc" down >/dev/null
  assert_eq "down stops compose without deleting volumes" "docker compose down" "$(tail -1 "$T/calls")"
  : > "$T/calls"; z "$T/dc" 'down -v </dev/null' >/dev/null
  assert_eq "down -v asks first and does nothing without a yes" "" "$(cat "$T/calls")"
  : > "$T/calls"; z "$T/dc" 'print y | down -v' >/dev/null
  assert_eq "down -v deletes volumes after a yes" "docker compose down -v" "$(tail -1 "$T/calls")"

  start=$(date +%s)
  z "$T" "WAITFOR_INTERVAL=1 WAITFOR_QUIET=1 waitfor '[[ -f $T/flag ]]'" >/dev/null &
  sleep 1; touch "$T/flag"; wait $!
  assert_eq "waitfor returns once the condition holds" "1" "$(( $(date +%s) - start < 10 ))"

  : > "$T/calls"; z "$T" gp >/dev/null
  assert_eq "gp pulls with submodules, then shows a short status" \
    "git pull --recurse-submodules|git status -sb" "$(paste -sd'|' "$T/calls")"
  : > "$T/calls"; z "$T" rd >/dev/null
  assert_eq "rd runs react-doctor on the diff" "npx -y react-doctor@latest . --verbose --diff" "$(tail -1 "$T/calls")"

  for a in cñear ckear clea clar exir exirt; do
    assert_eq "typo alias $a exists" "yes" "$(z "$T" "alias -- $a >/dev/null && echo yes || echo no")"
  done
fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

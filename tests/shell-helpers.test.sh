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
  # --- wt: worktree per task, its own .env and port ------------------------
  R="$T/repo"; mkdir -p "$R"; git -C "$R" init -q -b main
  printf '.env*\n.worktrees/\n' > "$R/.gitignore"; printf 'SECRET=1\n' > "$R/.env"
  git -C "$R" add .gitignore && git -C "$R" -c user.name=t -c user.email=t@t commit -qm init
  wtz() { (cd "$R" && PATH="/usr/bin:/bin:/opt/homebrew/bin" zsh -f -c "source '$HELPERS'; $1" 2>&1); }
  wtz 'wt feat-a' >/dev/null
  assert_eq "wt creates .worktrees/<name> on its own branch" "feat-a" \
    "$(git -C "$R/.worktrees/feat-a" branch --show-current 2>/dev/null)"
  assert_eq "wt copies the ignored .env files" "SECRET=1" "$(cat "$R/.worktrees/feat-a/.env" 2>/dev/null)"
  port="$(rg -o '^PORT=(\d+)$' -r '$1' "$R/.worktrees/feat-a/.env.local" 2>/dev/null)"
  assert_eq "wt gives the worktree its own port (not 3000)" "yes" \
    "$([ -n "$port" ] && [ "$port" != 3000 ] && echo yes || echo no)"
  wtz 'wt feat-a' >/dev/null
  assert_eq "the port is stable for the same name" "$port" \
    "$(rg -o '^PORT=(\d+)$' -r '$1' "$R/.worktrees/feat-a/.env.local" | tail -1)"
  assert_eq "wt ls lists it" "yes" "$(wtz 'wt ls' | rg -q 'feat-a' && echo yes || echo no)"
  wtz 'wt rm feat-a' >/dev/null
  assert_eq "wt rm removes the worktree" "no" "$([ -d "$R/.worktrees/feat-a" ] && echo yes || echo no)"

  # --- forrepos: one command across every repo -----------------------------
  F="$T/projects"; mkdir -p "$F/a" "$F/b" "$F/group/c" "$F/not-a-repo"
  for r in a b group/c; do git -C "$F/$r" init -q; done
  out="$(cd "$T" && PATH="/usr/bin:/bin:/opt/homebrew/bin" FORREPOS_ROOT="$F" zsh -f -c "source '$HELPERS'; forrepos 'pwd'" 2>&1)"
  assert_eq "forrepos runs in every repo, nested ones included" "3" \
    "$(printf '%s\n' "$out" | rg -c '/(a|b|group/c)$')"
  assert_eq "forrepos prefixes output with the repo name" "yes" \
    "$(printf '%s\n' "$out" | rg -q '^group/c +' && echo yes || echo no)"
  out="$(cd "$T" && PATH="/usr/bin:/bin:/opt/homebrew/bin" FORREPOS_ROOT="$F" zsh -f -c "source '$HELPERS'; forrepos '[ \$(basename \$PWD) != a ]'" 2>&1)"; rc=$?
  assert_eq "a failing repo does not stop the others, and the exit is non-zero" "1" "$rc"
  assert_eq "...with a summary naming it" "yes" "$(printf '%s\n' "$out" | rg -q 'failed.*\ba\b' && echo yes || echo no)"
fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

#!/usr/bin/env bash
# The pre-push hook is the last gate before anything reaches the public repo.
#
# Twice on 2026-11-21 employer material reached a commit: once a subagent
# committed an uncommitted file, once a task log named work repositories and
# was pushed without running the suite. Both times the check existed and was
# simply not run. A hook makes it unskippable by default.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOOK="$REPO/.githooks/pre-push"

. "$REPO/tests/lib/aval-bin.sh"

pass=0
fail=0

ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "pre-push"

assert_eq "the pre-push hook is versioned and executable" "yes" \
  "$([ -x "$HOOK" ] && echo yes || echo no)"
assert_eq "this clone uses the versioned hooks" ".githooks" \
  "$(git -C "$REPO" config --get core.hooksPath)"
assert_eq "the hook is a thin exec of the Go binary" "exec \"\$a\" hook pre-push \"\$@\"" \
  "$(tail -1 "$HOOK")"
# Same resolution as the Claude hooks: ~/.local/bin/aval first, PATH second,
# so a push works even when ~/.local/bin is not on PATH (GUI git clients).
assert_eq "the hook prefers ~/.local/bin/aval" "yes" \
  "$(rg -qF 'a="$HOME/.local/bin/aval"; [ -x "$a" ] || a=aval' "$HOOK" && echo yes || echo no)"
assert_eq "the hook carries no logic of its own" "3" "$(wc -l < "$HOOK" | tr -d ' ')"

# Behaviour: in a scratch clone, a commit carrying employer material is refused.
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"
if [ -z "$(build_aval "$work/bin")" ]; then ko "the Go binary builds" "a binary" "build failed"; fi
export PATH="$work/bin:$PATH"
# Real drift is the machine's business: default to a binary that reports none.
printf '#!/bin/sh\nexit 0\n' > "$work/aval-ok"; chmod +x "$work/aval-ok"
export AVAL_BIN="$work/aval-ok"
git clone -q --no-hardlinks "$REPO" "$work/clone" 2>/dev/null
# Test the working copy of the hook, not whatever is committed.
mkdir -p "$work/clone/.githooks"
cp "$HOOK" "$work/clone/.githooks/pre-push"
git -C "$work/clone" config core.hooksPath .githooks
git init -q --bare "$work/remote.git"
git -C "$work/clone" remote add scratch "$work/remote.git"
# Positive control: a clean commit must go through, or a hook that always
# fails would pass the refusal assertion below.
printf 'clean note\n' > "$work/clone/clean.md"
git -C "$work/clone" add clean.md
git -C "$work/clone" -c user.name=t -c user.email=t@t commit -qm "test: clean"
git -C "$work/clone" push -q scratch HEAD:main >/dev/null 2>&1
assert_eq "a clean push goes through" "0" "$?"

h="haga"
printf 'notes about %s\n' "${h}link-projects" > "$work/clone/leak.md"
git -C "$work/clone" add leak.md
git -C "$work/clone" -c user.name=t -c user.email=t@t commit -qm "test: leak"
leak_err="$(git -C "$work/clone" push -q scratch HEAD:main 2>&1 >/dev/null)"
rc=$?
assert_eq "a push carrying employer material is refused" "1" \
  "$([ "$rc" -ne 0 ] && echo 1 || echo 0)"
assert_eq "...for the right reason" "yes" \
  "$(printf '%s' "$leak_err" | rg -q 'employer material matching' && echo yes || echo no)"

# aval sync --check: a WARNING by default (another machine's ~/.claude must not
# block a push), blocking only with AVAL_PREPUSH_STRICT=1, silent when the
# binary is absent. A stub stands in for the real binary.
git -C "$work/clone" reset -q --hard HEAD~1   # drop the refused leak commit
# reset restores the committed hook file; put the working copy back
cp "$HOOK" "$work/clone/.githooks/pre-push"
stub="$work/aval-drift"
printf '#!/bin/sh\necho "drift  settings  hooks"\nexit 1\n' > "$stub"
chmod +x "$stub"
okstub="$work/aval-ok"
printf '#!/bin/sh\nexit 0\n' > "$okstub"
chmod +x "$okstub"

printf 'second clean note\n' > "$work/clone/clean2.md"
git -C "$work/clone" add clean2.md
git -C "$work/clone" -c user.name=t -c user.email=t@t commit -qm "test: clean 2"

err="$(AVAL_BIN="$stub" git -C "$work/clone" push scratch HEAD:main 2>&1 >/dev/null)"
assert_eq "sync drift only warns: the push goes through" "0" "$?"
assert_eq "sync drift is reported as a warning" "yes" \
  "$(printf '%s' "$err" | rg -q 'warning.*aval sync --check' && echo yes || echo no)"

printf 'third clean note\n' > "$work/clone/clean3.md"
git -C "$work/clone" add clean3.md
git -C "$work/clone" -c user.name=t -c user.email=t@t commit -qm "test: clean 3"
AVAL_PREPUSH_STRICT=1 AVAL_BIN="$stub" git -C "$work/clone" push -q scratch HEAD:main >/dev/null 2>&1
assert_eq "AVAL_PREPUSH_STRICT=1 makes sync drift block the push" "1" \
  "$([ "$?" -ne 0 ] && echo 1 || echo 0)"

AVAL_PREPUSH_STRICT=1 AVAL_BIN="$okstub" git -C "$work/clone" push -q scratch HEAD:main >/dev/null 2>&1
assert_eq "strict mode with no drift pushes" "0" "$?"

printf 'fourth clean note\n' > "$work/clone/clean4.md"
git -C "$work/clone" add clean4.md
git -C "$work/clone" -c user.name=t -c user.email=t@t commit -qm "test: clean 4"
AVAL_PREPUSH_STRICT=1 AVAL_BIN="$work/does-not-exist" git -C "$work/clone" push -q scratch HEAD:main >/dev/null 2>&1
assert_eq "no aval binary: nothing to check, even in strict mode" "0" "$?"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

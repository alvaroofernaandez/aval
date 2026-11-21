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
assert_eq "the hook uses the shared proprietary patterns" "yes" \
  "$(rg -q 'proprietary-patterns\.sh' "$HOOK" 2>/dev/null && echo yes || echo no)"
assert_eq "the hook scans outgoing commits with gitleaks" "yes" \
  "$(rg -q 'gitleaks' "$HOOK" 2>/dev/null && echo yes || echo no)"

# Behaviour: in a scratch clone, a commit carrying employer material is refused.
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
git clone -q --no-hardlinks "$REPO" "$work/clone" 2>/dev/null
# Test the working copy of the hook and patterns, not whatever is committed.
mkdir -p "$work/clone/.githooks" "$work/clone/tests/lib"
cp "$HOOK" "$work/clone/.githooks/pre-push"
cp "$REPO/tests/lib/proprietary-patterns.sh" "$work/clone/tests/lib/"
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
git -C "$work/clone" push -q scratch HEAD:main >/dev/null 2>&1
assert_eq "a push carrying employer material is refused" "1" \
  "$([ "$?" -ne 0 ] && echo 1 || echo 0)"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

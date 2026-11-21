#!/usr/bin/env bash
# Tests for the Kiri-only Docker skills. The cleanup command itself is covered
# by Go tests (internal/dockerclean).
#
# Kiri replaced OrbStack (which replaced Colima) on 2026-11-17: no skill may
# instruct starting either of them.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKILLS="$REPO/shared/skills"

pass=0
fail=0
STUB=""
cleanup() { [ -n "$STUB" ] && rm -rf "$STUB"; }
trap cleanup EXIT

ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }
assert_contains() { case "$3" in *"$2"*) ok "$1" ;; *) ko "$1" "text containing: $2" "$3" ;; esac; }
assert_not_contains() { case "$3" in *"$2"*) ko "$1" "no: $2" "$3" ;; *) ok "$1" ;; esac; }

echo "docker-runtime-skills"

# --- every skill exists with valid frontmatter -------------------------------
for s in kiri-docker docker-space-optimizer kiri-health docker-build-optimized; do
  f="$SKILLS/$s/SKILL.md"
  assert_eq "$s: SKILL.md exists" "yes" "$([ -f "$f" ] && echo yes || echo no)"
  assert_eq "$s: frontmatter name matches dir" "yes" \
    "$(rg -q "^name: $s\$" "$f" 2>/dev/null && echo yes || echo no)"
  assert_eq "$s: description is one quoted line" "yes" \
    "$(rg -q '^description: ".+"$' "$f" 2>/dev/null && echo yes || echo no)"
  # Colima/OrbStack may only be named as something to detect or refuse, never as a runtime to use.
  assert_eq "$s: never instructs to start/install Colima or OrbStack" "no" \
    "$(rg -qi '(colima start|brew install colima|--context colima|brew install --cask orbstack|orb start|orbctl start|--context orbstack|context use orbstack)' "$f" 2>/dev/null && echo yes || echo no)"
  assert_eq "$s: no orbctl invocation" "no" \
    "$(rg -q 'orbctl' "$f" 2>/dev/null && echo yes || echo no)"
done

assert_eq "old orbstack-docker skill is gone" "no" "$([ -e "$SKILLS/orbstack-docker" ] && echo yes || echo no)"
assert_eq "kiri-health detects Docker Desktop's socket helper hijacking /var/run/docker.sock" "yes" \
  "$(rg -q 'com\.docker\.socket' "$SKILLS/kiri-health/SKILL.md" 2>/dev/null && echo yes || echo no)"
assert_eq "old orbstack-health skill is gone" "no" "$([ -e "$SKILLS/orbstack-health" ] && echo yes || echo no)"
assert_eq "kiri-docker documents BUILDX_BUILDER=multiarch" "yes" \
  "$(rg -q 'BUILDX_BUILDER=multiarch' "$SKILLS/kiri-docker/SKILL.md" 2>/dev/null && echo yes || echo no)"
assert_eq "docker-build-optimized verifies the image architecture" "yes" \
  "$(rg -q "image inspect -f '\{\{.Architecture\}\}'" "$SKILLS/docker-build-optimized/SKILL.md" 2>/dev/null && echo yes || echo no)"
assert_eq "docker-space-optimizer points to aval docker cleanup" "yes" \
  "$(rg -q 'aval docker cleanup' "$SKILLS/docker-space-optimizer/SKILL.md" 2>/dev/null && echo yes || echo no)"
assert_eq "kiri-docker points to aval kiri up" "yes" \
  "$(rg -q 'aval kiri up' "$SKILLS/kiri-docker/SKILL.md" 2>/dev/null && echo yes || echo no)"
assert_eq "no shell scripts left in the docker skills" "" \
  "$(fd -e sh . "$SKILLS/kiri-docker" "$SKILLS/docker-space-optimizer" 2>/dev/null)"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

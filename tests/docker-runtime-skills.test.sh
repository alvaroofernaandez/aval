#!/usr/bin/env bash
# Tests for the Kiri-only Docker skills and the space cleanup script.
#
# Kiri replaced OrbStack (which replaced Colima) on 2026-11-17. The cleanup
# script must only ever run the safe prunes, and only against the `kiri`
# context: a prune sent to the wrong daemon deletes data from a runtime the user
# did not mean to touch. docker/df/du are stubbed on PATH so nothing real is pruned.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKILLS="$REPO/shared/skills"
SCRIPT="$SKILLS/docker-space-optimizer/scripts/cleanup.sh"

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
assert_eq "old orbstack-health skill is gone" "no" "$([ -e "$SKILLS/orbstack-health" ] && echo yes || echo no)"
assert_eq "kiri-docker documents BUILDX_BUILDER=multiarch" "yes" \
  "$(rg -q 'BUILDX_BUILDER=multiarch' "$SKILLS/kiri-docker/SKILL.md" 2>/dev/null && echo yes || echo no)"
assert_eq "docker-build-optimized verifies the image architecture" "yes" \
  "$(rg -q "image inspect -f '\{\{.Architecture\}\}'" "$SKILLS/docker-build-optimized/SKILL.md" 2>/dev/null && echo yes || echo no)"
assert_eq "cleanup.sh never calls orbctl" "no" \
  "$(rg -q 'orbctl' "$SCRIPT" 2>/dev/null && echo yes || echo no)"

# --- cleanup.sh ---------------------------------------------------------------
assert_eq "cleanup.sh exists" "yes" "$([ -f "$SCRIPT" ] && echo yes || echo no)"
assert_eq "cleanup.sh is executable" "yes" "$([ -x "$SCRIPT" ] && echo yes || echo no)"
assert_eq "cleanup.sh uses strict mode" "yes" \
  "$(rg -q '^set -euo pipefail' "$SCRIPT" 2>/dev/null && echo yes || echo no)"

STUB="$(mktemp -d)"
LOG="$STUB/calls.log"
mkstub() { printf '#!/usr/bin/env bash\necho "%s $*" >>"%s"\n%s\n' "$1" "$LOG" "$2" >"$STUB/$1"; chmod +x "$STUB/$1"; }
mkstub docker 'if [ "$1 $2" = "context show" ]; then echo "${FAKE_CONTEXT:-kiri}"; fi'
mkstub df 'echo "Filesystem Size Used Avail"; echo "/dev/disk3 460Gi 12Gi 100Gi"'
mkstub du 'echo "2.9G	$*"'

run() { : >"$LOG"; env PATH="$STUB:/usr/bin:/bin" FAKE_CONTEXT="$1" bash "$SCRIPT" 2>&1; echo "exit=$?"; }

if [ -f "$SCRIPT" ]; then
  out="$(run kiri)"; calls="$(cat "$LOG")"
  assert_contains "runs container prune" "docker container prune -f" "$calls"
  assert_contains "runs image prune (dangling only)" "docker image prune -f" "$calls"
  assert_contains "runs builder prune" "docker builder prune -af" "$calls"
  assert_not_contains "never runs system prune" "system prune" "$calls"
  assert_not_contains "never prunes volumes" "volume" "$calls"
  assert_not_contains "never prunes all images" "image prune -a" "$calls"
  assert_contains "prints a BEFORE section" "BEFORE" "$out"
  assert_contains "prints an AFTER section" "AFTER" "$out"
  assert_contains "prunes the multiarch builder cache" "docker buildx prune --builder multiarch -af" "$calls"
  assert_contains "measures the Kiri data dir" "Application Support/Kiri" "$out"
  assert_not_contains "does not claim the disk auto-shrinks" "auto-shrink" "$out"
  assert_contains "succeeds on kiri" "exit=0" "$out"

  for other in orbstack colima-rosetta desktop-linux; do
    out="$(run "$other")"; calls="$(cat "$LOG")"
    assert_not_contains "refuses to prune on context $other" "prune" "$calls"
    assert_not_contains "exits non-zero on context $other" "exit=0" "$out"
  done
fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

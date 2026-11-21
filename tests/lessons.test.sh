#!/usr/bin/env bash
# Tests for the lessons hook wiring and the lessons themselves.
#
# Why this exists
# ---------------
# Rules that only matter near certain files (Docker, UI, commits) were riding in
# the always-on instructions: ~6.6K chars paid on every turn of every session.
# `aval hook lessons` injects a short lesson only when a PreToolUse call touches
# a matching path or command, so the same rule costs nothing the rest of the
# time. These tests pin the three things that can silently rot: the wiring in
# settings.json, the shape of each lesson, and the latency of a hook that runs
# before every edit.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SETTINGS="$REPO/config/claude/settings.json"
LESSONS="$REPO/config/claude/lessons"
CLAUDE_MD="$REPO/config/claude/CLAUDE.md"

pass=0
fail=0
skipped=0
ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
skip() { printf '  \033[33mSKIP\033[0m %s\n' "$1"; skipped=$((skipped + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "lessons"

# --- the lesson files --------------------------------------------------------
n=0
bad=""
for f in "$LESSONS"/*.md; do
  [ -f "$f" ] || continue
  n=$((n + 1))
  name="$(basename "$f")"
  [ "$(head -1 "$f")" = "---" ] || { bad="$bad $name(no-frontmatter)"; continue; }
  fm="$(awk 'NR==1{next} /^---$/{exit} {print}' "$f")"
  printf '%s\n' "$fm" | rg -q '^(triggers|commands):' || bad="$bad $name(no-trigger)"
  body="$(awk 'c>=2{print} /^---$/{c++}' "$f")"
  [ "${#body}" -le 601 ] || bad="$bad $name(body-${#body}-chars)"
done
assert_eq "there are lessons" "yes" "$([ "$n" -ge 4 ] && echo yes || echo no)"
assert_eq "every lesson has triggers/commands and a body of at most 600 chars" "" "$bad"

# --- the wiring --------------------------------------------------------------
hook="$(jq -c '[.hooks.PreToolUse[]? | select(.matcher == "Edit|Write|NotebookEdit|Bash") | .hooks[]
  | select(.command | test("hook lessons"))] | first // empty' "$SETTINGS")"
assert_eq "PreToolUse runs the lessons hook for Edit|Write|NotebookEdit|Bash" "yes" \
  "$([ -n "$hook" ] && echo yes || echo no)"
assert_eq "the lessons hook is synchronous (context must arrive before the call)" "false" \
  "$(printf '%s' "${hook:-{\}}" | jq -r '.async // false')"
assert_eq "the lessons hook has a short timeout" "yes" \
  "$([ "$(printf '%s' "${hook:-{\}}" | jq -r '.timeout // 999')" -le 5 ] && echo yes || echo no)"

cmd="$(printf '%s' "${hook:-{\}}" | jq -r '.command // empty')"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# The binary may be absent (fresh machine, npm not installed): the hook must
# then be invisible, not an error on every single edit.
out="$(HOME="$tmp/home" PATH="/usr/bin:/bin" sh -c "$cmd" </dev/null 2>&1)"
rc=$?
assert_eq "with no aval binary the hook is a silent no-op (exit 0)" "0:" "$rc:$out"

assert_eq "install.manifest links the lessons directory" "yes" \
  "$(rg -q '^config/claude/lessons\s+\.claude/lessons$' "$REPO/install.manifest" && echo yes || echo no)"

# --- behaviour against the real lessons, with the built binary ---------------
if command -v go >/dev/null 2>&1; then
  BIN="$tmp/aval"
  (cd "$REPO" && go build -o "$BIN" ./cmd/aval) >/dev/null 2>&1
fi

if [ -x "${BIN:-}" ]; then
  export AVAL_LESSONS_DIR="$LESSONS" XDG_STATE_HOME="$tmp/state"
  fire() { printf '%s' "$2" | "$BIN" hook lessons | jq -r '.hookSpecificOutput.additionalContext // empty'; }
  has() { printf '%s' "$2" | rg -q -- "$1" && echo yes || echo no; }

  assert_eq "editing a .tsx file injects the screenshot lesson" "yes" \
    "$(has 'screenshot|shot.mjs' "$(fire s1 '{"session_id":"u1","tool_name":"Edit","tool_input":{"file_path":"/p/web/src/Card.tsx"}}')")"
  assert_eq "editing a .tsx file injects the native-elements lesson" "yes" \
    "$(has 'native' "$(fire s1 '{"session_id":"u2","tool_name":"Write","tool_input":{"file_path":"/p/web/src/Card.tsx"}}')")"
  assert_eq "editing a Dockerfile injects the Kiri lesson" "yes" \
    "$(has 'Kiri' "$(fire s1 '{"session_id":"d1","tool_name":"Edit","tool_input":{"file_path":"/p/api/Dockerfile"}}')")"
  assert_eq "running docker injects the Kiri lesson" "yes" \
    "$(has 'Kiri' "$(fire s1 '{"session_id":"d2","tool_name":"Bash","tool_input":{"command":"docker compose up -d"}}')")"
  assert_eq "committing injects the attribution lesson" "yes" \
    "$(has 'Co-Authored-By' "$(fire s1 '{"session_id":"c1","tool_name":"Bash","tool_input":{"command":"git commit -m \"feat: x\""}}')")"
  assert_eq "editing plain Go code injects nothing" "" \
    "$(fire s1 '{"session_id":"n1","tool_name":"Edit","tool_input":{"file_path":"/p/main.go"}}')"
  assert_eq "a lesson shows once per session" "" \
    "$(fire s1 '{"session_id":"d1","tool_name":"Edit","tool_input":{"file_path":"/p/api/Dockerfile"}}')"

  # --- latency ---------------------------------------------------------------
  # The hook runs before every Edit/Write/Bash call. Measured, not assumed.
  if command -v hyperfine >/dev/null 2>&1; then
    printf '%s' '{"session_id":"lat","tool_name":"Edit","tool_input":{"file_path":"/p/main.go"}}' >"$tmp/in.json"
    hyperfine -N --warmup 10 --runs 50 --input "$tmp/in.json" --export-json "$tmp/hf.json" \
      "$BIN hook lessons" >/dev/null 2>&1
    # hyperfine 1.x puts the mean in .results[0].mean; newer releases in
    # .results[0].summary.time_wall_clock.mean.
    mean_ms="$(jq -r '.results[0] | ((.summary.time_wall_clock.mean // .mean) * 1000 | floor)' "$tmp/hf.json" 2>/dev/null)"
    assert_eq "the lessons hook runs in under 10 ms (is ${mean_ms:-?} ms)" "yes" \
      "$([ -n "$mean_ms" ] && [ "$mean_ms" -lt 10 ] && echo yes || echo no)"
  else
    skip "latency assertion (hyperfine not installed)"
  fi
else
  skip "behaviour and latency (go toolchain not installed)"
fi

# --- the always-on text got smaller ------------------------------------------
block="$(awk '$0=="<!-- shared:agent-rules -->"{f=1;next} $0=="<!-- /shared:agent-rules -->"{f=0} f' "$CLAUDE_MD")"
MAXC=3000
assert_eq "the shared always-on block stays under ${MAXC} chars (is ${#block})" "yes" \
  "$([ "${#block}" -le "$MAXC" ] && echo yes || echo no)"
leaked=""
for needle in 'lint_native_elements' 'shot.mjs' 'BUILDX_BUILDER' 'OrbStack'; do
  printf '%s' "$block" | rg -qF -- "$needle" && leaked="$leaked $needle"
done
assert_eq "path-scoped rules left the always-on block" "" "$leaked"

printf '\n%d passed, %d failed' "$pass" "$fail"
[ "$skipped" -eq 0 ] || printf ', %d skipped' "$skipped"
printf '\n'
[ "$fail" -eq 0 ]

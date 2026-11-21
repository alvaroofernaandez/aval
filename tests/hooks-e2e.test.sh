#!/usr/bin/env bash
# Fake-agent end to end: play a scripted Claude Code session against the REAL
# `aval hook ...` commands, wired the way config/claude/settings.json wires them.
#
# The unit suites (design-pipeline, design-shot-stop) call each hook directly.
# This one proves the wiring too: the matcher in settings.json decides whether
# a hook runs at all, the command string is executed as configured, and the
# JSON on stdout is the exact decision Claude Code would act on. No API keys,
# no network, no real $HOME/$XDG: everything lives in a temp dir.
#
# Only the design-pipeline hooks (`aval hook design-*`) are fired; third-party
# hooks in the same settings (gentle-ai, rtk) are not ours to exercise and
# would need the network or other installs.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SETTINGS="$REPO/config/claude/settings.json"
. "$REPO/tests/lib/aval-bin.sh"

pass=0
fail=0
ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "hooks-e2e (fake agent)"

for tool in jq git; do
  command -v "$tool" >/dev/null 2>&1 || { echo "  missing: $tool"; exit 2; }
done

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# --- an isolated machine -------------------------------------------------------
FAKE_HOME="$TMP/home"
mkdir -p "$FAKE_HOME/.claude" "$FAKE_HOME/.local/bin" "$FAKE_HOME/.cache"
build_aval "$FAKE_HOME/.local/bin" >/dev/null   # what the installer puts on PATH
[ -x "$FAKE_HOME/.local/bin/aval" ] || { echo "  aval did not build"; exit 2; }
export HOME="$FAKE_HOME"
export XDG_CACHE_HOME="$FAKE_HOME/.cache"
export PATH="$FAKE_HOME/.local/bin:$PATH"
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
unset DESIGN_PIPELINE_OFF DESIGN_ACK_TTL

# --- a project the agent works in ----------------------------------------------
PROJ="$TMP/app"
mkdir -p "$PROJ/src/components" "$PROJ/.agents"
git -C "$PROJ" init -q
printf '# Design\nTokens only.\n' >"$PROJ/.agents/DESIGN.md"
PROJ="$(cd "$PROJ" && pwd -P)"

# --- the fake agent -------------------------------------------------------------
# fire <event> <tool> <payload-json>: run every repo hook configured for the
# event whose matcher matches the tool, exactly like Claude Code, and print
# their combined stdout.
fire() {
  local event="$1" tool="$2" payload="$3" matcher command out=""
  while IFS=$'\t' read -r matcher command; do
    [ -n "$command" ] || continue
    case "$command" in *'hook design-'*) ;; *) continue ;; esac
    # "*" stands for "no matcher" (an empty tab field would be collapsed by read).
    if [ "$matcher" != "*" ] && ! [[ $tool =~ ^($matcher)$ ]]; then continue; fi
    out+="$(cd "$PROJ" && printf '%s' "$payload" | bash -c "$command" 2>/dev/null)"
  done < <(jq -r --arg ev "$event" \
    '.hooks[$ev][]? | (.matcher // "" | if . == "" then "*" else . end) as $m |.hooks[] | [$m, .command] | @tsv' "$SETTINGS")
  printf '%s' "$out"
}

pre_payload() { # $1 tool  $2 file  $3 session  [$4 transcript]
  jq -cn --arg tool "$1" --arg fp "$2" --arg sid "$3" --arg tp "${4:-$TMP/main.jsonl}" --arg cwd "$PROJ" \
    '{session_id:$sid, transcript_path:$tp, cwd:$cwd, hook_event_name:"PreToolUse",
      tool_name:$tool, tool_input:{file_path:$fp}}'
}
stop_payload() { # $1 session  [$2 stop_hook_active]
  jq -cn --arg sid "$1" --argjson active "${2:-false}" --arg cwd "$PROJ" \
    '{session_id:$sid, transcript_path:"/x.jsonl", cwd:$cwd, hook_event_name:"Stop", stop_hook_active:$active}'
}
pre() { fire PreToolUse "$1" "$(pre_payload "$@")"; }
stop() { fire Stop "" "$(stop_payload "$@")"; }

deny_json() { # $1 file name  $2 why
  jq -cn --arg r "Blocked: $1 is a UI surface and $2. Do the design pipeline (invoke the design-pipeline skill if you have not: laws of UX, direction from .agents/DESIGN.md, gates), then run \`aval design ack <surface>\` and retry." \
    '{hookSpecificOutput:{hookEventName:"PreToolUse",permissionDecision:"deny",permissionDecisionReason:$r}}'
}
compact() { [ -z "$1" ] && return; printf '%s' "$1" | jq -c . 2>/dev/null || printf '%s' "$1"; }

BTN="$PROJ/src/components/Button.tsx"
MAIN=s-main

# --- the wiring itself ----------------------------------------------------------
assert_eq "settings.json wires the gate on PreToolUse Edit|Write|NotebookEdit" "Edit|Write|NotebookEdit" \
  "$(jq -r '[.hooks.PreToolUse[] | select(.hooks[].command | test("hook design-gate"))][0].matcher' "$SETTINGS")"
assert_eq "settings.json wires the shot hook on Stop" "yes" \
  "$(jq -r '[.hooks.Stop[].hooks[].command | select(test("hook design-shot-stop"))] | length > 0 | if . then "yes" else "no" end' "$SETTINGS")"

# --- turn 1: the agent edits a UI file before the pipeline ----------------------
assert_eq "Edit of a .tsx with no design-ack is denied (exact JSON)" \
  "$(deny_json Button.tsx 'no design-ack for this project yet')" \
  "$(compact "$(pre Edit "$BTN" $MAIN)")"

assert_eq "a Bash tool call never reaches the gate (matcher)" "" \
  "$(pre Bash "$BTN" $MAIN)"

# --- the agent runs design-ack and retries --------------------------------------
(cd "$PROJ" && aval design ack pricing >/dev/null 2>&1)
assert_eq "design-ack wrote a marker under the temp cache" "yes" \
  "$([ -n "$(ls "$XDG_CACHE_HOME/design-gate" 2>/dev/null)" ] && echo yes || echo no)"
assert_eq "after design-ack the same Edit is allowed (no output)" "" "$(pre Edit "$BTN" $MAIN)"
assert_eq "...and the session is stamped for the Stop hook" "$PROJ" \
  "$(cat "$XDG_CACHE_HOME/design-gate/ui-edited.$MAIN" 2>/dev/null)"

# --- a sub-agent: another session and transcript --------------------------------
assert_eq "a sub-agent edit (different session + transcript) is allowed" "" \
  "$(pre Write "$BTN" agent-7 "$TMP/subagents/agent-7.jsonl")"

# --- non-UI edits ----------------------------------------------------------------
assert_eq "a non-UI edit is allowed" "" "$(pre Edit "$PROJ/src/util.ts" $MAIN)"
assert_eq "a test file is allowed (TDD writes it first)" "" "$(pre Write "$PROJ/src/components/Button.test.tsx" $MAIN)"

start="${EPOCHREALTIME:-}"
if [ -n "$start" ]; then
  payload="$(pre_payload Edit "$PROJ/src/util.ts" $MAIN)"
  t0="${EPOCHREALTIME//[.,]/}"
  for _ in 1 2 3 4 5; do printf '%s' "$payload" | "$FAKE_HOME/.local/bin/aval" hook design-gate >/dev/null; done
  t1="${EPOCHREALTIME//[.,]/}"
  per=$(((10#$t1 - 10#$t0) / 5000))
  assert_eq "a non-UI edit costs under 150 ms through the gate (was ${per} ms)" "yes" \
    "$([ "$per" -lt 150 ] && echo yes || echo no)"
else
  printf '  SKIP non-UI latency (bash without EPOCHREALTIME)\n'
fi

# --- editing the design contract invalidates the ack ------------------------------
printf '# Design\nTokens only.\nNew rule.\n' >"$PROJ/.agents/DESIGN.md"
assert_eq "changing DESIGN.md revokes the ack (exact JSON)" \
  "$(deny_json Button.tsx '.agents/DESIGN.md changed since the last design-ack')" \
  "$(compact "$(pre Edit "$BTN" $MAIN)")"
(cd "$PROJ" && aval design ack pricing >/dev/null 2>&1)
assert_eq "a fresh design-ack allows it again" "" "$(pre Edit "$BTN" $MAIN)"

# --- end of turn --------------------------------------------------------------------
STOP_BLOCK='{"decision":"block","reason":"UI files were edited this session but .design/shots has no render newer than the last edit. Run `node ~/.claude/skills/design-pipeline/scripts/shot.mjs <url|file.html>` and quote its paths and numbers, or say \"not visually verified\" in your reply."}'
assert_eq "Stop after a UI edit with no shots blocks (exact JSON)" \
  "$(printf '%s' "$STOP_BLOCK" | jq -c .)" "$(compact "$(stop $MAIN)")"
assert_eq "the second Stop is allowed (blocks once)" "" "$(stop $MAIN)"
assert_eq "Stop with stop_hook_active never blocks" "" "$(stop $MAIN true)"
assert_eq "Stop in a session with no UI edit is silent" "" "$(stop s-untouched)"

# A later UI edit re-arms the block; a render newer than the edit satisfies it.
sleep 1
assert_eq "a later UI edit is still allowed" "" "$(pre Edit "$BTN" $MAIN)"
assert_eq "...and re-arms the Stop block" "block" "$(compact "$(stop $MAIN)" | jq -r '.decision // "none"')"
mkdir -p "$PROJ/.design/shots"
touch -t 203001010000 "$PROJ/.design/shots/home-390.png"
assert_eq "Stop with a render newer than the edit is allowed" "" "$(stop $MAIN)"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

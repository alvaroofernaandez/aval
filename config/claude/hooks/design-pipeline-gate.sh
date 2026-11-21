#!/usr/bin/env bash
# PreToolUse gate for the design pipeline declared in ~/.claude/CLAUDE.md.
#
# Contract: an Edit/Write on a UI surface is denied until `design-ack <surface>`
# has written a marker for the file's project (see
# shared/skills/design-pipeline/scripts/design-ack):
#
#   ${XDG_CACHE_HOME:-~/.cache}/design-gate/<sha1 of project root>
#
# The marker must be younger than DESIGN_ACK_TTL seconds (default 14400 = 4 h)
# and carry the sha1 of the project's .agents/DESIGN.md as it is NOW, so editing
# the design contract invalidates earlier acks.
#
# Why a marker and not a transcript scan: the old gate searched the session
# transcript for a filled checklist. Over 60 days that denied edits 229 times in
# 53 sessions (the checklist disappears at compaction and never exists in a
# sub-agent transcript) and cost 20-130 ms per call, growing with the transcript.
# A marker is per project, so sub-agents and post-compaction turns just work.
#
# Cost: non-UI paths exit inside a bash `case`, before any subprocess. The UI
# path spawns one hash process. stdin is parsed with bash regexes, no jq.
#
# On an allowed UI edit it also stamps ${cache}/ui-edited.<session_id> with the
# project root; design-shot-stop.sh reads that stamp.
#
# Fails OPEN on anything it cannot determine (unparseable stdin, no hash tool).
# DESIGN_PIPELINE_OFF=1 bypasses deliberately.
#
# Tests: tests/design-pipeline.test.sh

[ -n "${DESIGN_PIPELINE_OFF:-}" ] && exit 0

IFS= read -r -d '' INPUT || true
[ -n "$INPUT" ] || exit 0

RE_TOOL='"tool_name"[[:space:]]*:[[:space:]]*"([A-Za-z]+)"'
RE_FILE='"file_path"[[:space:]]*:[[:space:]]*"(([^"\\]|\\.)*)"'
RE_CWD='"cwd"[[:space:]]*:[[:space:]]*"(([^"\\]|\\.)*)"'
RE_SID='"session_id"[[:space:]]*:[[:space:]]*"([A-Za-z0-9._-]+)"'

[[ $INPUT =~ $RE_TOOL ]] || exit 0
case "${BASH_REMATCH[1]}" in
  Edit | Write | NotebookEdit) ;;
  *) exit 0 ;;
esac

[[ $INPUT =~ $RE_FILE ]] || exit 0
FILE="${BASH_REMATCH[1]}"
[ -n "$FILE" ] || exit 0
FILE="${FILE//\\\//\/}"
FILE="${FILE//\\\\/\\}"

# --- is this a design surface? (a `case`, so nothing is spawned) --------------
# Tests assert behaviour and are not design surfaces: blocking them would
# deadlock against strict TDD, which demands the test be written first.
BASE="${FILE##*/}"
case "$BASE" in
  *.test.* | *.spec.* | *.stories.test.*) exit 0 ;;
esac
case "$FILE" in
  */__tests__/* | */node_modules/*) exit 0 ;;
esac
case "$FILE" in
  *.tsx | *.jsx | *.vue | *.svelte | *.astro) ;;
  *.css | *.scss | *.sass | *.less | *.styl) ;;
  *.html | *.htm) ;;
  *) exit 0 ;;
esac

# --- which project? -----------------------------------------------------------
CWD=""
[[ $INPUT =~ $RE_CWD ]] && CWD="${BASH_REMATCH[1]}"
SID=""
[[ $INPUT =~ $RE_SID ]] && SID="${BASH_REMATCH[1]}"

case "$FILE" in
  /*) ;;
  *) FILE="${CWD:-$PWD}/$FILE" ;;
esac

# Walk up from the file looking for .git (dir or worktree file); builtins only.
ROOT=""
d="${FILE%/*}"
while [ -n "$d" ]; do
  if [ -e "$d/.git" ]; then ROOT="$d"; break; fi
  d="${d%/*}"
done
[ -n "$ROOT" ] || ROOT="${CWD:-${FILE%/*}}"
# Physical path, to match design-ack's `pwd -P` / git toplevel.
cd -P "$ROOT" 2>/dev/null && ROOT="$PWD"

# --- hash the root and DESIGN.md in one process --------------------------------
if command -v sha1sum >/dev/null 2>&1; then SHA=(sha1sum)
elif command -v shasum >/dev/null 2>&1; then SHA=(shasum -a 1)
else SHA=(); fi
[ "${#SHA[@]}" -gt 0 ] || exit 0

DESIGN="$ROOT/.agents/DESIGN.md"
if [ -f "$DESIGN" ]; then
  OUT="$(printf '%s' "$ROOT" | "${SHA[@]}" - "$DESIGN" 2>/dev/null)"
else
  OUT="$(printf '%s' "$ROOT" | "${SHA[@]}" - 2>/dev/null)"
fi
KEY="${OUT%% *}"
[ -n "$KEY" ] || exit 0
DHASH=""
if [ -f "$DESIGN" ]; then
  rest="${OUT#*$'\n'}"
  DHASH="${rest%% *}"
fi

# --- read the marker -----------------------------------------------------------
CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/design-gate"
WHY="no design-ack for this project yet"
M_EPOCH="" M_HASH=""
if [ -f "$CACHE/$KEY" ]; then
  while IFS='=' read -r k v; do
    case "$k" in
      epoch) M_EPOCH="$v" ;;
      design_sha1) M_HASH="$v" ;;
    esac
  done <"$CACHE/$KEY"

  TTL="${DESIGN_ACK_TTL:-14400}"
  case "$TTL" in '' | *[!0-9]*) TTL=14400 ;; esac
  NOW="${EPOCHSECONDS:-}"
  [ -n "$NOW" ] || NOW="$(date +%s)"

  case "$M_EPOCH" in
    '' | *[!0-9]*) WHY="the design-ack marker is unreadable" ;;
    *)
      if [ $((NOW - M_EPOCH)) -gt "$TTL" ]; then
        WHY="the last design-ack is older than $((TTL / 60)) min"
      elif [ "$M_HASH" != "$DHASH" ]; then
        WHY=".agents/DESIGN.md changed since the last design-ack"
      else
        # Allowed. Stamp the session so the Stop hook knows a UI file changed.
        [ -n "$SID" ] && printf '%s' "$ROOT" >"$CACHE/ui-edited.$SID" 2>/dev/null
        exit 0
      fi
      ;;
  esac
fi

# --- deny ----------------------------------------------------------------------
# One line, and it teaches: a bare "denied" sends the model looking for a way
# around the gate; naming the next command makes running it the obvious move.
NAME="${BASE//[\"\\]/_}"
printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Blocked: %s is a UI surface and %s. Do the design pipeline (invoke the design-pipeline skill if you have not: laws of UX, direction from .agents/DESIGN.md, gates), then run `design-ack <surface>` and retry."}}\n' "$NAME" "$WHY"

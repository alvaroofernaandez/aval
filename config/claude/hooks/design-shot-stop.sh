#!/usr/bin/env bash
# Stop hook: a turn that edited a UI file must end with a render, or an honest
# "not visually verified".
#
# design-pipeline-gate.sh stamps ${cache}/design-gate/ui-edited.<session_id>
# (content: project root) whenever it allows a UI edit. Here, if that stamp
# exists and the project's .design/shots holds no file newer than it, block the
# stop ONCE per stamp, asking for shot.mjs. "Once" is recorded in
# ui-edited.<session_id>.blocked: a block is only issued while that record is
# missing or older than the stamp, so a later UI edit can block again but a
# stubborn reply never loops.
#
# Cost: the common case (no UI edit this session) is one failed stat. Builtins
# only, no subprocess. Fails open on anything unparseable.
#
# Tests: tests/design-shot-stop.test.sh

[ -n "${DESIGN_PIPELINE_OFF:-}" ] && exit 0

IFS= read -r -d '' INPUT || true
[ -n "$INPUT" ] || exit 0

RE_SID='"session_id"[[:space:]]*:[[:space:]]*"([A-Za-z0-9._-]+)"'
RE_ACTIVE='"stop_hook_active"[[:space:]]*:[[:space:]]*true'
[[ $INPUT =~ $RE_SID ]] || exit 0
SID="${BASH_REMATCH[1]}"
[[ $INPUT =~ $RE_ACTIVE ]] && exit 0

CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/design-gate"
STAMP="$CACHE/ui-edited.$SID"
[ -f "$STAMP" ] || exit 0

BLOCKED="$STAMP.blocked"
if [ -e "$BLOCKED" ] && ! [ "$STAMP" -nt "$BLOCKED" ]; then exit 0; fi

IFS= read -r ROOT <"$STAMP" || true
[ -n "$ROOT" ] || exit 0

for f in "$ROOT"/.design/shots/*; do
  [ -f "$f" ] && [ "$f" -nt "$STAMP" ] && exit 0
done

: >"$BLOCKED" 2>/dev/null
printf '%s\n' '{"decision":"block","reason":"UI files were edited this session but .design/shots has no render newer than the last edit. Run `node ~/.claude/skills/design-pipeline/scripts/shot.mjs <url|file.html>` and quote its paths and numbers, or say \"not visually verified\" in your reply."}'
exit 0

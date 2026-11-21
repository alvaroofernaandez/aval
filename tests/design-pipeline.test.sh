#!/usr/bin/env bash
# Tests for the design-pipeline gate and its `design-ack` marker.
#
# History: the gate used to scan the session transcript for a filled
# [design-pipeline] checklist. Measured over 60 days that denied edits 229
# times in 53 sessions: the checklist vanished after compaction and never
# existed in a sub-agent transcript, and every call paid for a jq scan of a
# transcript that grows (20 ms for a non-UI edit, 130 ms for a UI edit).
#
# The gate now reads a MARKER instead: `design-ack <surface>` writes
# ${XDG_CACHE_HOME:-~/.cache}/design-gate/<sha1 of project root> after the
# pipeline has been done, and the hook allows a UI edit while that marker is
# fresh (4 h) and .agents/DESIGN.md still hashes the same. The marker is per
# project, so sub-agents and post-compaction turns work with no transcript.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOOK="$REPO/config/claude/hooks/design-pipeline-gate.sh"
ACK="$REPO/shared/skills/design-pipeline/scripts/design-ack"
SETTINGS="$REPO/config/claude/settings.json"

pass=0
fail=0

ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "design-pipeline"

TMP="$(mktemp -d)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

export XDG_CACHE_HOME="$TMP/cache"
unset DESIGN_PIPELINE_OFF DESIGN_ACK_TTL

# --- helpers -----------------------------------------------------------------

sha_of() { local o; o="$(printf '%s' "$1" | shasum -a 1)"; printf '%s' "${o%% *}"; }

# Marker path for a project, computed independently of the code under test.
marker_for() { printf '%s/design-gate/%s' "$XDG_CACHE_HOME" "$(sha_of "$(cd "$1" && pwd -P)")"; }

ack() { (cd "$1" && "$ACK" "${2:-component}" >/dev/null 2>&1); }

# Feed the hook a PreToolUse payload and print its stdout.
run_hook() { # $1 tool  $2 file  $3 cwd  [$4 transcript]  [$5 session]
  jq -cn --arg tool "$1" --arg fp "$2" --arg cwd "$3" \
    --arg tp "${4:-$TMP/transcript.jsonl}" --arg sid "${5:-sess-1}" \
    '{session_id:$sid, transcript_path:$tp, cwd:$cwd,
      hook_event_name:"PreToolUse", tool_name:$tool, tool_input:{file_path:$fp}}' \
    | "$HOOK" 2>/dev/null
}

decision() {
  [ -z "$1" ] && { printf 'none'; return; }
  printf '%s' "$1" | jq -r '.hookSpecificOutput.permissionDecision // "none"' 2>/dev/null || printf 'none'
}

# --- the pieces exist --------------------------------------------------------

assert_eq "gate hook exists" "yes" "$([ -f "$HOOK" ] && echo yes || echo no)"
assert_eq "gate hook is executable" "yes" "$([ -x "$HOOK" ] && echo yes || echo no)"
assert_eq "design-ack exists" "yes" "$([ -f "$ACK" ] && echo yes || echo no)"
assert_eq "design-ack is executable" "yes" "$([ -x "$ACK" ] && echo yes || echo no)"

if [ ! -x "$HOOK" ]; then
  printf '\n%d passed, %d failed\n' "$pass" "$((fail + 1))"
  exit 1
fi

# --- design-ack --------------------------------------------------------------

P1="$TMP/p1"; mkdir -p "$P1/src" "$P1/.agents"
printf '# DESIGN\n' >"$P1/.agents/DESIGN.md"
ACK_OUT="$(cd "$P1" && "$ACK" dashboard 2>&1)"
M1="$(marker_for "$P1")"

assert_eq "design-ack writes the project marker" "yes" "$([ -f "$M1" ] && echo yes || echo no)"
assert_eq "marker records the surface" "yes" \
  "$(rg -q '^surface=dashboard$' "$M1" 2>/dev/null && echo yes || echo no)"
assert_eq "marker records an epoch time" "yes" \
  "$(rg -q '^epoch=[0-9]{10}$' "$M1" 2>/dev/null && echo yes || echo no)"
assert_eq "marker records the DESIGN.md sha1" "$(sha_of_file() { local o; o="$(shasum -a 1 "$1")"; printf '%s' "${o%% *}"; }; sha_of_file "$P1/.agents/DESIGN.md")" \
  "$(rg -o '^design_sha1=([0-9a-f]{40})$' -r '$1' "$M1" 2>/dev/null)"
for word in laws direction gates; do
  assert_eq "design-ack reminder names $word" "yes" \
    "$(printf '%s' "$ACK_OUT" | rg -qi -- "$word" && echo yes || echo no)"
done
assert_eq "design-ack without a surface fails" "yes" \
  "$( (cd "$P1" && "$ACK" >/dev/null 2>&1) && echo no || echo yes)"

# The project root is the git toplevel of cwd, not cwd itself.
P2="$TMP/p2"; mkdir -p "$P2/src/deep"
git -C "$P2" init -q
(cd "$P2/src/deep" && "$ACK" card >/dev/null 2>&1)
assert_eq "design-ack from a subdirectory marks the git toplevel" "yes" \
  "$([ -f "$(marker_for "$(git -C "$P2" rev-parse --show-toplevel)")" ] && echo yes || echo no)"

# --- enforcement -------------------------------------------------------------

P3="$TMP/p3"; mkdir -p "$P3/src"

assert_eq "UI file with no marker is denied" "deny" \
  "$(decision "$(run_hook Edit "$P3/src/Button.tsx" "$P3")")"

ack "$P3"
assert_eq "UI file with a fresh marker is allowed" "none" \
  "$(decision "$(run_hook Edit "$P3/src/Button.tsx" "$P3")")"
assert_eq "a stylesheet is covered by the same marker" "none" \
  "$(decision "$(run_hook Write "$P3/src/app.css" "$P3")")"

# stale: rewrite the epoch to five hours ago (TTL default is four).
M3="$(marker_for "$P3")"
now="$(date +%s)"
sd '^epoch=.*$' "epoch=$((now - 18000))" "$M3"
assert_eq "a marker older than 4 hours is denied" "deny" \
  "$(decision "$(run_hook Edit "$P3/src/Button.tsx" "$P3")")"
assert_eq "DESIGN_ACK_TTL can lengthen the window" "none" \
  "$(DESIGN_ACK_TTL=86400 decision "$(DESIGN_ACK_TTL=86400 run_hook Edit "$P3/src/Button.tsx" "$P3")")"
sd '^epoch=.*$' "epoch=$((now - 30))" "$M3"
assert_eq "DESIGN_ACK_TTL can shorten the window" "deny" \
  "$(decision "$(DESIGN_ACK_TTL=5 run_hook Edit "$P3/src/Button.tsx" "$P3")")"

# DESIGN.md edited after the ack means the direction changed under the code.
P4="$TMP/p4"; mkdir -p "$P4/src" "$P4/.agents"
printf 'tokens v1\n' >"$P4/.agents/DESIGN.md"
ack "$P4"
assert_eq "an unchanged DESIGN.md keeps the marker valid" "none" \
  "$(decision "$(run_hook Edit "$P4/src/Button.tsx" "$P4")")"
printf 'tokens v2\n' >"$P4/.agents/DESIGN.md"
assert_eq "DESIGN.md changed after the ack is denied" "deny" \
  "$(decision "$(run_hook Edit "$P4/src/Button.tsx" "$P4")")"
ack "$P4"
assert_eq "re-running design-ack reopens the gate" "none" \
  "$(decision "$(run_hook Edit "$P4/src/Button.tsx" "$P4")")"

# The marker is per project: acking p3 says nothing about p5.
P5="$TMP/p5"; mkdir -p "$P5/src"
sd '^epoch=.*$' "epoch=$(date +%s)" "$M3"
assert_eq "another project's marker does not open this one" "deny" \
  "$(decision "$(run_hook Edit "$P5/src/Button.tsx" "$P5")")"

# A sub-agent call carries another transcript (or none at all); the marker is
# all that matters, which is why sub-agents work with no special case.
assert_eq "a sub-agent style call with another transcript is allowed" "none" \
  "$(decision "$(run_hook Edit "$P3/src/Button.tsx" "$P3" "$TMP/agent-xyz/subagents/agent-1.jsonl" other-session)")"
assert_eq "a call with a missing transcript is allowed too" "none" \
  "$(decision "$(run_hook Edit "$P3/src/Button.tsx" "$P3" "/nonexistent/t.jsonl")")"

# --- what is not a design surface --------------------------------------------

assert_eq "non-UI source with no marker is allowed" "none" \
  "$(decision "$(run_hook Edit "$P5/main.go" "$P5")")"
for f in src/Button.test.tsx src/Button.spec.tsx src/__tests__/Button.tsx node_modules/x/y.css; do
  assert_eq "exempt path $f is allowed with no marker" "none" \
    "$(decision "$(run_hook Write "$P5/$f" "$P5")")"
done
assert_eq "tools other than Edit/Write pass through" "none" \
  "$(decision "$(run_hook Bash "$P5/src/Button.tsx" "$P5")")"

# A non-UI edit must exit before spawning anything: with an empty PATH there is
# no jq, no git, no hash tool, so only bash builtins can have run.
NONUI_JSON="$(jq -cn --arg fp "$P5/main.go" '{tool_name:"Edit",tool_input:{file_path:$fp},cwd:"/x"}')"
EMPTY_PATH_OUT="$(printf '%s' "$NONUI_JSON" | PATH=/nonexistent "$(command -v bash)" "$HOOK" 2>&1; echo "rc=$?")"
assert_eq "a non-UI edit runs on bash builtins alone" "rc=0" "$EMPTY_PATH_OUT"

# --- bypass and fail-open ----------------------------------------------------

assert_eq "DESIGN_PIPELINE_OFF bypasses the gate" "none" \
  "$(decision "$(DESIGN_PIPELINE_OFF=1 run_hook Edit "$P5/src/Button.tsx" "$P5")")"

for bad in '' 'not json at all' '{"tool_name":"Edit","tool_input":{"file_path":' '{"tool_name":"Edit"}' '{}'; do
  out="$(printf '%s' "$bad" | "$HOOK" 2>/dev/null; echo "rc=$?")"
  assert_eq "malformed stdin fails open: ${bad:-<empty>}" "rc=0" "$out"
done

# --- the ui-edited stamp the Stop hook reads ---------------------------------

STAMP="$XDG_CACHE_HOME/design-gate/ui-edited.sess-stamp"
rm -f "$STAMP"
run_hook Edit "$P5/src/Button.tsx" "$P5" "" sess-stamp >/dev/null
assert_eq "a denied UI edit leaves no stamp" "no" "$([ -e "$STAMP" ] && echo yes || echo no)"
run_hook Edit "$P5/main.go" "$P5" "" sess-stamp >/dev/null
assert_eq "a non-UI edit leaves no stamp" "no" "$([ -e "$STAMP" ] && echo yes || echo no)"
run_hook Edit "$P3/src/Button.tsx" "$P3" "" sess-stamp >/dev/null
assert_eq "an allowed UI edit stamps the session" "yes" "$([ -e "$STAMP" ] && echo yes || echo no)"
assert_eq "the stamp names the project root" "$(cd "$P3" && pwd -P)" "$(<"$STAMP")"

# --- the denial has to teach, in one line ------------------------------------

DENY="$(run_hook Edit "$P5/src/Button.tsx" "$P5")"
REASON="$(printf '%s' "$DENY" | jq -r '.hookSpecificOutput.permissionDecisionReason // ""')"
assert_eq "denial reason names design-ack" "yes" \
  "$(printf '%s' "$REASON" | rg -q -- 'design-ack <surface>' && echo yes || echo no)"
assert_eq "denial reason names the design-pipeline skill" "yes" \
  "$(printf '%s' "$REASON" | rg -q -- 'design-pipeline' && echo yes || echo no)"
assert_eq "denial reason is a single line" "1" "$(printf '%s\n' "$REASON" | wc -l | tr -d ' ')"

# --- wiring ------------------------------------------------------------------
# The hook is inert unless settings.json calls it, and design-ack is inert
# unless the installer links it onto PATH.

assert_eq "settings.json wires the gate as a PreToolUse hook" "yes" \
  "$(jq -e '.hooks.PreToolUse[]?.hooks[]?.command | select(test("design-pipeline-gate"))' \
      "$SETTINGS" >/dev/null 2>&1 && echo yes || echo no)"

assert_eq "the gate matches Edit and Write" "yes" \
  "$(jq -e '.hooks.PreToolUse[]? | select(.hooks[]?.command | test("design-pipeline-gate"))
            | .matcher | select(test("Edit") and test("Write"))' \
      "$SETTINGS" >/dev/null 2>&1 && echo yes || echo no)"

assert_eq "install.manifest links design-ack onto PATH" "yes" \
  "$(rg -q '^shared/skills/design-pipeline/scripts/design-ack\s+\.local/bin/design-ack$' "$REPO/install.manifest" \
      && echo yes || echo no)"

# --- the colliding plugin ----------------------------------------------------
# bencium-innovative-ux-designer ships a description that is a word-for-word
# subset of frontend-design's. Two skills competing for one pipeline step make
# the step ambiguous by construction.

assert_eq "the bencium UX plugin is not enabled" "no" \
  "$(jq -r '.enabledPlugins["bencium-innovative-ux-designer@bencium-marketplace"] // false' \
      "$SETTINGS" | rg -q '^true$' && echo yes || echo no)"

# --- the skill step 0 invokes -------------------------------------------------
# The gate can demand the line; only the skill makes the line mean something.
# It lives in shared/skills so the manifest's fanout installs it into every
# agent's skills directory (Claude Code AND OpenCode) rather than just this one.

LAWS_SKILL="$REPO/shared/skills/laws-of-ux/SKILL.md"

assert_eq "the laws-of-ux skill exists" "yes" \
  "$([ -f "$LAWS_SKILL" ] && echo yes || echo no)"

if [ -f "$LAWS_SKILL" ]; then
  assert_eq "the skill declares frontmatter name and description" "yes" \
    "$(rg -q '^name: laws-of-ux$' "$LAWS_SKILL" && rg -q '^description:' "$LAWS_SKILL" \
        && echo yes || echo no)"

  # All 30 laws from lawsofux.com, verbatim names. A skill that quietly drops
  # half of them is the failure this assertion exists to catch: the ones that
  # get dropped are the unglamorous ones (Postel, Tesler, Parkinson) which are
  # precisely the ones nobody applies from memory.
  MISSING=""
  while IFS= read -r law; do
    rg -qF -- "$law" "$LAWS_SKILL" || MISSING="$MISSING $law"
  done <<'LAWS'
Aesthetic-Usability Effect
Choice Overload
Chunking
Cognitive Bias
Cognitive Load
Doherty Threshold
Fitts's Law
Flow
Goal-Gradient Effect
Hick's Law
Jakob's Law
Law of Common Region
Law of Proximity
Law of Prägnanz
Law of Similarity
Law of Uniform Connectedness
Mental Model
Miller's Law
Occam's Razor
Paradox of the Active User
Pareto Principle
Parkinson's Law
Peak-End Rule
Postel's Law
Selective Attention
Serial Position Effect
Tesler's Law
Von Restorff Effect
Working Memory
Zeigarnik Effect
LAWS
  assert_eq "the skill carries all 30 laws" "" "$MISSING"
fi

# --- the text diet -----------------------------------------------------------
# Measured: ~2.4K tokens of always-on design text per turn, most of it a checklist
# the marker gate no longer reads. The always-on part now has to stay small, and
# the pipeline skill has to say the one thing that matters: do the three steps,
# then `design-ack <surface>`. Detail (the DESIGN.md schema) lives in references/.
PIPELINE_SKILL="$REPO/shared/skills/design-pipeline/SKILL.md"
PSIZE="$(wc -c <"$PIPELINE_SKILL" | tr -d ' ')"
assert_eq "design-pipeline SKILL.md stays under 2500 chars (is $PSIZE)" "yes" \
  "$([ "$PSIZE" -lt 2500 ] && echo yes || echo no)"
assert_eq "the skill names the three steps" "yes" \
  "$(rg -q 'laws-of-ux' "$PIPELINE_SKILL" && rg -q 'direction' "$PIPELINE_SKILL" && rg -q 'gates' "$PIPELINE_SKILL" && echo yes || echo no)"
assert_eq "the skill tells the model to run design-ack" "yes" \
  "$(rg -q 'design-ack <surface>' "$PIPELINE_SKILL" && echo yes || echo no)"
assert_eq "the skill points at shot.mjs for render evidence" "yes" \
  "$(rg -q 'shot\.mjs' "$PIPELINE_SKILL" && echo yes || echo no)"
assert_eq "the skill says .design/ belongs in .gitignore" "yes" \
  "$(rg -q '\.design/' "$PIPELINE_SKILL" && rg -q 'gitignore' "$PIPELINE_SKILL" && echo yes || echo no)"
SCHEMA="$REPO/shared/skills/design-pipeline/references/design-md-schema.md"
assert_eq "the DESIGN.md schema moved to references/" "yes" \
  "$(rg -q 'Intent & North Star' "$SCHEMA" 2>/dev/null && rg -q 'Decision Log' "$SCHEMA" && echo yes || echo no)"
assert_eq "the skill links the schema" "yes" \
  "$(rg -q 'references/design-md-schema\.md' "$PIPELINE_SKILL" && echo yes || echo no)"

# Skills this repo owns keep their trigger words in <= 220 chars. design-gates is
# generated by scripts/p87-sync.sh from a heredoc this repo owns, so the budget
# holds for the heredoc and the generated file alike.
desc_len() { awk '/^---$/{c++;next} c==1' "$1" | awk '/^description:/{f=1;sub(/^description:[ >]*/,"")} f&&/^[a-z-]+:/&&!/^description:/{exit} f{print}' | tr '\n' ' ' | sd '\s+' ' ' | sd '^ | $' '' | wc -c | tr -d ' '; }
for sk in laws-of-ux design-gates design-refactor design-pipeline; do
  n="$(desc_len "$REPO/shared/skills/$sk/SKILL.md")"
  assert_eq "$sk description stays under 220 chars (is $n)" "yes" "$([ "$n" -le 220 ] && echo yes || echo no)"
done

# --- the always-on Design Architecture section ---------------------------------
CLAUDE_MD="$REPO/config/claude/CLAUDE.md"
SECTION="$(awk '/^## Design Architecture/{f=1} f&&/^## /&&!/^## Design Architecture/{exit} f' "$CLAUDE_MD")"
SLEN="$(printf '%s' "$SECTION" | wc -c | tr -d ' ')"
assert_eq "the Design Architecture section stays under 850 chars (is $SLEN)" "yes" \
  "$([ "$SLEN" -le 850 ] && echo yes || echo no)"
for needle in 'DESIGN.md' 'design-pipeline' 'design-ack <surface>' 'DESIGN_PIPELINE_OFF' 'design-gates'; do
  assert_eq "the section still says: $needle" "yes" \
    "$(printf '%s' "$SECTION" | rg -qF -- "$needle" && echo yes || echo no)"
done

# The path-scoped half of the section moved to lessons (aval hook lessons), which
# inject it when a UI file is touched instead of on every turn.
for pair in 'ui-visual-verification:shot.mjs' 'ui-visual-verification:not run' \
            'ui-native-elements:lint_native_elements.py' 'ui-native-elements:components/ui' \
            'ui-native-elements:shadcn/ui'; do
  lesson="${pair%%:*}"; needle="${pair#*:}"
  assert_eq "lesson $lesson still says: $needle" "yes" \
    "$(rg -qF -- "$needle" "$REPO/config/claude/lessons/$lesson.md" && echo yes || echo no)"
done

# CLAUDE.md must still point at the skill, or the move silently drops the rule
# for any session that never thinks to look for it.
CLAUDE_MD="$REPO/config/claude/CLAUDE.md"
assert_eq "CLAUDE.md still routes UI work to the design-pipeline skill" "yes" \
  "$(rg -q 'design-pipeline. skill' "$CLAUDE_MD" && echo yes || echo no)"

assert_eq "CLAUDE.md still states the gate is blocking" "yes" \
  "$(rg -q 'DESIGN_PIPELINE_OFF' "$CLAUDE_MD" && echo yes || echo no)"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

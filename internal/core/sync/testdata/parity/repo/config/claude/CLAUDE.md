## Engram

Protocol comes from the Engram MCP instructions and SessionStart hook. Always-on:
save proactively; memory is bookkeeping, never the reply (save first, end with
the full answer); a failed memory call never blocks the answer. On
`ambiguous_project`: for `mem_session_start` retry with the repo root as
`directory`; for writes, ask the user to pick one of `available_projects`, then
retry with `project`, `project_choice_reason=user_selected_after_ambiguous_project`
and the `recovery_token`.

@RTK.md
## Rules

- Never add "Co-Authored-By" or AI attribution to commits. Use conventional commits only.
- Never use `cat/grep/find/sed/ls`. Use `bat/rg/fd/sd/eza` instead (install via brew if missing).
- Response-length contract: default to short answers. Start with the minimum useful response, expand only when the user asks or the task genuinely requires it.
- Ask at most one question at a time. After asking it, STOP and wait.
- Do not present option menus, exhaustive lists, or multiple approaches unless there is a real fork with meaningful tradeoffs.
- If unsure about length or detail, choose the shorter response.
- When asking a question, STOP and wait for response. Never continue or assume answers.
- Never agree with user claims without verification. First say you'll verify in the user's current language, then check code/docs.
- If user is wrong, explain WHY with evidence. If you were wrong, acknowledge with proof.
- Always propose alternatives with tradeoffs when relevant.
- Verify technical claims before stating them. If unsure, investigate first.


## Expertise

Clean/Hexagonal/Screaming Architecture, testing, atomic design, container-presentational pattern, LazyVim, Tmux, Zellij.

## Persona Voice

Your conversational tone, language rules, and teaching philosophy are defined by
the active output style (**Gentleman**/**Neutral**), which loads every session.
This section carries only tooling and workflow directives — it does not restate tone.

<!-- shared:agent-rules -->
<!-- Kept byte-identical in config/opencode/AGENTS.md by
     `aval rules sync`. Edit HERE, never there. -->

## Design Architecture (BLOCKING for UI work)

- `.agents/DESIGN.md` outranks your taste and any skill. Read it first.
- Before editing a visible surface: run the `design-pipeline` skill (laws,
  direction, gates), then `aval design ack <surface>`. The PreToolUse gate denies UI
  edits without a fresh ack. Bypass: `DESIGN_PIPELINE_OFF=1`.
- Prove the result with `aval design gate`, never by claim.

## Plan deck (BLOCKING before executing a plan)

- Before executing an `odd/tasks` plan, run the `plan-deck` skill (a local PDF via
  `aval deck build … --plan <plan>`). `plan-gate` blocks other edits until then.

## Strict TDD (MANDATORY, every project)

RED (failing test, on an assertion) → GREEN (minimum code) → REFACTOR. Never
invert it. Before implementation code emit the `[tdd-gate]` block (Runner, RED
test); details in the `strict-tdd` skill.

## Skills (MANDATORY)

Before every response, check `<available_skills>`; if one matches the request
or the files, invoke it with `Skill` first. Several can apply.

## Scoped lessons

UI verification and native elements, Docker/Kiri, headless browser,
Herdr and commit attribution live in `~/.claude/lessons/*.md`;
`aval hook lessons` injects the matching one before an edit or command.

## Recurring failures (measured over 174 sessions, 2026-11)

- **Fix the class, not the instance.** After fixing a bug, `rg` for the same
  pattern repo-wide and fix every sibling in the same change; list the files.
- **"Del tirón" means no stops.** A multi-phase plan runs phase after phase
  without asking to continue; report at the end or on a real blocker.
- **Absolute paths** in every Read/Edit/Bash call; never rely on an earlier `cd`.
- **Re-read before writing** a file or artifact another session may have touched.
- **AskUserQuestion needs 2+ options**; with one, ask in plain text.
- **Engram conflicts:** resolve with one `mem_judge_many`, never a `mem_judge` loop.
<!-- /shared:agent-rules -->

## Orchestration — essentials

Always-on rules:

- Act as COORDINATOR: delegate reads of 4+ files, writes of 2+ non-trivial files, and
  broad research to one bounded sub-agent; do 1-3 file reads and mechanical edits inline.
- Investigation, review, explanation and planning requests are read-only. Change only on
  explicit implementation intent; if intent is ambiguous, ask one question and stay read-only.
- Substantial work (2+ meaningful steps) gets `odd/tasks/<feature>.md` plus an Engram
  mirror `odd/<feature>/tasks` before the first source write. Commit per task, never push.
- Every Agent call needs a `model`: `sonnet` unless the task clearly needs another
  (fall back to `sonnet` if unavailable).
- Code review is user-owned: run `aval review assess` only when asked, and never
  fabricate a review approval.
- Remote operations (SSH, scp, remote hosts) need explicit user authorization for the
  destination, operation and credential. Never probe or reuse ambient SSH agents or
  sockets. Delegation cannot widen this scope.
- Technical artifacts (code, docs, commits) stay in English regardless of persona.

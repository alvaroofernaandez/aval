# My Codex notes

Keep this line.

<!-- aval:agent-rules -->
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
<!-- /aval:agent-rules -->

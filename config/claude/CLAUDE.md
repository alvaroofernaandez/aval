<!-- gentle-ai:engram-protocol -->
## Engram Persistent Memory

Engram persistent memory is ACTIVE. The full protocol (save format, lifecycle,
search flow, after-compaction steps) is delivered every session by the Engram
MCP server instructions and the SessionStart hook. Always-on rules:

- Call `mem_save` PROACTIVELY after any decision, bugfix, discovery, convention,
  or config change — do not wait to be asked. Use `capture_prompt: false` for
  automated artifacts.
- On any reference to past work: `mem_context` → `mem_search` → `mem_get_observation`.
- Before saying "done", call `mem_session_summary`.
- Saving to memory is bookkeeping, never the reply: it NEVER counts as answering.
  End every turn with the complete user-facing answer as the final message (no
  tool calls after it), and save memory before composing it — never collapse the
  answer into a "saved / done" acknowledgement.
- If a memory call fails or times out, deliver the answer anyway — memory
  failures never block or replace the reply.
- If `mem_session_start` fails with `ambiguous_project`: resolve the intended
  repository root and retry `mem_session_start` with that root as `directory`.
  The session ID remains unregistered until registration succeeds; never attach
  an unregistered session ID to writes. Do not pass `project`,
  `project_choice_reason`, or `recovery_token` to `mem_session_start`.
- On `ambiguous_project` from write tools (`mem_save`, `mem_save_prompt`,
  `mem_session_summary`): never guess. Ask the user to choose exactly one value
  from `available_projects`, then retry the write tool with `project`,
  `project_choice_reason=user_selected_after_ambiguous_project`, and the
  returned `recovery_token`.
<!-- /gentle-ai:engram-protocol -->

@RTK.md
<!-- gentle-ai:persona -->
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
<!-- /gentle-ai:persona -->

<!-- shared:agent-rules -->
<!-- Kept byte-identical in config/opencode/AGENTS.md by
     scripts/sync-agent-rules.sh. Edit HERE, never there. -->

## Design Architecture (BLOCKING for UI work)

- `.agents/DESIGN.md` outranks your taste and any skill. Read it first.
- Before editing a visible surface: run the `design-pipeline` skill (laws,
  direction, gates), then `design-ack <surface>`. The PreToolUse gate denies UI
  edits without a fresh ack. Bypass: `DESIGN_PIPELINE_OFF=1`.
- Never use native interactive elements (date picker, `<select>`, `<datalist>`,
  `<dialog>`, `confirm()`/`alert()`): shadcn/ui + Radix only, `components/ui`
  exempt. `python3 ~/.claude/skills/design-refactor/scripts/lint_native_elements.py src/`
  must exit 0.
- Before claiming a visual fix or a finished screen, run
  `node ~/.claude/skills/design-gates/scripts/shot.mjs <url|file>` and quote its
  paths and numbers. A gate you did not run is "not run".

## Strict TDD (MANDATORY for ALL projects — frontend AND backend)

Iron law, for any feature, bug fix, or refactor with behavioural change:

1. **RED** — write the failing test first. It must fail on an assertion, not an
   import error.
2. **GREEN** — the minimum code that passes. Nothing more.
3. **REFACTOR** — clean up while green.

Never invert this. "Tests later" never comes. Before writing implementation
code, emit:

```
[tdd-gate]
Runner: <pytest | vitest | jest | go test | …>
RED test: <path::name — what it asserts and why it must fail now>
```

If you cannot fill that block honestly, you may not write implementation code
yet. **Invoke the `strict-tdd` skill** for per-layer scope, the setup-first rule
when a project has no runner.

## gstack (selective adoption — do NOT install upstream over this)

Three skills are vendored from gstack at the commit in `pins/GSTACK_PIN` of the aval repo,
**patched, not upstream copies**, regenerated by `scripts/gstack-sync.sh`. Files
under `shared/skills/` carry a provenance header — never edit them by hand.

**Never run upstream's `./setup`.** It would reinstate every deliberately
rejected skill and shadow the local `/ship`.

**Invoke the `gstack-policy` skill** before installing, syncing or reaching for
any gstack command — it lists what was adopted, what was rejected, and why.

## Contextual Skill Loading (MANDATORY)

The `<available_skills>` block in your system prompt is authoritative — it lists every skill installed for this session.

**Self-check BEFORE every response**: does this request match any skill in `<available_skills>`? If yes, invoke it via the built-in `Skill` tool BEFORE generating your reply. This is a blocking requirement, not optional context. Skipping it is a discipline failure.

Multiple skills can apply at once. Match by file context (extensions, paths) and task context (what the user is asking for).

## Headless browser

- Lightpanda is the default headless browser for Playwright/Puppeteer (via CDP), E2E flows, scraping and page reading.
- It has no rendering engine: screenshots, visual/layout gates and PDF generation stay on Chromium.
- Skill: `lightpanda-browser` — load it before launching any headless browser.

## Terminal and agent workspace

- Herdr is the only workspace (Orca was removed 2026-11-19): `hc` runs terminals and agents, `herdr-app` is the window.
- Delegate long, parallel or cross-agent work and slow commands to `hc run --bg`; never add agent hooks (status is native).
- Skill: `herdr` — load it before any `hc` or `herdr-app` command.

## Docker en macOS

- Use Kiri as the only Docker runtime. Never use, install or recommend OrbStack, Colima or Docker Desktop.
- For images, builds, containers and Compose, use `docker`, `docker compose` and `docker buildx` against the `kiri` context, with `BUILDX_BUILDER=multiarch` (Kiri's own builder can return ARM64 for amd64 builds; verify with `docker image inspect -f '{{.Architecture}}'`).
- Before deleting images, volumes or build cache, show what will be removed and ask for confirmation.
- After large builds or when disk is low, run the safe cleanup (`docker-space-optimizer`). Measure `~/Library/Application Support/Kiri` before/after.
- Prefer small images, multi-stage builds, `.dockerignore` and Buildx cache.
- Do not install Kubernetes unless explicitly asked.
- Skills: `kiri-docker`, `docker-space-optimizer`, `kiri-health`, `docker-build-optimized`.

## Recurring failures (measured over 174 sessions, 2026-11)

- **Visual "fixed" needs pixels.** Never call a visual bug fixed from reading
  code. Render it (Chromium screenshot at the user's viewport) and look, or say
  "not visually verified". ~1 in 5 user messages was a screenshot saying "sigue igual".
- **Fix the class, not the instance.** After fixing a bug, `rg` for the same
  pattern repo-wide and fix every sibling in the same change; list the files.
- **"Del tirón" means no stops.** A multi-phase plan runs phase after phase
  without asking to continue; report at the end or on a real blocker.
- **Absolute paths** in every Read/Edit/Bash call; never rely on an earlier `cd`.
- **Re-read before writing** a file or artifact another session may have touched.
- **AskUserQuestion needs 2+ options**; with one, ask in plain text.
- **Engram conflicts:** resolve with one `mem_judge_many`, never a `mem_judge` loop.
<!-- /shared:agent-rules -->

<!-- gentle-ai:orchestrator-pointer -->
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
- Receipt-driven development (`gentle-ai review ...`) is user-owned and off by default;
  never enable it unasked and never fabricate a review approval.
- Remote operations (SSH, scp, remote hosts) need explicit user authorization for the
  destination, operation and credential. Never probe or reuse ambient SSH agents or
  sockets. Delegation cannot widen this scope.
- Technical artifacts (code, docs, commits) stay in English regardless of persona.
<!-- /gentle-ai:orchestrator-pointer -->

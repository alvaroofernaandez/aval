# plan-deck: every plan becomes a PDF deck before it runs

Why: aval must make the person understand what an agent is about to do. Before a
plan is executed, the agent turns it into a short, very visual slide deck (PDF),
rendered locally by aval with no vendor (no Claude Artifacts), in any project and
any agent runtime. The look, tone and story arc are fixed by aval, learned from the
context-relay deck iterations (2026-11-22).

## T1 — internal/deck: spec and validation
- [x] RED: JSON spec with a closed set of layouts; required fields per layout;
      limits that keep every slide inside its margins; clear errors with slide numbers
- [x] GREEN: spec.go

## T2 — internal/deck: HTML render (dark aval brand)
- [ ] RED: each layout renders its content; escaping; one page per slide at 1920x1080
- [ ] GREEN: render.go + embedded template and stylesheet

## T3 — internal/deck: browser lookup, overflow check, PDF print
- [ ] RED: lookup order (AVAL_CHROME, then OS paths), argv for print and dump-dom,
      overflow report parsing
- [ ] GREEN: chrome.go

## T4 — `aval deck build|check|example`
- [ ] RED: build writes the PDF and the spec beside it; --plan records the ack; usage errors
- [ ] GREEN: cmd/aval/deck.go

## T5 — plan gate
- [ ] RED: source edits denied while the active odd/tasks plan has no deck for its
      current headings; odd/ and deck specs always allowed; fails open; bypass env
- [ ] GREEN: `aval hook plan-gate` + Claude Code settings hook

## T6 — skill, rules and docs
- [ ] skills/plan-deck/SKILL.md (story arc, tone, layouts, checklist, where to save)
- [ ] global rule in config/claude/CLAUDE.md, mirrored with `aval rules sync`
- [ ] docs/guide.md and README

## T7 — live on this machine
- [ ] build and install the binary, `aval install --yes`, deck of this very plan as smoke test

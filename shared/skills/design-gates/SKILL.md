---
name: design-gates
description: >
  Runnable UI quality gates that print measured numbers and exit non-zero: WCAG contrast, tokens, hardcodes, a11y, render shots (shot.mjs). Run after any UI change, or for AA/AAA questions.
---

# Design gates

Vendored from plugin87/ux-ui-agent-skills at the commit in `pins/P87_PIN`. Regenerate
with `scripts/p87-sync.sh` — **never edit `scripts/`, `skills/` or this file by
hand**, the next sync overwrites them.

## The rule these gates exist to enforce

> Never state a number you did not measure. Any contrast ratio, "WCAG pass", or
> "100%" must come from running a gate and reporting its real output.

The pipeline these replace was five advisory skills that asked the model to
grade its own work, and a self-report cannot fail — which is how `impeccable`
silently aborted on every project for months without anyone noticing. A gate
that exits non-zero cannot be talked around.

## Static gates — no browser, no dependencies

Pure Python stdlib. Run from anywhere; paths are absolute.

| Gate | What it measures |
| ---- | ---------------- |
| `contrast.py` | WCAG 2.2 ratio for a colour pair; AA/AAA for normal, large, UI |
| `validate_tokens.py` | DTCG token files parse and resolve |
| `validate_contrast.py` | every foreground/background pair in a token set |
| `lint_hardcodes.py` | literal colours/sizes that should be token references |
| `validate_theme_refs.py` | theme keys pointing at nothing |
| `validate_component_spec.py` | a component definition is complete |
| `check_no_emoji.py` | emoji used as UI iconography |

```
python3 ~/.claude/skills/design-gates/scripts/contrast.py "#767676" "#ffffff"
```

Exit 0 passes, non-zero fails. Report the **printed number**, not your reading
of it.

## Render gates - need Playwright + Chromium

`shot.mjs` is the one to run before claiming a visual fix or a finished screen:

```
node ~/.claude/skills/design-gates/scripts/shot.mjs <url | file.html>
```

It screenshots 390/768/1440 px into `.design/shots/` (add `.design/` to the
project's `.gitignore`), runs the overflow audit and prints paths and numbers.
It exits 1 when it cannot render or when the layout overflows. Look at the PNGs.

The other render gates (`verify_states`, `verify_keyboard`, `verify_focustrap`,
`verify_target_size`, `verify_reduced_motion`, `verify_rtl`, `verify_responsive`,
`verify_overflow`, `axe_audit`, `measure_render`...) take a file or URL too.
Never claim a render gate ran when it did not.

## Reference

`accessibility/wcag-checklist.md` and `accessibility/aria-patterns.md` are
vendored alongside. `skills/<name>/SKILL.md` holds upstream's procedure for each
of `design-doctrine`, `design-review`, `design-qa`, `a11y-audit`,
`design-tokens`. Read the one matching the task, not all five.

## Reporting

```
[design-audit]
contrast     #1d1d1f on #ffffff → 17.4:1  AA PASS  AAA PASS
hardcodes    src/Button.tsx → 0 findings
tokens       tokens/core.json → valid, 48 resolved
render gates not run (Playwright not installed)
```

A gate you did not run is reported as not run. Never as a pass.

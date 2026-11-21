---
name: design-pipeline
description: >
  Blocking design pipeline and the DESIGN.md contract: laws, direction, gates, then `aval design ack <surface>`. Invoke before ANY visible-surface work (component, screen, layout, style, motion, UI copy) or DESIGN.md changes.
---

# Design pipeline

A UI edit is denied by the PreToolUse gate until `aval design ack <surface>` has
recorded this project (valid 4 h, and only while `.agents/DESIGN.md` is
unchanged). Do the three steps, then ack once.

1. **laws-of-ux** - which of the 30 Laws govern this surface and the number each
   forces: Hick caps option count, Fitts sets target size, Jakob decides whether
   a novel pattern is allowed. Constraints first; after the direction exists
   they get rationalised away.
2. **direction** - read `.agents/DESIGN.md`; it outranks taste and this skill.
   Cite the section that settles it. If it is open (new surface, redesign, no
   file), settle it with the user and write the winner back into DESIGN.md in
   the same change. Schema:
   `references/design-md-schema.md`.
3. **gates** - name the `aval design gate` checks that can fail for this surface
   (contrast for colour, hardcodes for a component, tokens for a token change).

Then run `aval design ack <surface>` (it prints this reminder) and write the code.

## After the code

Prove it with output, not reading:

- `node ~/.claude/skills/design-pipeline/scripts/shot.mjs <url|file.html>` renders
  390/768/1440 px into `.design/shots/` and prints paths and overflow numbers.
  Run it before claiming a visual fix or a finished screen. Add `.design/` to
  the project's `.gitignore`. A Stop hook asks for it once per UI turn.
- Run the gates that apply and report their real output. A gate not run is
  "not run", never a pass:

  ```bash
  aval design gate contrast <fg> <bg>            # WCAG ratio, AA/AAA
  aval design gate pairs tokens/colors.json      # semantic pairs, light and dark
  aval design gate hardcodes src/                # literals that should be tokens
  aval design gate theme-refs --theme theme.css src/
  aval design gate tokens tokens/                # DTCG aliases resolve
  aval design gate emoji src/                    # no emoji, no dashes in UI copy
  ```

## Rules

- Native interactive elements never: shadcn/ui + Radix (`design-refactor`).
- Laws of UX persuasion (Goal-Gradient, Zeigarnik, Von Restorff) help users
  finish their task; used against it they are dark patterns. Refuse those.
- No generic AI aesthetics. Motion needs a stated reason or it goes.

Bypass on purpose: `DESIGN_PIPELINE_OFF=1`.

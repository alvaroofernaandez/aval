---
triggers:
  - "**/*.{tsx,jsx,vue,svelte,html}"
---
Never use native interactive elements: no native date picker, raw `<select>`,
`<datalist>`, browser `<dialog>`, `confirm()` or `alert()`. Use shadcn/ui + Radix
(`components/ui/**` is exempt). Text, email, password, number, search inputs and
`<textarea>` stay native. Enforce with
`python3 ~/.claude/skills/design-refactor/scripts/lint_native_elements.py src/`,
which must exit 0; the `design-refactor` skill covers each replacement.

---
triggers:
  - "**/*.{go,ts,tsx,js,jsx,mjs,py,rs,vue,svelte}"
---
Strict TDD before this edit: write the failing test first (it fails on an
assertion, not an import error), then the minimum code, then refactor while
green. Emit before implementation code:

    [tdd-gate]
    Runner: <pytest | vitest | jest | go test | …>
    RED test: <path::name — what it asserts and why it must fail now>

If you cannot fill it honestly, do not write implementation code yet.

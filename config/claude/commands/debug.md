---
description: Systematic root-cause debugging from a log, stack trace, screenshot or issue - reproduce, hypothesise, prove, fix with a failing test first
argument-hint: "<log | error | issue URL | description>"
---
Debug `$ARGUMENTS` to its root cause. No guess-and-patch.

1. **Reproduce** it with the smallest command, request or test. No repro, no
   fix: say what is missing.
2. **Hypothesise**: list up to 3 causes ranked by evidence. Check the cheapest
   discriminating one first (logs, a print, a breakpoint, `git log -S`,
   `git bisect` when it used to work).
3. **Prove** the cause with evidence you show (output, line numbers). Change one
   thing at a time; revert experiments that did not confirm anything.
4. **Fix**: write the failing test that reproduces it (RED), then the minimal
   fix (GREEN). Search the repo for the same pattern and fix siblings too.
5. Report: cause, evidence, fix, test, and anything still unexplained.

---
description: Write tests for the current diff - detect the runner, cover changed behaviour and edge cases, report coverage gaps and flaky tests
argument-hint: "[path | --flaky <test>]"
---
Write tests for the behaviour changed in the current diff (or `$ARGUMENTS`).

1. Detect the runner from the repo (vitest, jest, pytest, go test, ...) and
   follow the existing test style and locations. No new framework.
2. From `git diff` (staged + unstaged vs the base branch) list the changed
   behaviours, then test each through its public interface: happy path, edge
   cases, error paths. Behaviour, not implementation details.
3. Run them. A test must be able to fail: break the code briefly to prove it.
4. With `--flaky <test>`: run it 20 times, report the failure rate and the
   cause (time, order, shared state, network) and fix the cause.
5. Report what is covered, what is not and why. Do not commit unless asked.

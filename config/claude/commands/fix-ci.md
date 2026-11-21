---
description: Fix the failing CI run of this branch - read the failed logs, reproduce locally, fix with a test, stop at green
argument-hint: "[run-id]"
---
Fix the failing CI of the current branch.

1. Find the run: `gh run list --branch "$(git branch --show-current)" --limit 5`,
   or use the run id in `$ARGUMENTS`. Read only the failure:
   `gh run view <id> --log-failed`.
2. Reproduce the failing job locally with the same command (read the workflow
   file for it). If it does not reproduce, say why (env, secrets, OS) and stop.
3. Fix the cause, not the symptom. A behaviour fix starts with a failing test
   (strict TDD); a config or flake fix explains why it is not a test.
4. Run the full local equivalent of the job, then commit (conventional, one
   work unit). Do not push unless asked; say which job should now pass.

Never disable, skip or loosen a check to get green. If the fix belongs in
another repo or needs a secret, stop and report.

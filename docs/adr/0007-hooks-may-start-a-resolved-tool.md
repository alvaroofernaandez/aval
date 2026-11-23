# 0007. Hooks may start a resolved tool under a timeout

Status: accepted (2026-11-23)

## Context

`AGENTS.md` says hooks and the statusline "are hot paths: pure work on their
input, no config read, no exec, no network". Every Edit, Write and Bash call
pays for each synchronous hook.

The rule already has one exception in the code: `aval hook rtk-rewrite`
(`internal/agent/rtkhook`) starts `rtk` to rewrite a Bash command, and only
when `rtk` is installed. Issue #25 asks for a hook that formats the file an
agent just edited, which cannot be done without starting the project's
formatter (`gofmt`, `biome`, `prettier`, `ruff`).

## Decision

A hook may start an external tool that it has already resolved (found on PATH
or in the project with `stat` calls only), bounded by a timeout, and never on
its no-op path. The no-op path stays pure: no process, no config read. No
config read and no network stay forbidden for every hook.

`aval hook fmt` (`internal/agent/fmthook`) is the first hook built under this
rule: an edit with no known extension, no formatter or outside a repository
starts nothing; a resolved formatter runs with a 3 s timeout, and a failure is
one warning line on stderr that never fails the edit.

## Rejected alternatives

- **Keep "no exec" absolute.** It rules out formatting after an edit, and it
  already does not describe `rtk-rewrite`.
- **Format in-process.** Only Go has a formatter in the standard library; the
  project's own formatter and its configuration are the source of truth.
- **Run the formatter asynchronously.** The agent's next read would race the
  rewrite; a synchronous hook finishes before the agent continues.

## Consequences

- The no-op cost is measured by `aval budget` against its 20 ms budget and by
  `BenchmarkRunNoFormatter`; `TestRunWithoutAFormatterIsSilentAndStartsNothing`
  pins that nothing starts on that path.
- A hook that starts a process documents its timeout and its no-op path.
- `AGENTS.md` carries a dated amendment that points here.

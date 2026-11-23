# Field notes behind aval-testing

Background for the rules in `../SKILL.md`. Read only when a rule needs its why.

## Where the rules come from in aval

| Rule | Mechanism or record |
| --- | --- |
| Temp dirs through `t.TempDir()`, no fixed sleeps | `tests/test_hygiene_test.go` (S5) |
| Assert errors with `errors.Is`/`errors.As` | `AGENTS.md` hard rules; S7: sentinels and typed errors where tests needed them, `TestHygieneNoErrorTextComparisons`, user text pinned once in `tests/messages_test.go` |
| One real-binary smoke test per subcommand | S6, the in-process `run` seam; smokes in `tests/cli_smoke_test.go` |
| Every test package runs in CI on three OSes | ADR 0003, `tests/workflows_test.go` |
| Layering is a test, not a review comment | ADR 0002, `tests/architecture_test.go` |
| Guards prove they catch what they claim | planted-violation self-tests (architecture), `AVAL_TEST_REPO` mutated copies |
| Opening the hub writes nothing | ADR 0004, `cmd/aval/hub_test.go` hashes HOME and the cache and fails on any HTTP request |

## Principles

- **Behavioural and structure-insensitive.** A test that breaks on a refactor that kept behaviour is a change detector; asserting error text is the commonest one.
- **Deterministic.** Shared state (HOME, env, files, ports) and timing waits cause most flakes; a flake is a finding.
- **Fast, at the lowest level.** Spawning the binary per case is slow; in process with one smoke test per entry point keeps OS startup covered.
- **Fakes over mocks.** A fake with real semantics survives refactors; an interaction mock pins call order.
- **Mutation testing on demand.** Coverage shows lines ran, not that anything was asserted; a mandatory gate costs too much per change.

## Sources

- Kent Beck, Test Desiderata: behavioural, structure-insensitive, deterministic, fast, isolated, specific.
- Martin Fowler and Ham Vocke, The Practical Test Pyramid.
- Titus Winters, Tom Manshreck, Hyrum Wright (eds.), Software Engineering at Google, chapters 11 to 13: test sizes, change-detector tests, fakes over mocks.
- Luo et al., An Empirical Analysis of Flaky Tests (FSE 2014): async waits, concurrency, test order dependency.
- The hagaharness testing skill, which these rules were ported from and adapted to Go.

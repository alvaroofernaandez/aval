# Engineering standards: hagaharness's way of building, testing and deciding, in aval

Status: approved by the owner on 2026-11-23. Decision: the architecture gate is a Go test; archsieve is an optional report tool, not a dependency and not a gate.

Why: aval has strict TDD and good repo-level guards, but several things are missing:
- No project `AGENTS.md`, no project skills, no ADRs and no architecture model.
- No boundary check at all. The layering is clean today, but nothing enforces it.
- Some tests assert error text instead of the error.
- Some tests sleep.
- Some tests create temp dirs by hand.
- Many tests spawn the real binary, because `run()` takes no args or writers.

hagaharness solves each of these with a mechanism, not a habit. This plan ports those mechanisms, adapted to Go, so the standards hold for future work by people and agents alike.

The owner asked for this to apply both to developing aval and to using aval. So the generic parts also ship to aval users, through the distributed skills (S9).

Source: a digest of hagaharness (`AGENTS.md`, `.claude/skills/*`, `scripts/arch-check.mjs`, `gate.config.mjs`, `tests/arch-check.test.mjs`, `tests/layer-kit-boundary.test.mjs`, `docs/adr/`, `docs/arquitectura/modelo.c4`, `docs/research/dx-gate-agentes.md`).

## Principles carried over

1. **Mechanisms, not review.** Every rule that matters has a test that fails when it breaks. Each such test also has a planted-violation self-test, which proves the guard catches what it claims to.
2. **Evidence before abstractions.**
   - A new package, interface or seam is a visible cost. Justify it with a concrete need: a second implementation, a test seam, or a security or ownership boundary.
   - Keep AS-IS (what exists) and TO-BE (the proposal) separate.
   - Existing findings are the baseline. Only new violations block.
3. **Testing at the lowest level that catches the regression.**
   - One RED test per rule, failing on an assertion.
   - Assert errors with `errors.Is` or `errors.As`, never their text.
   - Pin user-facing text in exactly one place.
   - No fixed sleeps. Temp dirs only through `t.TempDir()`.
   - One real-binary smoke test per entry point; everything else runs in process.
   - Report only what ran and what was observed. Skipped is not passed.
4. **Decisions are recorded, never rewritten.** An ADR has context, decision, rejected alternatives and consequences. A changed decision gets a dated amendment or a new ADR.
5. **The project skill wins.** Where a project skill disagrees with a global or personal one, the project skill applies.

Not carried over:
- **No hosted CI.** aval keeps its hardened three-OS GitHub Actions, guarded by `workflows_test.go`.
- **Node tooling as a dependency.** "Go only" stays.
- **Spanish message catalogs.** aval is in English.
- **Running tests in pre-push.** aval's pre-push scans outgoing commits by design (ADR in S4).

## Tasks

- [ ] **S1 (opus): project `AGENTS.md` and `CLAUDE.md`.**
  - `CLAUDE.md` is a single line, `@AGENTS.md`.
  - `AGENTS.md` holds:
    - "the project skill wins";
    - a task-to-skill table;
    - the hard rules: Go only, strict TDD, conventional commits with no AI attribution, plan and deck before execution, the design pipeline for visible surfaces, never push unasked;
    - the commands: `go test ./...`, `go vet ./...`, `gofmt -l .` and the architecture test;
    - the output contract: what ran, what did not, and evidence.
  - RED: `context_budget_test.go` gains ceilings for both files, and a test requires the task-to-skill table to name every project skill that exists.
- [ ] **S2 (opus): `tests/architecture_test.go` and the generated model.**
  - Run `go list -f '{{.ImportPath}}|{{join .Imports ","}}|{{join .TestImports ","}}|{{join .XTestImports ","}}' ./...` and check every internal edge against a declared allowlist:
    - Leaves import nothing internal: `internal/agent/*`, `devops/*`, `term/*`, `deck`, `design/{ack,gate}`, `release`, `docs/*` and `devmode`.
    - `core/*` imports only `core/*` and `release`.
    - `ui` imports only `core/*`, `devmode`, `release` and `ui/caps`.
    - `ui/cli` imports only `ui` and `ui/caps`.
    - Nothing imports `cmd/aval`.
    - `internal/testenv` appears only in test imports.
  - Ratchet: any counted exception may shrink, never grow.
  - Self-test: a planted violation in a temp module is reported as `file -> import`.
  - `docs/architecture/model.md` holds a Mermaid component diagram generated from the same data. A test fails when the committed diagram is stale.
- [ ] **S3 (opus): project skills.** Create `.claude/skills/aval-architecture`, `aval-testing` (with `references/aval.md` and `references/field-notes.md`), `aval-docs` and `aval-release`.
  - `aval-architecture`:
    - evidence before abstractions, AS-IS and TO-BE, the allowlist and how to change it (ADR first);
    - optional archsieve reports via `npx` at a pinned version (diagrams, `affected`, `--history` churn), never a gate.
  - `aval-testing` carries the full checklist:
    - levels, assertions, determinism, the flake protocol;
    - the differential check with `git worktree`;
    - the output contract.
  - A test checks every skill's frontmatter and that `AGENTS.md` lists it.
- [ ] **S4 (sonnet): `docs/adr/`.**
  - A README with the template and the never-rewrite rule.
  - Seed ADRs:
    - 0001: Go only.
    - 0002: layering of `internal/` (the S2 allowlist).
    - 0003: pre-push scans while CI tests.
    - 0004: the hub writes nothing and stays offline.
    - 0005: the relay is cost-aware (from `odd/tasks/relay-cost-model.md`).
  - A test checks the template sections and the README index.
- [ ] **S5 (sonnet): test hygiene.**
  - `tests/test_hygiene_test.go` fails on `os.MkdirTemp`, `ioutil.TempDir` and `time.Sleep` in `_test.go` files, outside an exemption map that gives a reason for each entry.
  - Fix today's offenders: 3 `MkdirTemp` files and 8 `time.Sleep` files. Use `t.TempDir()`, an injected clock, or polling with a deadline.
  - The exemption map ends empty, or holds only reasons that are justified.
- [ ] **S6 (opus): in-process command seam.**
  - `run(args []string, stdin io.Reader, stdout, stderr io.Writer, env func(string) string) int` in `cmd/aval`, with `main` as a thin wrapper.
  - Move binary-spawning tests to the seam, except one real-binary smoke test per subcommand.
  - RED: an in-process test of a subcommand that today only has a subprocess test.
- [ ] **S7 (sonnet): errors as values.**
  - Introduce sentinel or typed errors where the 44 `err.Error()` comparisons point, and switch those tests to `errors.Is` or `errors.As`.
  - `tests/messages_test.go` becomes the single place that pins user-facing text.
  - A hygiene rule (S5) blocks new `err.Error()` comparisons outside it.
- [ ] **S8 (sonnet): every test runs in CI.** Extend `workflows_test.go`: every package with tests runs under `go test ./...` in `ci.yml`, and every build-tagged e2e directory has a CI step or a stated exemption.
- [ ] **S9 (sonnet): ship the generic standards to aval users.**
  - Update the distributed `skills/strict-tdd` with the testing checklist, without the aval-specific parts.
  - Add a distributed `skills/architecture-evidence`: evidence before abstractions, AS-IS and TO-BE, and boundary tests as the mechanism, generic per language with a Go example.
  - Both go through the manifest, so `aval install` links them.

## Waves

| Wave | Tasks |
| :--- | :--- |
| 1 | S1, S2, S4, S5 |
| 2 | S3 (it needs S2's allowlist and S4's ADRs), S6, S8 |
| 3 | S7 (it needs S6's seam for in-process assertions), S9 |

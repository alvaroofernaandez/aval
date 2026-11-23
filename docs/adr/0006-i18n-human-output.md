# 0006. i18n: English and Spanish for human output, machine output stays English

Status: accepted (2026-11-23)

## Context

`odd/tasks/i18n.md`, approved by the owner on 2026-11-23, asks for the TUI and
the human-facing CLI in English and Spanish. Until then every string was an
English literal in `internal/ui`, `internal/ui/cli` and `cmd/aval`; S7 of
`odd/tasks/engineering-standards.md` moved command errors to values and pinned
the text a person reads in one place, `tests/messages_test.go`.

Some output is read by programs, not people: `--json`, hook output and deny
reasons, `statusline`, error codes and sentinel identities, log files and
generated files. Hooks and the statusline are hot paths that never load config
(`AGENTS.md`, `TestHotPathsNeverReadConfig` in `cmd/aval/ui_hotpaths_test.go`).

`golang.org/x/text` is already in the module graph, but only indirectly, at
v0.3.8, through `bubbletea` and `go-localereader` (`go mod why -m
golang.org/x/text`). ADR 0001 keeps the dependency budget small.

## Decision

- `internal/i18n` is a leaf package (ADR 0002, amendment of 2026-11-23). It
  holds one table per language, keyed by stable dotted keys. English is the
  source of truth and Spanish covers every key.
- Messages use named placeholders (`{path}`, `{count}`) and `one`/`other`
  plural forms, which is all English and Spanish need. The formatter is a
  standard library string replacement; a key missing at run time falls back to
  English.
- Tests keep the catalog honest: every key in both languages, the same
  placeholders in both, both plural forms for a plural key, and no key that Go
  code never looks up (an AST scan of the literals passed to `T` and `N`).
- The language comes from `--lang`, then `AVAL_LANG`, then the `language`
  config key (`auto`, `en`, `es`), then detection from `LC_ALL`, `LC_MESSAGES`
  and `LANG` (`es` or `es_*` is Spanish), then English.
- Only human output is translated. Machine-facing output stays English, and the
  hot paths never resolve a language at all.

## Rejected alternatives

- **`golang.org/x/text/message` with `gotext`.** It would turn an indirect
  dependency into a direct one and add a code generation step for two
  languages whose plural rules are `n == 1` versus the rest. A map and a
  replacer do the same job with nothing to generate.
- **Translating docs and the distributed skills.** Agents and contributors read
  them; two copies drift, and the plan keeps them English.
- **Translating `--json`.** Scripts parse it. Field names, values and messages
  in it are a contract and stay English, whatever the language.

## Consequences

- A new human string is a catalog key in both languages, or a test fails.
- Spanish runs longer, so the TUI width and height invariants must hold in both
  languages (tasks I2 and I3).
- The English output stays byte-stable: `tests/messages_test.go` pins it.
- A wrong `language` value warns, names the valid values with a suggestion, and
  falls back to detection; a wrong `--lang` is an error, like `--theme`.

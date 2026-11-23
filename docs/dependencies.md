# Dependency budget

Every direct dependency is future maintenance: updates, security advisories and breakage. This page is where admitting one is decided. Each row says why the dependency exists and what was rejected instead.

`tests/dependencies_test.go` compares these tables with `go.mod` (through the imports of every `.go` file, tests included), `npm/cli/package.json` and `config/opencode/package.json`. It fails when a direct dependency has no row, when a row outlives its dependency, or when a row has no reason. It also fails when `go.mod` marks a directly imported module `// indirect`; `go mod tidy` fixes that.

To add a dependency, add its row with its reason in the same PR. Indirect Go modules are not listed: they come with a direct one.

## Go module

| Dependency | Version | Reason | Instead of |
| :-- | :-- | :-- | :-- |
| `github.com/charmbracelet/bubbletea` | v1.3.10 | The event loop and model-update-view cycle of the hub (`internal/ui`). | A hand-written raw-mode loop, which would reimplement input decoding and resize handling. |
| `github.com/charmbracelet/lipgloss` | v1.1.0 | Styling, borders and width-aware layout of every hub screen. | Raw ANSI escapes, which break on wide characters and themes. |
| `github.com/muesli/termenv` | v0.16.0 | The colour profile (true colour, 256, 16, none) that `internal/ui/caps` detects and the themes degrade to. | Reading `COLORTERM` and `TERM` by hand, which misses the common terminals' quirks. |
| `github.com/charmbracelet/x/term` | v0.2.2 | `IsTerminal` and `GetSize` in `cmd/aval`, to decide between the hub and plain output and to size the hub. | `golang.org/x/term`, an equivalent module; this one already comes with bubbletea. |
| `github.com/mattn/go-runewidth` | v0.0.19 | Test only: `internal/ui/screen_hub_v2_test.go` checks that the hub uses no ambiguous-width or wide characters. | A hand-made width table, which would drift from what lipgloss measures. |

## npm package (npm/cli)

The npm wrapper has no third-party dependencies and uses only Node's standard library. At publish time `npm/build.mjs` adds aval's own per-platform binary packages as `optionalDependencies`; they are not in the source `package.json`, so they are not listed here.

## OpenCode plugins (config/opencode)

| Dependency | Version | Reason | Instead of |
| :-- | :-- | :-- | :-- |
| `@opencode-ai/plugin` | 1.2.20 | The `Plugin` type that `plugin/design-gate.ts` and `plugin/terax-hooks.ts` are written against. | Untyped plugins, which would break silently when the OpenCode API changes. |

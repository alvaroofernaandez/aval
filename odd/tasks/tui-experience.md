# TUI experience: brand, first run, views and configuration

Status: draft, waiting on owner decisions (section 6).

All paths are relative to the repository root.

Why: `aval install` is the first thing a person sees. Today it is a three-screen Tokyo Night list with no first-run path, no configuration and no brand beyond the word "aval". This plan makes the installer and the CLI output one designed surface: on-brand, readable in any terminal, and configurable from a file or from the TUI.

Scope: the installer TUI, first run, a config model, and styled CLI output for `help`, `doctor` and `install --yes`. Not the v0.7.0 "aval TUI" multiplexer.

## 1. Current-state audit

### Aesthetic and theme
- **Palette contradicts the contract.** `internal/ui/theme.go:12-27` hardcodes Tokyo Night. `.agents/DESIGN.md:24-26` makes Kanagawa the source of truth, and `:119` says the brand is "pure ink". DESIGN.md (`:3-5`) only covers the status bar, so the TUI has no contract.
- **Contrast fails.** `Muted` (`#565f89` on `#1a1b26`) is 2.76:1. `Unchecked`, `Unavailable` and `BarEmpty` (`#414868`) are 1.91:1. DESIGN.md `:96` requires 4.5:1.
- **No capability handling.** There is no light-background variant and no tested `NO_COLOR` or 256-colour path. The cursor row (`theme.go:64-67`) assumes a dark terminal.
- **Glyphs have no fallback.** `▸`, `█░`, `↑/↓` and `—` are emitted unconditionally (`model.go:239`, `theme.go:105-107`, `model.go:261`, `:312`).
- **Branding is one bold word** (`model.go:206`). `Theme.Box` (`theme.go:76-79`) is never used.

### Onboarding and first run
- **The npm path dead-ends.** `npx @alvaroofernaandez/aval install` without a clone fails in `resolveRepo` (`cmd/aval/main.go:176`, `internal/core/repoloc/repoloc.go:68`) before any UI appears.
- **Bug: quitting still writes to HOME.** `runInstall` (`main.go:331-341`) calls `finishInstall` without checking whether the user quit. Pressing `q` on the select screen still seeds settings, runs sync, records the repo and sets `core.hooksPath` (`cmd/aval/core_seed.go:20-29`).
- **No TTY guard.** Without `--yes`, a pipe or CI gets Bubble Tea's raw error.
- **Flat usage.** All 23 commands get the same weight (`cmd/aval/core_usage.go:19-43`).
- **No "what next"** on the done screen (`model.go:337`).

### Installation
- **Three sound routes:** npm optional-dependency packages, GoReleaser archives, and `go install`. The gap is the clone, `cd`, `aval install` step that follows, which is explained only in prose.
- **Stale docs:** `docs/guide.md:83` and `:380`, and `docs/installer-demo.tape`, still name Tokyo Night.

### Views
- **Select:**
  - It does not say what will happen to each file before writing.
  - Columns are fixed at `%-22s`, and the help line never wraps.
  - Enter with nothing selected does nothing.
  - The unavailable reason is Windows-only text (`main.go:301`).
- **Install:** failures are invisible until the end, `%-40s` overflows narrow terminals, and `q` gives no feedback.
- **Height is never tracked** (`model.go:94-96`).
- **CLI output is unstyled** (`internal/core/doctor/doctor.go:122-147`, `main.go:354-385`).

### Configuration
- **One key in one file:** `repo` in `${XDG_CONFIG_HOME:-~/.config}/aval/config` (`repoloc.go:98-175`). There is no theme, no `aval config` command and no UI environment variables.
- **go.mod is untidy.** Every requirement is marked `// indirect`, and `bubbles` is unused.

### Tests
- `internal/ui/model_test.go` has 13 substring tests. There are no goldens, no width tests and no colour-profile coverage.

## 2. Target experience

### Journey
1. **Get the binary:** npm, a release archive or `go install`. Unchanged.
2. **Bare `aval`:** shows branded, grouped usage and writes nothing. When no config exists it ends with `Not set up yet. Run: aval install`.
3. **`aval install` with no repository found:** a welcome screen offers a checkout found in the working directory, a typed path, or cloning to `~/.aval`. Cloning is always an explicit choice.
4. **Select:** each group shows its per-destination state before anything is written.
5. **Install:** failures appear inline, and `ctrl+c` stops after the current step.
6. **Done:** counts, the backup path, the undo command and three next steps.
7. **Later runs:** `aval config` opens settings.

Non-interactive paths never open a TUI, and their output stays byte-identical when stdout is not a terminal.

### Welcome
```
  aval  0.2.0                                              macOS (arm64)
  Nothing passes without evidence.
  ────────────────────────────────────────────────────────────────────────

  Welcome. aval links its configs into your home directory.
  Nothing is deleted: anything in the way moves to ~/.aval-backup first.

  Where is the aval repository?

  › ( ) Clone it to ~/.aval            needs git and a network connection
    ( ) Use a folder I already have    type or paste a path

  ────────────────────────────────────────────────────────────────────────
  step 1 of 3 · repository
  ↑↓ move · enter choose · ? help · q quit
```

### Select
```
  Choose what to install               symlinked: repo edits apply at once

  › [✓] Terminal environment   11 items   8 new · 3 linked
        tmux, yazi, Ghostty, zsh and Powerlevel10k
    [✓] Git and GitHub CLI      3 items   3 linked
    [ ] Claude Code             7 items   6 new · 1 replaces a file
    [✓] OpenCode                5 items   5 new
    [✓] Agent skills           31 items   31 new

  ────────────────────────────────────────────────────────────────────────
  50 of 57 items selected · 44 new · 6 linked · 0 files moved to backup
  ↑↓ move · space toggle · → details · a all · n none · enter install · ? help
```

### Install
```
  Installing                                                     23 of 50
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━──────────────────────────────────────
    ✓ linked     ~/.config/yazi
    ! backed up  ~/.zshrc
    ✗ failed     ~/.config/ghostty   source missing
    › linking    ~/.zprofile
  ────────────────────────────────────────────────────────────────────────
  1 failed so far · ctrl+c stops after the current step
```

### Done
```
  Installed. 49 linked, 1 failed.

    linked            49
    already in place   6
    failed             1   ~/.config/ghostty: source missing

  1 existing file moved to ~/.aval-backup/2026-11-23T10-14-02
  Undo everything with: aval restore latest

  Next
    aval doctor     check this machine against the repo
    aval config     theme, glyphs and update checks
    exec zsh        reload your shell to pick up the new config
```

### Settings
```
  Settings                                        ~/.config/aval/config

  › Theme          aval           aval · catppuccin · tokyo-night · gruvbox …
    Background     auto           auto · dark · light
    Glyphs         auto           auto · unicode · ascii
    Motion         auto           auto · off
    Update check   on             on · off
    Repository     ~/.aval

  Preview   [✓] Selected   [ ] Not selected   ✓ linked   ! backed up   ✗ failed
  ────────────────────────────────────────────────────────────────────────
  theme is set by $AVAL_THEME in this shell; the file value applies elsewhere
  ↑↓ move · ←→ change · enter save · esc discard
```

### Compact (under 40 columns, `NO_COLOR`, ASCII)
```
 aval 0.2.0
 Choose what to install
 > [x] Terminal enviro... 11
   [x] Git and GitHub CLI  3
   [ ] Claude Code         7
 50/57 selected
 space toggle  enter install
```

## 3. Design system

### Principles
- The terminal belongs to the user: paint no canvas.
- Colour only speeds up locating a state. Every state also has a glyph and a word.
- No emoji, no Nerd Font glyphs, no powerline separators.
- Every fg/bg pair is verified at 4.5:1 or better.

### Colour tokens

Owner decision (2026-11-23): the default theme is `aval`, built from the brand colours in DESIGN.md section 9. The widely used community terminal themes are available as opt-in themes.

Every theme defines the same tokens: `text`, `strong`, `muted`, `rule`, `ok`, `warn`, `fail`, `key`, `cursor`.

| Token | `aval` dark | `aval` light | `terminal` | `mono` / `NO_COLOR` |
| :--- | :--- | :--- | :--- | :--- |
| text | `#ffffff` | `#000000` | terminal default | default |
| strong | `#ffffff` bold | `#000000` bold | bold | bold |
| muted | `#a3a3a3` (8.33:1) | `#525252` (7.81:1) | default, never faint | default |
| rule (decorative) | `#2e2e2e` | `#d4d4d4` | faint | faint |
| ok | `#98BB6C` springGreen | darker green, chosen in T4 | ANSI 2 | none |
| warn | `#E6C384` carpYellow | darker yellow, chosen in T4 | ANSI 3 | none |
| fail | `#C34043` autumnRed + bold | autumnRed + bold | ANSI 1 + bold | bold |
| key | bold ink | bold ink | bold | bold |
| cursor row | reverse video | reverse video | reverse video | reverse |

- **Brand rule:** text and chrome are pure ink; the only colour is state (ok, warn, fail), the same green and yellow the plan decks use. No accent colour and no background canvas.
- **`aval` light/dark** follows the `background` setting. `auto` asks the terminal and falls back to dark.
- **Contrast:** light-variant state colours are picked in T4 so that every pair reaches 4.5:1 on white. Every pair is measured with `aval design gate contrast`.
- **`terminal`** uses only the 16 ANSI colours, so it inherits the user's own terminal theme.
- **`NO_COLOR`** wins over everything.

### Community themes

These are opt-in. Each ships with its published dark and light variants where upstream has them:

| Theme | Variants |
| :--- | :--- |
| `catppuccin` | mocha (dark), latte (light); `frappe` and `macchiato` as their own names |
| `tokyo-night` | night, storm, day (light) |
| `gruvbox` | dark, light |
| `dracula` | dark (light: Alucard) |
| `nord` | dark |
| `kanagawa` | wave, dragon, lotus (light) |
| `rose-pine` | main, moon, dawn (light) |
| `solarized` | dark, light |
| `one-dark` | dark (light: One Light) |
| `everforest` | dark, light |

- **Mapping:** each theme maps upstream palette entries to the tokens, for example `ok` = green, `warn` = yellow, `fail` = red, `muted` = the palette's subtext/comment colour.
- **Contrast:** a token that misses 4.5:1 on the palette's own background is replaced by the closest palette entry that passes. The swap is recorded in the theme's test.
- **Source:** palette hex values come only from the upstream repositories, with each source URL and licence in `internal/ui/themes/SOURCES.md`. All ten are MIT-licensed upstream; T4b must verify this.
- **Variants:** a theme name follows `background`, and a variant can be pinned explicitly, for example `theme = catppuccin-frappe`.

### Capabilities
One `Caps` value, computed once:
- **Colour profile:** termenv detection.
- **Background:** the `auto`, `dark` or `light` setting.
- **Glyphs:** unicode on Unix with a UTF-8 locale and `TERM` not `linux` or `dumb`. On Windows, unicode only with `WT_SESSION`, `TERM_PROGRAM` or `ConEmuANSI`. ASCII otherwise.
- **Motion:** off when not a TTY or when `motion = off`.
- **Size:** width and height.

### Glyphs
| Role | Unicode | ASCII |
| :--- | :--- | :--- |
| cursor | `›` | `>` |
| check states | `[✓]` `[ ]` `[-]` | `[x]` `[ ]` `[-]` |
| ok / warn / fail | `✓` `!` `✗` | `ok` `!` `x` |
| progress | `━` `─` | `#` `-` |
| separator, ellipsis | `·` `…` | two spaces, `...` |
| keys | `↑↓` `←→` | `up/down` `left/right` |

### Layout
- **Margins:** 2 columns, 1 when compact. No outer border.
- **Frame:** header, rule, body, rule, status line, key hints.
- **Width:** content caps at 76 columns.
- **Breakpoints:** compact under 40, narrow 40 to 79, regular 80 and up.
- **Invariants:** no line wider than the terminal, and the frame never taller than it. Lists scroll with the cursor visible.
- **Truncation:** labels end with an ellipsis; paths truncate in the middle.

### Brand
- **Lockup:** `aval` in bold, the version muted, the platform right-aligned, and the tagline on line two. The tagline is dropped when compact.
- **Large wordmark:** on the welcome screen only, and only if it passes the T20 screenshot check.
- **Tone:**
  - Sentence case, verb first, no exclamation marks.
  - Report evidence, never "Success".
  - Every error says what, where and the command that fixes it.

### Motion
One spinner, for network waits only. `motion = off` replaces it with static text.

## 4. Config model

- **File:** same path and `key = value` format as today, so there is no migration. Writes are atomic and preserve comments and unknown lines.

| Key | Values | Default | Env | Flag |
| :--- | :--- | :--- | :--- | :--- |
| repo | path | none | AVAL_REPO | --repo |
| theme | aval, terminal, mono, or a community theme or variant (section 3) | aval | AVAL_THEME | --theme |
| background | auto, dark, light | auto | AVAL_BACKGROUND | |
| glyphs | auto, unicode, ascii | auto | AVAL_GLYPHS | |
| motion | auto, off | auto | AVAL_MOTION | |
| update_check | on, off | on | AVAL_NO_UPDATE_CHECK | |

- **Precedence:** flag, then env, then file, then default. `--plain` means `mono`, ASCII glyphs and motion off. `NO_COLOR` sits above all of it.
- **Commands:**
  - `aval config` opens the TUI on a terminal and runs `list` otherwise.
  - `aval config list` shows each value and its source.
  - `aval config get|set|path`.
- **Validation:** a bad UI value warns once on stderr (with file, line and a suggestion) and falls back to the default. `set` refuses invalid values with exit 1. A bad `repo` stays a hard error.
- **Hooks, `statusline` and `tmux`** never read the UI keys.

## 5. Tasks

Rules for every task:
- Strict TDD.
- `go vet`, `gofmt -l` and `go test ./...` pass on all three OSes.
- One conventional commit per task.
- Testing:
  - Update tests drive the model with key messages.
  - View goldens live in `internal/ui/testdata/*.golden` and render with a fixed-profile renderer; `-update` rewrites them.
  - Invariant tests cover widths 20 to 120 and heights 8 to 40.
  - No teatest.

### Phase 0: contract and foundations
- [ ] **T1 (opus): DESIGN.md amendment.** Add the installer and CLI surfaces, the tokens, glyph tiers, breakpoints, lockup and motion rule, and a decision-log entry. Fix the stale pointers at `:125-126`.
- [ ] **T2 (sonnet): `internal/ui/caps`.** Pure `Detect(Inputs) Caps` with table tests over injected env and GOOS. Run `go mod tidy`.
- [ ] **T3 (sonnet): `internal/core/config`.** Schema, validation, precedence and round-trip preservation. `repoloc` delegates to it, and `repoloc_test.go` passes untouched.
- [ ] **T4 (opus, after T1 and T2): theme engine, `aval`, `terminal` and `mono`, plus glyph sets.**
  - Contrast tests use `internal/design/gate` Ratio.
  - `aval` matches the DESIGN.md section 9 brand hexes for dark and light.
  - `terminal` sets no hex colours.
  - The ASCII glyph set is pure ASCII.
  - The theme registry and variant resolution (name plus background, or a pinned variant) live here.
- [ ] **T4b (sonnet, after T4): community themes.**
  - The ten themes from section 3, each with a table test that pins its upstream hexes and checks 4.5:1 for every token.
  - `internal/ui/themes/SOURCES.md` with each upstream URL and licence.
  - An unknown theme name suggests the closest match.
- [ ] **T5 (sonnet, after T4): golden harness and width/height invariants.** RED: the width invariant fails today at width 30.

### Phase 1: views
- [ ] **T6 (opus, after T5): frame, lockup, breakpoints and key map.** Split `model.go` into one file per screen.
- [ ] **T7 (sonnet, independent): `install.Preview`.** Classify each destination as new, linked or replaces, without touching the filesystem.
- [ ] **T8 (opus, after T6 and T7): select screen.** State counts, `→` details, the empty-selection message, and a caller-provided unavailable reason.
- [ ] **T9 (opus, after T6): install screen.** Inline failures and a graceful stop that still commits the backup run (prove it with a restore test).
- [ ] **T10 (sonnet, after T6): done screen.** Counts headline, undo line, next steps, and dry-run and stopped wording.
- [ ] **T11 (sonnet, after T6): `?` help overlay** generated from the key map.

### Phase 2: first run
- [ ] **T12 (sonnet, after T6): quit safety and TTY guard.** Gate `finishInstall` on `Completed()`. Without a terminal and without `--yes`, print a clear error.
- [ ] **T13 (opus, after T6 and T12): welcome and repository step.** An injected `Cloner`, inline errors, and the repo recorded only after a completed install.
- [ ] **T14 (sonnet, after T4): branded, grouped usage** with the first-run hint.
- [ ] **T15 (sonnet, after T4): styled CLI printer** for `doctor` and `--yes`, byte-identical when not on a TTY.

### Phase 3: config
- [ ] **T16 (sonnet, after T3): `aval config list|get|set|path`.**
- [ ] **T17 (opus, after T3, T4 and T6): settings view** with live preview. Enter saves, `esc` discards, and env overrides are noted.
- [ ] **T18 (sonnet, after T3, T4 and T12): wire the settings into every surface.** Add a `doctor` config check.

### Phase 4: ship
- [ ] **T19 (sonnet): docs and demo.** README, guide, npm README, and re-recording the GIF with vhs.
- [ ] **T20 (opus): cross-terminal verification.**
  - Ghostty dark and light, Terminal.app, Windows Terminal, conhost, the Linux console, `NO_COLOR` and a 36x10 pane.
  - Decide the large wordmark from the screenshots.

### Waves
| Wave | Tasks |
| :--- | :--- |
| 1 | T1, T2, T3, T7 |
| 2 | T4, T16, then T4b (parallel with wave 3) |
| 3 | T5, T14, T15 |
| 4 | T6 |
| 5 | T8, T9, T10, T11, T12, T17 |
| 6 | T13, then T18 (both edit `main.go`) |
| 7 | T19, then T20 |

If the release must shrink, cut T13 (keep a better error message), then T15, then T17 (keep `aval config set`).

## 6. Open questions

1. ~~Default theme~~ decided 2026-11-23: `aval` brand colours by default, community themes as opt-in.
2. **DESIGN.md:** may this surface use unicode glyphs and one spinner, or must it stay ASCII-only and motionless like the status bar?
3. **Version:** v0.2.0, with "Maintainability" moved elsewhere?
4. **Cloning:** should the installer offer to clone to `~/.aval`?
5. **Language:** keep the TUI in English with "Nothing passes without evidence."?

### Risks
- **Background detection** is unreliable in tmux and on Windows. `background = dark|light` overrides it.
- **Goldens break on Charm upgrades.** Pin the versions and regenerate goldens in their own commit.
- **The graceful stop replaces `TestQuitIsRefusedMidInstall`.** It must keep `aval restore latest` working.
- **`main.go` is a hot file.** The wave table runs T12, T13 and T18 in sequence.

## 7. Later
- A hub screen for bare `aval`.
- User theme files.
- An interactive doctor.
- A TUI for `restore` and `uninstall`.
- A Homebrew tap and a one-line install script.
- Spanish copy.
- Mouse support.
- A high-contrast theme and a screen-reader mode.

# TUI hub: bare `aval` opens a home screen, one view per action

Status: decisions taken (section 7); ready to execute.

All paths are relative to the repository root.

Why: the installer is a designed surface now, but every other command is still text you have to know by name. The owner wants gentle-ai's structure with aval's aesthetic: bare `aval` opens a hub with a large logo, a status panel and a menu of every action, each with its own view.

Scope:
- The hub and a router.
- Seven new views: update, sync, doctor, backups, uninstall, dev sandbox and about.
- The installer and settings, hosted inside the hub.
- The core data functions the views need.

Out of scope: the v0.7.0 multiplexer, and any change to non-interactive output.

## 1. Audit

### Commands
| Command | Today | Data the view can call | Gap |
| :--- | :--- | :--- | :--- |
| bare `aval` | Usage text (`cmd/aval/main.go:91-97`, `core_usage.go:79-118`) | none | No hub. `settingsForCommand` already accepts `""` (`ui_settings.go:94`) |
| `install` | Full TUI: welcome, select, install, done | `ui.NewModel`, `ui.NewWelcome`, `install.Runner` | `finishInstall` runs only after the program ends (`main.go:390-408`) and warns on stderr (`core_seed.go:78`) |
| `config` | Settings view on a TTY, text otherwise | `ui.NewSettings` | Standalone mode ends the program (`screen_settings.go:289-296`) |
| `update` | Text only (`main.go:584-637`); dry run is `previewUpdate` | `update.Check`, `update.Pull`, `update.DryRun` → `Preview` | Managed paths and the re-apply loop live in `cmd/aval` and print inline |
| `sync` | Text only (`core_commands.go:93-120`) | `txsync.Run` → `Outcome`; `DryRun` previews | None in core |
| `doctor` | Styled table | `doctor.Run` → `[]Result` | One truncated message per check (hardcoded `…`). `Fix` is all-or-nothing and removes links with no backup |
| `restore` | Text only | `Store.List`, `Load`, `Restore` with `DryRun`, `Pin` | `Action.Action` is free text. A `Run` does not record what made it |
| `uninstall` | Text plus a `[y/N]` prompt | `uninstall.Run` with `DryRun` → `[]Item` | `Item.Action` is free text |
| `dev` | Text only | `devmode.List`, `Remove`, `Prepare`, `Shell`, `GuardNested` | `Prepare` runs `go build` and writes to a writer; it is not a TUI operation |
| `version` | Text | `currentBuild()` | No paths or links |
| update notice | stderr before install | `update.CheckLatest` | The stamp keeps no result, so the notice needs a network call |
| budget, rules, review, design, deck, pdf, release, plumbing | Text | n/a | Not in the hub; they stay in `aval help` |

### `internal/ui`
- **One model owns everything.** `Model` (`model.go:71-116`) holds all five screens and switches on a `Screen` enum.
- **Frame and text helpers are `Model` methods.** `render` and `hintLines` call `m.keyMap()`.
- **Tests reach inside.** There are 31 `.(Model)` casts and 36 direct field reads, and `cmd/aval` casts to `ui.Model` too. `Model` must keep its name, fields and behaviour.
- **The golden harness is typed to `Screen`.** `goldenScreens` and `screenFixture` cannot host a screen outside `Model`.
- **Reusable as is:** theme engine and `Theme.State`, glyph tiers, key maps (hints and help), `scrollWindow`, the spinner, and the one-action-per-cycle runner.
- **Absorbed here:** T20 of `odd/tasks/tui-experience.md`.

## 2. Hub design

### Decisions
- **Only bare `aval` opens the hub**, and only with a terminal on stdin and stdout and `TERM` not `dumb`.
  - `aval install` still goes straight to the installer; `aval config` goes straight to settings.
  - `aval help`, `--help`, pipes and CI keep the usage text byte for byte.
- **Opening the hub writes nothing and uses no network** (commit 5a9d5ad's promise).
  - The doctor summary is read-only.
  - The update line comes from a cache, never a fetch.
- **Nine entries in four groups of at most three** (Hick). Group names sit in a gutter, so they cost no rows.
- **Entries that cannot run show the reason.** With no repository, only Install, Backups, Settings and About are live.
- **Inside `aval dev`** the hub shows the sandbox marker (`[aval dev:default]`) in place of the platform label.

### Large wordmark (owner-approved 2026-11-23)
The wordmark is rasterised from the real brand geometry in `tools/brand/logo.go`: each `a` is a ring plus a stem, the `v` is the check, and the `l` is the ascender. It is never hand-drawn.
- **Unicode tier:** quadrant block elements (`▗▟█▛▜▙▀▄▌▐`), 7 rows by 40 columns, sampled 4x4 per subpixel at a 0.42 coverage threshold.
- **ASCII tier:** derived cell by cell from the same quadrant raster, so the shape never drifts. Cells with 3 or 4 quarters filled become `#`. A top pair becomes `"` and a bottom pair `,`; other pairs become `#`. A single top quarter becomes `'`, a single bottom quarter `.`. Pure ASCII, 7 rows by 40 columns.
- **Generation:** a generator in `tools/brand` emits `internal/ui/wordmark_gen.go`, and a test checks that the generated file is current. When the brand changes, the terminal logo changes with it.
- **Rejected:** braille (it reads dotted and grainy in screenshots) and a hand-drawn box-letter `a`.
```
                     ▗██              ██
                    ▗██▘              ██
   ▄▄▄▄▄▄▄▖  ▗▄▖    ▟█▘    ▗▄▄▄▄▄▄▄   ██
 ▗██▀▀▀▜██▌  ▝██▖  ▟█▛    ▟█▛▀▀▀███   ██
 ██▘    ▜█▌   ▝██▖▟█▛    ▐█▛    ▝██   ██
 ▜█▙▖  ▄██▌    ▝███▛     ▝██▄  ▗▟██   ██
  ▀▜████▜█▌     ▝█▛       ▝▀████▛██   ██
```
- **Colour:** ink only, in bold `strong`.
- **Placement:** at 80 columns or more, beside the status panel. From 40 to 79 columns, stacked above it. Compact screens never show it.
- **Fallback order:** first the wordmark gives way to the one-line lockup, then the gaps, then the tagline, then the panel collapses, and finally the menu scrolls.
- **Hub only.**

### Hub at 80x24
```
                 ╱         │    Version  0.3.0 · ! 0.3.1 available
                ╱          │    Repo     ~/.aval
  ╭────┐  ╲    ╱   ╭────┐  │    Doctor   ! 9 ok, 1 warn, 0 fail
  │    │   ╲  ╱    │    │  │    Backup   2026-11-23 10:14 · sync · 3 entries
  ╰────┘    ╲╱     ╰────┘  │    Theme    aval-dark · unicode
  Nothing passes without evidence.                               macOS (arm64)
  ────────────────────────────────────────────────────────────────────────────

  Set up     › Install       link the repo configs into this home
               Update        pull the repo, then re-apply
               Sync          merge repo-managed settings into the live config
  Health       Doctor        check this machine against the repo
  Undo         Backups       list backup runs and restore one
               Uninstall     remove the links aval made, put back what was there
  Tools        Settings      theme, glyphs, motion and update checks
               Dev sandbox   a throwaway home for trying aval
               About         version, paths and links

  ────────────────────────────────────────────────────────────────────────────
  1 warning · open Doctor to see it
  ↑↓ move · enter open · ? help · q quit
```

### Status panel
| Row | Source | Notes |
| :--- | :--- | :--- |
| Version | `currentBuild()` plus the cached notice | `! x available` only when the cache holds a newer version |
| Repo | `repoloc.Resolve` | `× not found`; the status line then reads `Not set up yet. Start with Install.` |
| Doctor | `doctor.Run` in a `tea.Cmd` | `checking` on first paint, then the worst state's glyph and the CLI summary words |
| Backup | `Store.List()[0]` | Absolute time, so goldens need no clock; `none yet` when empty |
| Theme | Resolved theme and glyph tier | |

### Narrow and compact
- **Narrow (40 to 79):** panel stacked, group gutter from 60 columns up, descriptions end in an ellipsis.
- **Compact (under 40):** no groups, no descriptions, one status line.
```
 aval 0.3.0
 ! 1 warn  0.3.1 available
 > Install
   Update
   Sync
   Doctor
   Backups
   Uninstall
   Settings
   Dev sandbox
   About
 enter open  ? help
```

## 3. Views

Every view uses the existing frame: lockup, tagline, rule, body, rule, status line and hints.

### Operation pattern
Anything that writes goes through four steps: preview (nothing written), confirm, run, result (counts, backup id and the undo command).
- **Confirm** is two rows chosen with arrows and enter. There is no `y` key and no box.
- **Destructive actions** confirm on their own step with the cursor on `Cancel`: restore, uninstall, a doctor fix, sandbox reset and sandbox remove.
- **Update and sync** put their two rows under the preview, with the cursor on the action.
- **The confirm step always names the safety net**, or says there is none.

### Install (reuse)
The hub hosts the existing `Model`: select when a repository is known, welcome otherwise. Closing the done screen runs `finishInstall`, returns to the hub and refreshes the panel.

### Update
```
  Update                                                             ~/.aval

  3 commits behind origin                                  a1b2c3d → d4e5f6a

  Managed paths that change
    config/zsh/.zshrc              12 +++--
    skills/plan-deck/SKILL.md       4 +-
  New manifest entries
    + skills/new-skill -> ~/.claude/skills/new-skill

  › Update and re-apply    pull with --ff-only, then link what is new
    Back                   change nothing
  ────────────────────────────────────────────────────────────────────────────
  Files in the way move to ~/.aval-backup first · undo: aval restore latest
  ↑↓ move · enter choose · esc back · ? help · q quit
```
- **Opening the view** fetches, with a spinner; `esc` cancels.
- **Run:** the pull, then the install progress screen titled `Re-applying`.
- **Result:** `Updated a1b2c3d → d4e5f6a. 2 linked, 55 already in place.`
- **Dirty tree:** an error plus the fix, and only `Back`.
- **Up to date:** `√ up to date` and `Re-apply links anyway`.
- **npm installs:** show the `update.commandFor` command without running it.

### Sync
```
  Sync                                               ~/.claude/settings.json

    ! would change   settings   hooks, statusLine, env
    √ up to date     rules      ~/.codex/AGENTS.md
    ! would update   rules      ~/.gemini/GEMINI.md
    √ linked         overlay    ~/.claude/skills/private-skill

  › Apply sync    back up, write, verify with doctor, roll back on failure
    Back          change nothing
  ────────────────────────────────────────────────────────────────────────────
  The previous files are saved to ~/.aval-backup · undo: aval restore latest
```
- **No drift:** `√ nothing to sync` and only `Back`.
- **Rolled back:** `× rolled back`, the failed checks and the backup id.
- **No spinner:** sync is local.

### Doctor
```
  Doctor                                              9 ok · 1 warn · 2 fail

  › × fail   broken-links     2 broken                         fix available
    × fail   settings-drift   live settings differ in: hooks   fix available
    ! warn   skills           1 skill incomplete
    √ ok     links            57 manifest links in place
    √ ok     hooks-resolve    14 hook commands resolve

  broken-links
  Dangling links in the agent directories. They point at files that no
  longer exist, usually a skill removed from the repo.
    ~/.claude/skills/old-skill
    ~/.claude/skills/retired
  ────────────────────────────────────────────────────────────────────────────
  3 of 12 need attention · 2 can be fixed here
  ↑↓ move · f fix this · a fix all · r re-run · s security · esc back · ? help
```
- **Sorting:** problems sort first; the lower pane explains the check under the cursor.
- **Safe fixes:** removing dangling links (recorded in a backup first) and the sync-repairable drift checks, through `txsync.Run`.
- **Other checks:** show `How to fix` with the exact command, and no fix key.
- **After a fix:** the checks re-run and update in place.

### Backups and restore
```
  Backups                                            ~/.aval-backup · 4 runs

  › 2026-11-23 10:14   sync         3 entries
    2026-11-23 09:02   install     12 entries   pinned
    2026-11-20 18:40   uninstall   14 entries
    2026-11-19 08:11   sync         2 entries   × failed, rolled back

  20261123-101402 holds
    ~/.claude/settings.json      file
    ~/.codex/AGENTS.md           file
    ~/.gemini/GEMINI.md          created by aval
  ────────────────────────────────────────────────────────────────────────────
  The newest 5 unpinned runs are kept
  ↑↓ move · enter preview restore · p pin · esc back · ? help · q quit
```
- **Enter** opens a preview (`! would restore`, `! would remove`, `√ unchanged`), then confirm on `Cancel`.
- **Safety net:** foreign content moves to the run's `.displaced` folder; it is not deleted.
- **Legacy and pending runs** have no preview and say why.
- **Without a repository** the list still works.

### Uninstall
- **Opening the view** runs the preview at once (a dry run) and lists items with their kind. Then:
  - a count line: `57 links removed · 3 previous files restored · 1 rules block cut`;
  - the status line: `Nothing has changed yet`.
- **Enter** goes to a confirm step with the cursor on `Cancel`, which names `~/.aval-backup` and `aval restore latest`.
- **Nothing to remove:** `√ nothing to uninstall: no link points into this repo`.

### Settings (reuse)
The hub hosts the existing settings screen. Enter saves and esc discards; both return to the hub, which adopts the saved theme at once.

### Dev sandbox
```
  Dev sandbox                                             ~/.cache/aval/dev

  A throwaway home for trying aval. Nothing in it touches your real home.

  › default    ready        ~/.cache/aval/dev/default
    release    incomplete   ~/.cache/aval/dev/release
  ────────────────────────────────────────────────────────────────────────────
  Enter leaves this screen for a shell in the sandbox; exit brings you back
  ↑↓ move · enter open shell · n new · r reset · d remove · esc back · ? help
```
- **Enter, new and reset** hand the terminal to `aval dev [reset] --name <n>` through `tea.ExecProcess`.
- **New** asks for a name in an inline field, validated with `devmode.ValidName`.
- **Reset and remove** confirm on `Cancel`, with the line: `There is no backup: a sandbox is disposable. Your real home is not touched.`
- **Inside a sandbox** the list is read-only.
- **Without Go or a checkout** the entry is unavailable, with the reason.

### About
Shows the version, commit, build date and channel. Then the paths (repository, config, backups, sandboxes, binary) and links (source, issues). `esc back`.

## 4. Navigation
- **Stack.** The hub is the bottom of the stack. Enter pushes a view and `esc` pops one; at the hub, `esc` does nothing.
- **Global keys.** The router adds these to every screen's key map, so hints and help keep one source:

| Key | Idle | Text field open | Operation running |
| :--- | :--- | :--- | :--- |
| `esc` | back | close the field | cancel if cancellable, else inert |
| `q` | quit | typed as text | stop after the current step |
| `ctrl+c` | quit | quit | stop after the current step; never mid-write |
| `?` | help | typed as text | help |

- **Back at the hub,** doctor re-runs in the background and the backup row reloads.
- **Running operations come in three kinds:**
  - Stepped (install, re-apply): one action per cycle; a stop still commits the backup run.
  - Network (fetch, pull, clone): spinner and a screen-owned `context`; `esc` cancels.
  - Atomic (sync, restore, uninstall, doctor fix): keys are inert and the program cannot quit.
- **Async messages carry a generation number.** A stale one is dropped. The router broadcasts non-key messages to the whole stack.

## 5. Architecture

**Dependency rule:** `internal/core` never imports `internal/ui`. Views reach behaviour only through injected functions (`Cloner`, `WelcomeEnv.Prepare` style), which `cmd/aval` wires.

### Refactor without touching existing tests
1. **Extract `chrome`.** It holds theme, version, platform, width and height, plus the frame and text methods. `Model` embeds it. `chrome.renderWith(v view, km keyMap)`; `Model.render` and `Model.hintLines` stay as one-line wrappers.
2. **Add the router.** `App` is a `tea.Model` holding `chrome`, `[]screen`, the help overlay and the services. Navigation uses `pushMsg`, `popMsg` and `quitMsg`.
   ```go
   type screen interface {
       init() tea.Cmd
       update(msg tea.Msg) (screen, tea.Cmd)
       keys() keyMap
       view() view
       busy() busyKind
   }
   ```
3. **Host `Model` instead of dissolving it.** A `flow` screen wraps a `Model`. The four places that return `tea.Quit` go through `m.exit()`, which quits standalone and pops when hosted.
4. **Views register themselves.** Each `screen_<name>.go` registers its constructor in `init()`. A menu entry with no view renders as unavailable.
5. **A second golden table, `goldenViews`,** with twins of the invariant tests. `goldenScreens` and its files are untouched.

### Services
`internal/ui.Services` is defined once, in H7:
- `Home`, `Repo`, `RepoErr`, `Build`, `Sandbox`
- `Doctor(security)`, `FixCheck(id)`
- `Backups()`, `RunEntries(run)`, `RestorePreview(run)`, `Restore(run)`, `Pin(id, on)`
- `UninstallPreview()`, `Uninstall()`
- `UpdatePreview(ctx)`, `Pull(ctx)`, `Reapply()`
- `SyncPreview()`, `Sync()`
- `Sandboxes()`, `DevCommand(sub, name)`, `RemoveSandbox(name)`
- `Installer()`, `FinishInstall(Model) ([]string, error)`, `Settings()`, `CachedNotice()`

### Core additions
- **doctor:**
  - `Result.Details []string` and `Result.Fix FixKind`, both `json:"-"`.
  - `Describe(id)`.
  - `RemoveBroken(opt, rec)`, which snapshots before removing.
  - The sync fix stays out of doctor, because txsync imports doctor.
- **backup:**
  - `Run.Origin` (a marker file, `Recorder.SetOrigin`).
  - `Action.Kind`.
  - `Store.Preview(run, opt)`.
- **uninstall:** `Preview(opt)`, `Item.Kind` and `Item.Restores`.
- **update:**
  - `ManagedPaths(m)`, moved from `main.go`.
  - `Cached()`; `CheckLatest` stores the version it saw.

## 6. Tasks

Rules for every task:
- **Testing:**
  - Strict TDD.
  - `go vet`, `gofmt -l` and `go test ./...` pass on macOS, Linux and Windows.
  - Every new view has goldens at 36, 80 and 120 columns, in both glyph tiers, and passes the width and height invariants.
  - Use fake services: no network and no real HOME.
- **Process:**
  - One commit per task.
  - UI tasks run the design pipeline and `aval design ack` first.
  - Existing `internal/ui` tests and goldens are not modified.

### Phase 0: contract and core data
- [x] **H1 (opus): DESIGN.md amendment.**
  - Covers the hub, the wordmark (both tiers, size rule, fallback), the navigation table and the confirmation pattern.
  - Groups hold at most three entries; the spinner is allowed for fetch and pull.
  - Ends with a decision-log entry.
  - RED: a docs guard test.
- [x] **H2 (sonnet): doctor as data.**
  - Adds `Details`, `Fix`, `Describe` and `RemoveBroken`.
  - RED: `Details` lists all items when the message truncates; `RemoveBroken` is restorable; `--json` is unchanged.
- [x] **H3 (sonnet): backup origin and typed preview.**
  - RED: origin round-trips; a missing marker yields `""`; `Preview` writes nothing.
  - `aval restore --list` stays byte-identical.
- [x] **H4 (sonnet, after H3): `uninstall.Preview`.**
  - RED: kinds and `Restores` are reported; the HOME tree hash is unchanged; no backup dir is created.
- [x] **H5 (sonnet): `update.ManagedPaths` and `Cached`.**
  - RED: `Cached` returns what the last check saw; it is empty with no cache; it never touches the network or writes.

### Phase 1: router
- [x] **H7 (opus, after H1): `chrome`, `screen`, router and `goldenViews`.**
  - RED, router tests with fake screens:
    - enter pushes and `esc` pops; `esc` at the root does nothing;
    - `q` quits when idle and stops when busy; quitting is refused while atomic;
    - size reaches every screen; a stale generation is dropped;
    - `?` lists the screen's keys plus the global ones.
  - No diff under `internal/ui/testdata` or in existing tests.
- [x] **H8 (opus, after H7): host the installer and settings.**
  - RED:
    - a hosted `Model` pops where it used to quit;
    - closing done calls `FinishInstall` exactly once; quitting at select never calls it, and neither does a stopped run;
    - saving settings changes the router theme;
    - `finishInstall` returns warnings instead of writing to stderr.
- [x] **H9 (opus, after H7): the operation component.**
  - RED:
    - a destructive confirm opens on `Cancel`, and enter on `Cancel` runs nothing;
    - the safety line is required;
    - `esc` cancels a cancellable run; an atomic run ignores keys.

### Phase 2: hub
- [x] **H10 (opus, after H1): the wordmark generator.**
  - `tools/brand` rasterises the brand geometry into `internal/ui/wordmark_gen.go`. Reference implementation: the session prototype `raster.py` (quad, 7 rows).
  - RED:
    - The generated file is current.
    - Every row has the same width.
    - The ASCII tier is pure ASCII.
    - The rhythm test: the check and the `l` reach row 1 and the bowls do not.
    - The emoji gate passes.
    - Nothing renders below the size rule.
- [x] **H11 (opus, after H7 and H10): the hub screen.**
  - RED:
    - `checking` shows before doctor arrives, and the worst state is shown after;
    - with no repo, entries are disabled with the reason;
    - the wordmark gives way at 80x20;
    - a view that is not registered shows as unavailable;
    - an 80x24 golden.
- [x] **H12 (sonnet, after H2-H5, H8 and H11): dispatch and real services.**
  - RED:
    - with a fake TTY, bare `aval` opens the hub;
    - without a TTY, and for help, `--help` and `TERM=dumb`, the usage is byte-identical;
    - opening and quitting leaves HOME and the cache byte-identical with no HTTP request;
    - the hot paths still never read config.

### Phase 3: views (each after H9 and H11)
- [x] **H13 (opus, +H2): Doctor.**
  - RED: problems sort first; the detail pane follows the cursor; there is no `f` without a fix; a fix confirms on Cancel and then re-runs; `s` shows security.
- [x] **H14 (opus, +H3): Backups and restore.**
  - RED: runs are listed newest first with origin and pin; `p` pins; legacy and pending runs have no preview; the preview only previews; the confirm names `.displaced`.
- [x] **H15 (sonnet, +H4): Uninstall.**
  - RED: opening only previews; counts match; an empty run has no continue; the confirm is on Cancel; the result names restore.
- [x] **H16 (opus, +H5, H8): Update.**
  - RED:
    - `esc` cancels the fetch context;
    - a dirty tree offers only Back; up to date offers re-apply;
    - the pull hands over to progress;
    - npm shows its command.
- [x] **H17 (sonnet): Sync.**
  - RED: opening only previews; no drift offers only Back; a rollback shows its checks and the backup id.
- [x] **H18 (sonnet): Dev sandbox.**
  - RED: enter returns `ExecProcess` for `dev --name`; an invalid name stays in the field; remove confirms on Cancel with no backup; inside a sandbox there are no write keys.
- [x] **H19 (sonnet): About.**
  - RED: paths are shortened with `~`; the build block matches `aval version`; the view scrolls at 80x12.

### Phase 4: ship
- [x] **H20 (sonnet, after H12): docs, CHANGELOG and the vhs tape.**
  - The docs guard requires the hub to be described.
- [ ] **H21 (opus): cross-terminal verification.**
  - Closes T20 of the previous plan.
  - Screenshots decide `╱╲` versus `/\`.

### Waves
| Wave | Tasks |
| :--- | :--- |
| 1 | H1, H2, H3, H5 |
| 2 | H4, H7, H10 |
| 3 | H8, H9, H11 |
| 4 | H12, H13, H14, H15, H17, H18, H19 |
| 5 | H16, H20 |
| 6 | H21 |

If the release must shrink, cut in this order:
1. H18.
2. H19.
3. H17; sync stays reachable as `aval sync`.
4. The fix keys in H13.

## 7. Risks and open questions

### Risks
- **`╱` and `╲` vary by font.** H21 decides, with ASCII slashes as the fallback.
- **Writes to stderr corrupt the alt screen.** `recordRepo` does it today; H8 fixes it.
- **`tea.ExecProcess` with captured stderr** has not been verified on bubbletea 1.3.10.
- **A background doctor run could race an operation.** Refresh only on return.
- **The uninstall dry run is assumed to write nothing.** H4 proves it.
- **About 60 new golden files.** Charm upgrades go in their own commit.
- **`main.go` is a hot file.** Only H5 and H12 touch it, in separate waves.

### Open questions
1. ~~Wordmark~~ decided: a 7-row quadrant raster of the real geometry.
2. ~~Update line~~ decided: read the cache; opening the hub stays offline and writes nothing.
3. ~~Dev sandbox without a checkout~~ decided: shown as unavailable with the reason "needs a checkout and Go".

## 8. Later
- `aval doctor`, `aval restore` and `aval uninstall` opening their views on a TTY.
- Doctor fix for missing links.
- `Details` and `Fix` in `--json`.
- Restoring a single path.
- Pruning runs.
- Letter accelerators and a filter.
- Mouse, Spanish copy, user theme files.

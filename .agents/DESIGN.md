# DESIGN.md — aval

Design system for the visible surfaces in this repository. Three surfaces, all
terminal UI, not web UI, so the constraints are different and stricter:

- **The tmux status bar**: sections 1 to 7. ASCII only, no motion.
- **The installer TUI** (`aval install`, `aval config`): section 10.
- **The hub** (bare `aval` on a TTY) and its views: section 10.
- **CLI output** (`aval` usage, `aval doctor`, `aval install --yes`): section 10.

Sections 1 to 7 govern only the status bar; section 10 does not relax them.

## 1. Intent & North Star

**Instrumentation you read at a glance, never a dashboard you study.**

The status bar sits above every window all day. It must answer "is anything
wrong?" in peripheral vision, without ever competing with the terminal content
below it. Colour encodes *which metric*, not decoration.

Anti-references:
- Powerline arrow separators. They require patched Nerd Font glyphs, break in
  any unpatched terminal, and add three characters of chrome per segment.
- Live sparklines, animated bars, gradients. tmux repaints by polling; anything
  that implies continuity is lying about its own refresh rate.
- Emoji as metric icons. They render at inconsistent widths and misalign the row.

## 2. Tokens

### Colour — Kanagawa (matches the configured theme)

The theme is the source of truth. No invented palette.

| Role | Kanagawa | Hex | 256 | Contrast vs ink |
| --- | --- | --- | --- | --- |
| CPU | crystalBlue | `#7E9CD8` | 110 | 7.4:1 |
| GPU | oniViolet | `#957FB8` | 140 | 5.6:1 |
| RAM | springGreen | `#98BB6C` | 108 | 9.1:1 |
| Time | carpYellow | `#E6C384` | 179 | 11.6:1 |
| Date | sakuraPink | `#D27E99` | 174 | 7.1:1 |
| Segment text | sumiInk | `#1F1F28` | 235 | — |
| Alert | autumnRed | `#C34043` | 167 | 4.9:1 |

**Rule: every segment is a saturated background with `sumiInk` text.** Light text
on saturated mid-tones is where terminal bars usually fail contrast. All ratios
above are computed against `#1F1F28` and clear 4.5:1.

**Rule: the bar itself has no background.** `status-style bg=default` inherits
the terminal's own background, so Ghostty's `background-opacity` and blur show
through. The coloured blocks are the only painted surface; everything else is
the terminal. Bar text is `fujiWhite` (223), active window `springGreen` (108),
inactive windows muted grey (245).

Threshold colouring is reserved for *values*, never for chrome: a metric turns
`autumnRed` only when it actually crosses its alert threshold.

### Typography

The terminal's font, one cell per character. The only levers available are
**weight** (`bold`) and **case**.

- Metric labels: uppercase, ≤3 characters (`CPU`, `GPU`, `RAM`). Uppercase is
  permitted here precisely because these are labels, not prose.
- Values: bold. The number is the payload; the label is the key.

### Spacing

- One padding space inside each segment: `| CPU 32% |` renders as `␣CPU 32%␣`.
- One unstyled space between segments. That gap is the separator; no glyph does
  that job.
- No trailing space at the row's end.

## 3. Layout Primitives

Single row, two anchors:

- **Left**: session and window list (owned by the kanagawa theme).
- **Right**: metric segments, ordered by volatility — fastest-changing leftmost,
  so the eye lands on movement first: `CPU · GPU · RAM · time · date`.

Width budget: the right side must stay under 60 columns so it never collides
with the window list on a split screen.

## 4. Component Patterns

### Metric segment

- **Purpose**: one measurement, readable without focus.
- **Anatomy**: `#[fg=ink,bg=<role>,bold]␣LABEL VALUE␣#[default]`
- **States**: normal (role colour) · alert (`autumnRed` when over threshold).
- **Accessibility**: never encode meaning in colour alone. The label and the
  numeric value carry the information; colour only speeds up locating it.

## 5. Motion System

**None, deliberately.** tmux repaints on `status-interval` polling. Any
animation would be a stroboscopic artefact of the polling rate, not motion
design. There is nothing here to ease.

## 6. Accessibility Baselines

- Contrast ≥4.5:1 for every segment, verified numerically, not by eye.
- Information never carried by colour alone (colour blindness, monochrome
  terminals, screenshots).
- Degrades correctly: no glyph outside plain ASCII, so the bar stays intact in
  any terminal without a patched font.

## 7. Anti-patterns (banned in this project)

- **Powerline separators** — require patched fonts; break silently elsewhere.
- **Any status command over 100ms** — it runs forever, on every refresh. The
  kanagawa `cpu_info.sh` cost 5064ms because it slept to sample a delta.
- **Duplicated metrics** — two sources rendering CPU is how they drift apart.
- **Emoji or Nerd Font icons as labels** — inconsistent widths misalign the row.
- **Colour as the only signal** — always paired with a label and a value.
- **An opaque band behind the bar** — it would cover the terminal's own
  background and defeat its transparency and blur.

## 9. Brand — aval

**Intent.** "Aval" is a guarantee. The mark puts the guarantee inside the name:
the v of aval is a check. Monoline geometry (circle, line, polyline), round
caps, drawn as paths — never set in a font, so it renders identically anywhere.

- **Colour:** pure ink, `#000000` on light, `#ffffff` on dark. No accent, no
  gradient. Tagline grey `#525252` (7.81:1 on white) / `#a3a3a3` (8.33:1 on black).
- **Wordmark:** x-height 100, stroke 22, ascender 150. The check's long arm and
  the l share the ascender, giving the rhythm short-tall-short-tall.
- **Isotype:** the a's bowl with its stem replaced by the check. Stroke = 1/8 of
  the canvas so it keeps 2px lines at a 16px favicon.
- **Source:** `tools/brand`; guarded by `tests/readme_test.go`.
- **Anti-references:** a tick inside a box (the stock "task done" icon — tried
  first and rejected), a tick inside a circle, colour accents, glassmorphism.

## 10. Installer TUI and CLI output

**Intent.** The first screen a person sees must say what will happen before it
happens, and stay readable in any terminal. Evidence over decoration: report
counts and paths, never "Success".

### Themes

The default theme is `aval`, built from the brand (section 9): pure ink for text
and chrome, colour only for state. Every pair is measured with
`aval design gate contrast`.

| Token | `aval` dark (on `#000000`) | `aval` light (on `#ffffff`) |
| --- | --- | --- |
| text, strong, key | `#ffffff` (21:1) | `#000000` (21:1) |
| muted | `#a3a3a3` (8.33:1) | `#525252` (7.81:1) |
| rule (decorative, carries nothing) | `#2e2e2e` | `#d4d4d4` |
| ok | `#98BB6C` springGreen (9.66:1) | `#5a7339` darkened springGreen (5.32:1) |
| warn | `#E6C384` carpYellow (12.50:1) | `#77713f` lotusYellow (4.98:1) |
| fail | `#E46876` waveRed (6.54:1) | `#C34043` autumnRed (5.08:1) |
| cursor row | reverse video | reverse video |

`autumnRed` fails on black (4.13:1), so dark uses `waveRed`. `lotusGreen`
fails on white (3.91:1), so light uses a darkened springGreen.

Opt-in themes:
- **`terminal`** uses the 16 ANSI colours only, so it inherits the user's theme.
- **`mono`** uses attributes only.
- **Community palettes:** catppuccin, tokyo-night, gruvbox, dracula, nord,
  kanagawa, rose-pine, solarized, one-dark and everforest. They use upstream hex
  values only, with sources in `internal/ui/themes/SOURCES.md`.

**State colour rule (all themes).**
- **Colour goes on the glyph only.** A state renders as a coloured glyph plus
  its word in the `text` token (`√ linked`, with only `√` coloured). The word is
  text and meets 4.5:1 through `text`. The glyph is a graphic, so `ok`, `warn`
  and `fail` need 3:1 (WCAG 1.4.11) against the theme background.
- **Text tokens need 4.5:1:** `text`, `strong`, `muted` and `key`. A community
  text token that misses it is swapped for the closest neutral (foreground,
  subtext, comment or base) entry of the same palette that passes.
- **A state token keeps its hue.** If the upstream colour misses 3:1, use the
  closest entry of the same hue family that passes, so green stays green. If no
  entry of that hue passes, the glyph renders in `strong` with bold. It never
  takes a different hue: a warning is never red, a failure is never grey.
- **The three states stay distinct.** No two of `ok`, `warn` and `fail` share
  a colour, unless more than one has fallen back to `strong`. Each glyph shape
  still differs.

`NO_COLOR` overrides every theme and flag. `muted` is never SGR dim or bright
black.

### Glyphs

There are two tiers, picked from the terminal's capabilities:

| Role | Unicode | ASCII |
| --- | --- | --- |
| cursor | `›` | `>` |
| check states | `[√] [ ] [-]` | `[x] [ ] [-]` |
| ok / warn / fail | `√ ! ×` | `ok ! x` |
| progress | `━ ─` | `# -` |
| separator, ellipsis | `· …` | two spaces, `...` |

Rules for both tiers:
- A state glyph is always followed by its word (`√ linked`).
- No emoji, no Nerd Font icons, no powerline glyphs. States colour the glyph only; the word uses `text`. The Dingbat check marks fail `aval design gate emoji`, so the tiers use `√` and `×`.
- The one exception to "no icons" is the hub's nine entry icons (see Hub icons below): one cell wide each, from that table only, never as decoration anywhere else.
- Box drawing and block element characters (`▗▟█▛▜▙▀▄▌▐`) are allowed for the
  hub wordmark only. Nothing else uses them as decoration.
- No em dashes in UI copy.

### Layout

- **Margins:** 2 columns, 1 when compact. No outer border, no painted canvas.
- **Frame:** header lockup, rule, body, rule, status line, key hints.
- **Width:** content caps at 76 columns.
- **Breakpoints:** compact under 40 columns, narrow 40 to 79, regular 80 and up.
- **Invariants:** no line is wider than the terminal and the frame is never
  taller than it. Lists scroll with the cursor visible.
- **Truncation:** labels end with an ellipsis. Paths shorten from the front and
  keep the tail (`…/dev/default/repo`); a path under HOME becomes `~/…`.

### Brand

- **Lockup:** `aval` in bold, the version muted, the platform right-aligned,
  and the tagline `Nothing passes without evidence.` on the next line. The
  tagline is dropped when compact.
- **Copy:**
  - Sentence case, verb first, no exclamation marks.
  - Every error says what, where and the command that fixes it.
  - Anything that writes names its safety net: the backup path and
    `aval restore latest`.

### Interaction

- **Keys follow convention (Jakob):** arrows move, space toggles, enter
  confirms, esc goes back, q quits, `?` shows every binding of the screen.
- **Option counts stay small (Hick):** the welcome screen has 2 choices, the
  select screen 5 groups, the settings screen 6 rows, the hub 4 groups of at
  most 3 entries.

### Hub

Bare `aval` opens the hub, and only with a terminal on stdin and stdout and
`TERM` not `dumb`. `aval install` and `aval config` still go straight to their
screens; `aval help`, `--help`, pipes and CI keep the usage text byte for byte.

- **Opening the hub writes nothing and makes no network calls.** The doctor
  summary is read-only, and the update line is read from the cache that the
  last update check left, never fetched.
- **Layout:** the wordmark beside the status panel, a rule, the menu, a rule,
  the status line and the key hints. Every view it opens uses the existing
  frame (lockup, tagline, rule, body, rule, status line, hints).
- **Inside `aval dev`** the sandbox marker (`[aval dev:default]`) replaces the
  platform label.

### Wordmark

Owner-approved on 2026-11-23. A raster of the real brand geometry (section 9):
each `a` is a ring plus a stem, the `v` is the check, the `l` is the ascender.
It is generated, never hand-drawn: a generator in `tools/brand` emits
`internal/ui/wordmark_gen.go`, and a test fails when the file is stale.

- **Unicode tier:** quadrant block elements, 7 rows by 40 columns, sampled 4x4
  per subpixel at a 0.42 coverage threshold.
- **ASCII tier:** derived cell by cell from the same quadrant raster, so the shape never drifts. Cells with 3 or 4 quarters filled become `#`. A top pair becomes `"` and a bottom pair `,`; other pairs become `#`. A single top quarter becomes `'`, a single bottom quarter `.`. Pure ASCII, 7 rows by 40 columns.
- **Colour:** ink only, bold `strong`. No state colour, no gradient.

```
                     ▗██              ██
                    ▗██▘              ██
   ▄▄▄▄▄▄▄▖  ▗▄▖    ▟█▘    ▗▄▄▄▄▄▄▄   ██
 ▗██▀▀▀▜██▌  ▝██▖  ▟█▛    ▟█▛▀▀▀███   ██
 ██▘    ▜█▌   ▝██▖▟█▛    ▐█▛    ▝██   ██
 ▜█▙▖  ▄██▌    ▝███▛     ▝██▄  ▗▟██   ██
  ▀▜████▜█▌     ▝█▛       ▝▀████▛██   ██
```

- **Size rule:** at 80 columns or more, beside the status panel. From 40 to 79
  columns, stacked above it. Compact (under 40) never shows it.
- **Fallback order** when the height runs short: the wordmark gives way to the
  one-line lockup, then the gaps go, then the tagline, then the panel
  collapses, and finally the menu scrolls.
- **Hub only.** No other screen shows the wordmark.
- **Rejected:** braille (reads dotted and grainy in screenshots) and a
  hand-drawn box-letter `a`.

### Status panel

Rows: Version, Repo, Doctor, Backup, Theme.
- **Labels** in `muted`, **values** in `text`.
- **Colour only on state glyphs** (`! 0.3.1 available`, `× not found`), per the
  state colour rule.
- **Doctor** shows `checking` on first paint, then the worst state's glyph, its
  count and word (`! 1 warn`); the full counts live in the Doctor detail pane.
- **Repo** keeps the tail of the path (Layout, Truncation).
- **Backup** is a relative day plus origin: `today 12:54 · install`,
  `yesterday 18:40 · sync`, older runs a date (`2026-11-20 09:00 · sync`), or
  `none yet`. The run count moves to the Backups badge and detail.
- **Theme** is the theme name; the glyph tier and motion live in the Settings
  detail pane.
- **Compact** collapses the panel to one status line.

### Menu

Nine entries in four groups of at most three (Hick). Group names sit in a left
gutter, so they cost no rows; the gutter appears from 60 columns up.

| Group | Entries |
| --- | --- |
| Set up | Install, Update, Sync |
| Health | Doctor |
| Undo | Backups, Uninstall |
| Tools | Settings, Dev sandbox, About |

- Each row is the cursor gutter, an icon, the name and an optional badge. The
  one-line description moves to the detail pane.
- **An unavailable entry is shown dimmed (`muted`), its badge slot reads `off`
  (so monochrome terminals still see it), and the detail pane shows its reason**
  in place of the description. It never disappears, so the menu does not
  reflow. With no
  repository only Install, Backups, Settings and About are live. Without a
  checkout or Go, Dev sandbox reads "needs a checkout and Go".

### Hub icons

Owner-approved on 2026-11-23 (hub v2). Each icon is one cell wide, passes
`aval design gate emoji` and has a non-ambiguous East Asian width (one cell in
CJK locales too). Rejected: ambiguous width (`↓ ● ◆ ⊕`), emoji variants
(stopwatch U+23F1, information U+2139, gear U+2699) and Nerd Font glyphs. `⊕` from the first draft is ambiguous, so
Doctor uses `⊞`, which keeps the plus of the ASCII `+`.

| Entry | Unicode | ASCII | Key |
| :--- | :--- | :--- | :--- |
| Install | `⇣` | `v` | `i` |
| Update | `⟳` | `@` | `u` |
| Sync | `⇄` | `=` | `s` |
| Doctor | `⊞` | `+` | `d` |
| Backups | `◷` | `o` | `b` |
| Uninstall | `⌫` | `x` | `x` |
| Settings | `⎈` | `*` | `,` |
| Dev sandbox | `⧉` | `#` | `v` |
| About | `◉` | `i` | `a` |

Icons are `muted`. The selected entry's icon and name are bold ink (`strong`).

- **Selection:** the cursor `›` sits in the gutter, right after the group name,
  and the selected row is bold. There is no reverse-video block on the hub.
- **Badges:** right-aligned in the list, one to five cells, shown only when they
  carry information: a state glyph plus a number, so colour stays on the glyph.

| Entry | Badge |
| :--- | :--- |
| Update | `↑ N` (`^ N` in ASCII): commits behind the last fetched upstream, read locally; none when unknown or zero |
| Doctor | worst state glyph and its count (`! 1`), none while checking |
| Backups | run count |
| Dev sandbox | sandbox count |

- **Detail pane:** right of a `│` divider (`|` in ASCII), from 80 columns. It
  shows the icon, the name and the shortcut letter; the one-line description
  (or the reason the entry is unavailable); a state block of two or three lines
  specific to the entry, built only from data the hub already holds (Doctor:
  last run, counts, the top problem; Backups: the newest runs; Update: the
  cached notice and the behind count; Settings: theme, glyphs, motion; Dev:
  sandboxes; Install and Sync: the repo and their last run; Uninstall: its
  description; About: version and channel); and the keys that entry's view
  offers (`enter` plus its own).
- **Under 80 columns** the pane collapses to one line under the list: the most
  urgent state, else the description. **Compact** does not show it.
- **Shortcuts:** a letter opens its entry directly, as enter would. The letters
  come from the key map, so the `?` help and the hint line
  (`i u s d b x , v a jump`) never drift.

### Navigation

- **Stack.** The hub is the bottom. Enter pushes a view, `esc` pops one; at the
  hub, `esc` does nothing.
- **Global keys** are added by the router to every screen's key map, so hints
  and `?` help keep one source:

| Key | Idle | Text field open | Operation running |
| --- | --- | --- | --- |
| `esc` | back | close the field | cancel if cancellable, else inert |
| `q` | quit | typed as text | stop after the current step |
| `ctrl+c` | quit | quit | stop after the current step; never mid-write |
| `?` | help | typed as text | help |

- **Running operations** are stepped (install, re-apply: a stop still commits
  the backup run), network (fetch, pull, clone: spinner, `esc` cancels) or
  atomic (sync, restore, uninstall, doctor fix: keys are inert and the program
  cannot quit).
- **Back at the hub,** the doctor row re-runs in the background and the backup
  row reloads.

### Operation pattern

Anything that writes goes through four steps: preview, confirm, run, result.
- **Preview** writes nothing.
- **Confirm** is two rows chosen with the arrows and enter. No `y` key, no box.
- **A destructive action opens on `Cancel`,** on its own step: restore,
  uninstall, a doctor fix, sandbox reset and sandbox remove. Update and sync put
  their two rows under the preview with the cursor on the action.
- **The safety net is always named** on the confirm step (`~/.aval-backup`,
  `aval restore latest`, the run's `.displaced` folder), or the step says there
  is none.
- **Result** reports counts, the backup id and the undo command, never
  "Success".

### Motion

One spinner, for network waits only: cloning, and the update fetch and pull.
Local work (sync, doctor, restore) never spins. Progress bars show real state.
With `motion = off` or no TTY, the spinner becomes static text.

### Non-interactive output

Without a TTY, output is plain and byte-stable for scripts. Hooks, `statusline`
and `tmux` helpers never read the visual settings.

## 8. Decision Log

### 2026-11-23: Hub v2: icons, badges and a detail pane
The v1 hub was correct but flat: every entry looked the same, the panel cut
paths in the middle (`/Users/alvar…default/repo`), the reverse-video row was
heavy, and nothing said what enter would do. The owner chose a list plus a
detail pane. The "no icons" rule is lifted for exactly the nine one-cell hub
icons (section 10, Hub icons); emoji and Nerd Font glyphs stay banned. The
cursor row on the hub is `›` plus bold, no reverse video. Paths keep their tail,
the backup row is relative, and every entry gets a letter from the key map.
Doctor's `⊕` was replaced by `⊞`: `⊕` has an ambiguous East Asian width.

### 2026-11-23: TUI hub: gentle-ai structure, aval aesthetic
Every command except install and config was text you had to know by name. The owner chose gentle-ai's structure (a home screen with a large logo, a status panel and a menu, one view per action) with aval's aesthetic: ink only, colour on state glyphs, the existing frame. Decided questions:
- **Wordmark:** a 7-row by 40-column quadrant raster generated from the real brand geometry in `tools/brand`, with an ASCII tier derived from the same raster. Braille and a hand-drawn box-letter `a` were rejected.
- **Update line:** read from the cache, so opening the hub stays offline and writes nothing.
- **Dev sandbox without a checkout:** shown dimmed as unavailable with the reason "needs a checkout and Go", not hidden.

### 2026-11-23: Installer TUI and CLI output join the contract; aval brand theme by default
The installer had hardcoded Tokyo Night, with muted text at 2.76:1 and unchecked items at 1.91:1, outside any contract. The owner chose the aval brand colours as the default and the widely used community themes as opt-in. Rejected options: Tokyo Night by default, and ANSI-only by default. ANSI-only is kept as the `terminal` theme. The unicode glyph tier and the single network spinner apply to this surface only; the status bar stays ASCII and motionless.

### 2026-11-21 — diff-review mod: Herdr panels follow Herdr's theme, not aval's brand; GitHub-like line comments
A Herdr panel lives inside Herdr's window, so it uses only `theme()` tokens and
the user's fonts (monospace for code), at the density of the shipped `changes`
mod; aval's Kanagawa/brand tokens don't apply there. Interaction mirrors a GitHub
PR review (Jakob): click a diff line to open an inline comment under it; per
comment only Edit/Delete, panel-level only Send to agent and Clear (Hick); rows
≥ 24 px (Fitts); diffs load per file into a virtual list (Doherty); a toast
confirms "N comments sent to <agent>" (peak-end). Rows of a virtual list cannot
register click handlers, so the list routes clicks and a comment's Edit/Delete
are fixed-width zones resolved from the press position.

### 2026-11-21 — Linux support: GPU segment omitted, not faked
Rationale: Linux has no cheap, unprivileged, vendor-neutral GPU utilisation
source (nvidia-smi, rocm-smi, intel_gpu_top are separate tools, tens of ms, some
privileged). A fixed `GPU 0%` would be a false reading, so on Linux the row is
`CPU · RAM · time · date`. CPU = /proc/loadavg / cores, RAM = MemTotal -
MemAvailable, both read with shell builtins (no forks). Platform comes from
`$OSTYPE`, so the macOS path pays nothing.

### 2026-11-21 — Stack cards in neutral grey, logos adapted per theme
Measured every logo's legible-pixel share (≥3:1 vs card) on candidate greys:
no single grey works (mid grey #a3a3a3 fails most colour logos; dark grey fails
black marks at 0–9%). Cards: #ededed light / #262626 dark. On dark, apple,
github, zsh, bash, tmux have dark tones lightened (brand colours kept) and bat
uses a lightened variant (2% → 66%). App icons with their own background are
clipped to a rounded box matching their aspect. Atuin and bat backgrounds made
transparent; OmniWM shown by its official app icon.

### 2026-11-21 — Hero presented as a brand, not a document
Rationale: the user wanted branding, not labelled fields. Lockup only —
wordmark (1.45×), claim "Nada sin pruebas." (sans 600, 36px), descriptor in
mono caps — beside the check-matrix isotype. No rules, no DEFINICIÓN/TESIS/PARA,
no pronunciation; the word's meaning lives in the README body instead.

### 2026-11-21 — Hero rebuilt again: Swiss grid, mark made of checks
Rationale: the guilloché read as classic; the user asked for modern. Editorial
grid — mono metadata on hairline rules, wordmark flush left at 1.6×, and the
isotype rendered as a 20×20 matrix of mini checks (cells on the mark's stroke
inked, the rest faint). The mark is built out of passed checks. Faint checks
are decorative at 1.48:1 (light) / 1.55:1 (dark); the shape is carried by the
inked checks at 21:1. Supersedes the guilloché hero.

### 2026-11-21 — Hero rebuilt as a guilloché certificate
Rationale: guilloché is the engraving that vouches for banknotes and
certificates, which is literally what "aval" means. Rosette of interlaced polar
sine waves sealed with the isotype, rope border, microprint of the thesis, and
the dictionary entry beside it. Microprint uses rotated glyphs, not <textPath>,
because librsvg does not render textPath. Supersedes the suminagashi hero.

### 2026-11-21 — Rebrand to aval; mark is the a fused with a check
Rationale: the harness's thesis is "nothing passes without evidence"; aval
(guarantee) says it in one word and contains "Alva". A boxed check was
rendered and rejected as generic. Supersedes the "Álvaro's Dotfiles" hero title.

### 2026-08-11 — Segment bar for CPU / GPU / RAM / time / date
Solid colour blocks separated by a space, per the reference the maintainer
supplied. Colours taken from Kanagawa so the bar belongs to the configured
theme instead of introducing a second palette.

### 2026-08-11 — GPU read via `ioreg`, not `powermetrics`
`powermetrics` requires superuser and cannot run from a status hook.
`ioreg -r -d 1 -w 0 -c AGXAccelerator` exposes `Device Utilization %`
unprivileged in ~20ms.

### 2026-08-11 — One tmux session per Ghostty tab
Startup used `new-session -A -s main`. Every Ghostty tab is a separate tmux
CLIENT, so `-A` attached them all to one session and both tabs rendered the
same window: a `cd` in one moved the other's sidebar. Confirmed live — one
session, two clients (ttys003, ttys006).

Each tab now takes its own session, reusing one with no client attached before
creating a new one, so closing a tab does not lose its work.

**Rule: tmux WINDOWS and Ghostty TABS are different layers.** Verifying one says
nothing about the other; the original test suite passed while this was broken.

### 2026-08-11 — Sidebar directory resolved against the target pane
`split-window -t <pane> -c '#{pane_current_path}'` creates the pane in the
window given by `-t` but expands `-c` against the ACTIVE pane. Opening a sidebar
in a background window inherited the foreground window's directory. The cwd is
now read from the target first and passed literally.

**Rule: `-t` selects where a command acts; format strings in other flags are
still evaluated in the current context.**

### 2026-08-11 — `sidebar-watch.sh` as a shell-independent safety net
zsh registers hooks at shell START, so any shell opened before the chpwd hook
was installed silently stopped syncing the tree. The watcher polls every 2s
(~0.65% of a core) and lives only while its sidebar does.

Two bugs found while building it, both caught by the suite: it was spawned
BEFORE the pane was tagged `@sidebar` (its first check, so it exited instantly),
and the background child inherited the parent's descriptors, hanging any
pipeline around the toggle. `nohup ... &` needs `</dev/null` and `disown` too.

### 2026-08-11 — nano as the file editor, so `Ctrl+X` closes
Files opened from the sidebar use nano, not `$EDITOR`. The requirement was
closing a file with `Ctrl+X`, which is nano's own Exit key — no invented
shortcut sits in between. nano also asks "Save modified buffer?" on unsaved
changes, so the safety property comes from the editor rather than being added
on top. `FILE_EDITOR` overrides it.

`$EDITOR` is ignored here on purpose: it is `nvim`, which needs `:q`.

**macOS note:** `/usr/bin/nano` is Apple's PICO build and tmux reports the
process as `pico`. Anything matching on the editor name must accept both.

### 2026-08-11 — Clickable `[FILES]` button in the status bar
Added because the keyboard route proved fragile on this machine (dead keys plus
`macos-option-as-alt` behaviour). A mouse target depends on neither.

It shows **no open/closed state**, deliberately: the state is already visible,
since the panel is either on screen or it is not. A second indicator would be
noise and would desynchronise the moment yazi is closed with `q`.

Rendered as bracketed text, never as a solid block — in this bar solid blocks
mean measurements, so a control has to read differently. Clicking anywhere else
on the bar keeps tmux's default window selection.

### 2026-08-11 — `allow-passthrough on` for yazi
yazi sends DA1/DSR probes at startup to detect terminal capabilities. With
passthrough off, tmux swallowed them and yazi waited out its full timeout,
printing "Terminal response timeout" over the panel for seconds before drawing.
Enabling passthrough lets the probes reach Ghostty.

### 2026-08-11 — Sidebar moved off `M-e` (dead-key collision)
`Alt+e` never reached tmux on a "Spanish - ISO" layout: `Option+E` is the dead
key for the acute accent, so macOS consumes it to compose `á`/`é`. The binding
was correct and the script worked; the keystroke simply never arrived.

Now bound twice: `M-t` (single press, no dead-key collision — those are `e`,
`i`, `u`, `n`) and `prefix` + `e` (never touches `Option`, so it is independent
of layout and of `macos-option-as-alt`). Supersedes the `M-e` binding.

**Rule for this project: never bind `Option` + `e`/`i`/`u`/`n`.**

### 2026-08-11 — Transparent bar (`status-style bg=default`)
The bar was rendering on a green band. That green is *tmux's own default*
(`bg=green,fg=black`); the kanagawa theme never assigns `status-style`, so
nothing was overriding it. Set to `bg=default` so the bar inherits the
terminal background and Ghostty's opacity and blur survive.

Related finding, not acted on: kanagawa emits its `status-left` with empty
colour fields (`#[fg=]`, `#[bg=]`), so its theme is not resolving. The window
list is styled explicitly here instead of relying on it.

### 2026-08-11 — CPU from load average, not sampled utilisation
Sampling utilisation needs two readings separated by a sleep. That is exactly
what made the previous implementation cost seconds per refresh. Load average is
a single instantaneous read. Supersedes nothing; it is the founding constraint.

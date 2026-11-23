# Hub v2: icons, live badges and a detail pane

Status: approved by the owner on 2026-11-23 ("much better UI/UX, with ASCII icons"). The direction chosen is a list plus a detail pane.

Why: the v1 hub is correct but flat.
- Every entry looks the same, so nothing stands out at a glance.
- The panel truncates paths. A sandbox repo shows as `/Users/alvar…default/repo`, and the backup row as `insta…`.
- The full-row reverse highlight is heavy.
- Nothing answers "what will this do?" before you press enter.

## Target at 80x24

```
                       ▗██              ██  Version  dev
                      ▗██▘              ██  Repo     …/dev/default/repo
     ▄▄▄▄▄▄▄▖  ▗▄▖    ▟█▘    ▗▄▄▄▄▄▄▄   ██  Doctor   ! 1 warn
   ▗██▀▀▀▜██▌  ▝██▖  ▟█▛    ▟█▛▀▀▀███   ██  Backup   today 12:54 · install
   ██▘    ▜█▌   ▝██▖▟█▛    ▐█▛    ▝██   ██  Theme    tokyo-night
   ▜█▙▖  ▄██▌    ▝███▛     ▝██▄  ▗▟██   ██
    ▀▜████▜█▌     ▝█▛       ▝▀████▛██   ██
  Nothing passes without evidence.                        [aval dev:default]
  ────────────────────────────────────────────────────────────────────────────
  Set up  ⇣ Install          │ ⊕ Doctor                                  d
          ⟳ Update     ↑ 3   │ Check this machine against the repo.
          ⇄ Sync             │
  Health› ⊕ Doctor     ! 1   │ Last run  today 12:54
  Undo    ◷ Backups      4   │           √ 10 ok  ! 1 warn  × 0 fail
          ⌫ Uninstall        │ Needs you ! plugins: 1 missing
  Tools   ⎈ Settings         │
          ⧉ Dev sandbox  2   │ enter  open the full report
          ◉ About            │ f      fix what is safe to fix here
  ────────────────────────────────────────────────────────────────────────────
  1 warning · open Doctor to see it
  ↑↓ move · enter open · i u s d b x , v a jump · ? help · q quit
```

## Design rules (DESIGN.md section 10 amendment)

### Icons
Each icon is one cell wide, passes `aval design gate emoji`, and has a non-ambiguous East Asian width. Glyphs rejected:
- ambiguous width: `↓ ● ◆`;
- emoji variants: `⏱ ℹ ⚙`;
- Nerd Font glyphs.

| Entry | Unicode | ASCII | Key |
| :--- | :--- | :--- | :--- |
| Install | `⇣` | `v` | `i` |
| Update | `⟳` | `@` | `u` |
| Sync | `⇄` | `=` | `s` |
| Doctor | `⊕` | `+` | `d` |
| Backups | `◷` | `o` | `b` |
| Uninstall | `⌫` | `x` | `x` |
| Settings | `⎈` | `*` | `,` |
| Dev sandbox | `⧉` | `#` | `v` |
| About | `◉` | `i` | `a` |

Icons are muted. The selected entry's icon and name are bold ink.

### Selection
The cursor `›` sits in the gutter, and the selected row is bold. There is no reverse-video block.

### Badges
A badge is right-aligned in the list, two to five cells wide. It appears only when it carries information. It uses a state glyph plus a number, so colour stays on the glyph only:

| Entry | Badge |
| :--- | :--- |
| Update | `↑ N` (commits behind the last fetched upstream, read locally) |
| Doctor | worst state and its count |
| Backups | run count |
| Dev sandbox | sandbox count |

### Detail pane
Right of a `│` divider, about 40 columns at 80. It shows:
- the icon, the name and the shortcut letter;
- the one-line description;
- a state block of two or three lines, specific to the entry;
- the keys that entry offers (`enter` plus its own).

Under 80 columns, the pane collapses to one line under the list: the description, or the most urgent state. It is not shown in compact mode.

### Shortcuts
A letter opens its entry directly. The letters come from the key map, so help and hints never drift.

### Paths
Paths shorten from the front and keep the tail: `…/dev/default/repo`. A path under HOME becomes `~/…`.

### Backup row
Relative day plus origin: `today 12:54 · install` or `yesterday 18:40 · sync`. The entry count moves to the Backups badge and detail.

### Offline
Opening the hub still writes nothing and touches no network. The commits-behind count comes from refs already fetched. It is a local read-only git query and is skipped when no upstream exists.

## Tasks

- [ ] **V1 (opus): DESIGN.md section 10 amendment and the hub v2 screen.**
  - Implement icons, badges, gutter groups, the detail pane per entry, letter shortcuts, the new selection style, the narrow-width collapse, path shortening and the relative backup row.
  - RED tests, each on an assertion:
    - every icon passes the gate and is width 1;
    - the ASCII tier is pure ASCII;
    - the detail pane follows the cursor;
    - each letter opens its view;
    - badges appear only with data;
    - no reverse video on the selected row;
    - a path keeps its tail;
    - the backup row is relative;
    - the 80x24 golden matches the target;
    - under 80 columns the pane collapses to one line;
    - width and height invariants hold for 20 to 120 by 8 to 40.
- [ ] **V2 (sonnet, parallel with V1): `Services.RepoStatus`.**
  - Return the commits behind the last fetched upstream and whether the tree is dirty, reading locally with no network and no writes.
  - Wire it in `cmd/aval/hub.go`.
  - RED tests: a temp repo with an upstream two commits ahead returns 2; a repo with no upstream returns unknown; the file tree is byte-identical after the call.
- [ ] **V3 (sonnet, after V1 and V2): wiring, docs excerpt and GIF.**
  - Regenerate the README and guide hub excerpt from the new golden, and re-record `docs/assets/hub.gif`.

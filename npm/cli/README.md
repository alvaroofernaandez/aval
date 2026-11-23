# aval

Installer for [aval](https://github.com/alvaroofernaandez/aval), a personal agent harness. *Aval* is Spanish for a guarantee: nothing in it counts as done without evidence. It sets up Claude Code and OpenCode (instructions, skills, hooks, memory) plus a terminal environment (tmux, yazi, Ghostty, zsh) on macOS, Linux and Windows.

## Use it

```sh
npx @alvaroofernaandez/aval install
```

No clone is needed. `npx` always resolves the latest published version, so this is the way to stay current: every push to `main` publishes a new build.

To keep the command around:

```sh
npm install -g @alvaroofernaandez/aval
aval install
```

A globally installed copy does **not** update itself, because npm never pushes updates to a machine. The binary checks for a newer version when it starts and tells you when there is one (turn that off with `aval config set update_check off`):

```sh
npm update -g @alvaroofernaandez/aval
```

## First run

1. **Welcome.** If aval cannot find its repository, it offers to clone it to `~/.aval` or to use a folder you already have. Cloning is always your choice.
2. **Select.** The TUI detects your platform and offers only what applies. Each group shows what will happen to its files before anything is written: new, linked, or replaces a file.
3. **Install.** Failures appear inline, and `ctrl+c` stops after the current step.
4. **Done.** Counts, the backup path, `aval restore latest` to undo, and next steps.

- **macOS / Linux**: everything, symlinked into `$HOME`.
- **Windows**: Claude Code, OpenCode and git natively; the terminal environment is shown greyed out with the reason, and installed in full if WSL is detected.

Existing files are never deleted. Anything in the way is moved to `~/.aval-backup/<timestamp>/` first, and re-running is a no-op for everything already linked.

## Commands

`aval` on its own prints the usage and changes nothing. After the first completed install the repository location is recorded, so later commands work from any directory. `$AVAL_REPO` and `--repo <path>` override it.

```sh
aval install              # interactive TUI
aval install --dry-run    # show what would happen, change nothing
aval install --yes        # install everything applicable, no TUI
aval update       # pull the repo and re-link what is new
aval doctor       # check links, hooks, agents and settings (--fix, --json)
aval sync         # merge repo-managed settings into ~/.claude/settings.json
aval restore      # undo an install or sync from ~/.aval-backup
aval uninstall    # remove aval's links and give back what they displaced
aval --version
```

## Configuration

Settings live in `${XDG_CONFIG_HOME:-~/.config}/aval/config`, as `key = value` lines. Precedence: flag, then environment variable, then file, then default. `NO_COLOR` is honoured.

| Key | Values | Default | Env | Flag |
| :--- | :--- | :--- | :--- | :--- |
| `repo` | path | none | `AVAL_REPO` | `--repo` |
| `theme` | `aval`, `terminal`, `mono`, or a community theme | `aval` | `AVAL_THEME` | `--theme` |
| `background` | `auto`, `dark`, `light` | `auto` | `AVAL_BACKGROUND` | |
| `glyphs` | `auto`, `unicode`, `ascii` | `auto` | `AVAL_GLYPHS` | |
| `motion` | `auto`, `off` | `auto` | `AVAL_MOTION` | |
| `update_check` | `on`, `off` | `on` | `AVAL_NO_UPDATE_CHECK` | |

```sh
aval config               # settings view on a terminal, list otherwise
aval config set theme nord
aval install --plain      # mono theme, ASCII glyphs, no motion
```

The default `aval` theme is plain ink with colour only on the state glyphs (`√` `!` `×`). Community themes are opt-in: catppuccin, tokyo-night, gruvbox, dracula, nord, kanagawa, rose-pine, solarized, one-dark and everforest, each with its light and dark variants where upstream has them. The full table is in the [repository README](https://github.com/alvaroofernaandez/aval#configuration).

## How the binary is delivered

Each platform's binary ships as a separate package, pulled in through `optionalDependencies` with `os`/`cpu` constraints — the esbuild pattern. npm downloads only the one that matches your machine.

There is **no postinstall script**. Nothing executes at install time, so this works under `npm ci --ignore-scripts` and in environments that forbid install scripts.

## Without Node

```sh
go install github.com/alvaroofernaandez/aval/cmd/aval@latest
```

Or download the binary for your platform from the
[releases page](https://github.com/alvaroofernaandez/aval/releases) (checksums in `SHA256SUMS`).

## License

MIT

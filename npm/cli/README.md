# aval

Installer for [aval](https://github.com/alvaroofernaandez/aval), a personal agent harness. *Aval* is Spanish for a guarantee: nothing in it counts as done without evidence. It sets up Claude Code and OpenCode (instructions, skills, hooks, memory) plus a terminal environment (tmux, yazi, Ghostty, zsh) on macOS, Linux and Windows.

## Use it

```sh
npx @alvaroofernaandez/aval
```

`npx` always resolves the latest published version, so this is the way to stay current: every push to `main` publishes a new build.

## Or install it

```sh
npm install -g @alvaroofernaandez/aval
aval
```

A globally installed copy does **not** update itself — npm never pushes updates to a machine. The binary checks for a newer version when it starts and tells you when there is one:

```sh
npm update -g @alvaroofernaandez/aval
```

## What it does

Opens a TUI that detects your platform and offers only what applies to it:

- **macOS / Linux** — everything, symlinked into `$HOME`.
- **Windows** — Claude Code, OpenCode and git natively; the terminal environment is shown greyed out with the reason, and installed in full if WSL is detected.

Existing files are never deleted. Anything in the way is moved to `~/.aval-backup/<timestamp>/` first, and re-running is a no-op for everything already linked.

## Commands

```sh
aval              # interactive TUI
aval --dry-run    # show what would happen, change nothing
aval --yes        # install everything applicable, no TUI
aval update       # pull the repo and re-link what is new
aval --version
```

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

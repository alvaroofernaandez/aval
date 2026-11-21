# Dev mode

`aval dev` lets you try aval without touching your machine: not your `HOME`, not `~/.claude`, not your config, not your projects. It builds **this** checkout inside a sandbox and runs everything there.

## Five commands

| Command | What it does |
| :--- | :--- |
| `aval dev` | Opens your `$SHELL` inside the sandbox (prompt `[aval dev:default]`). The first time, it runs `aval --yes` and `aval sync` there. |
| `aval dev run -- <cmd>` | A single command in the same environment: `aval dev run -- aval doctor`. Returns its exit code. |
| `aval dev diff` | What `aval --yes` and `aval sync` would change on your **real** machine. Read-only: it is the gate between "works in dev" and "safe to apply". |
| `aval dev project <path>` | Clones a real project into the sandbox so you can test hooks and commands against real code. |
| `aval dev ls` · `reset` · `rm` | Lists, recreates from scratch, or deletes sandboxes. |

`--name N` picks a different sandbox (default: `default`). They live in `${XDG_CACHE_HOME:-~/.cache}/aval/dev/<name>`.

## What is isolated

Inside the sandbox, each of these variables points to a folder in the sandbox:

`HOME` (and `USERPROFILE`, `APPDATA`, `LOCALAPPDATA` on Windows) · `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME` · `CLAUDE_CONFIG_DIR` · `CODEX_HOME` · `GEMINI_CLI_HOME` · `AVAL_PRIVATE_DIR` · `ENGRAM_DATA_DIR` (and its own `ENGRAM_PORT`) · `GIT_CONFIG_GLOBAL` (only `user.name` and `user.email`) · `TMPDIR` and `TMUX_TMPDIR` · `ZDOTDIR`.

In addition, `TMUX`, `GIT_DIR`, `GIT_WORK_TREE`, every `AVAL_*` and every `ENGRAM_*` variable (including the cloud token) are removed, and `PATH` starts with the sandbox's `bin`. `AVAL_DEV=1` marks that you are inside.

The sandbox installs from a **copy** of the repo (without `.git`), so the symlinks point into the sandbox and nothing can write to your checkout. Go caches are shared on purpose (they are content-addressed).

## Guarantees

- It refuses to create the sandbox if its path falls inside `~/.claude`, `~/.ssh`, `~/.gnupg`, `~/.codex`, `~/.gemini`, `~/.engram`, `~/.aval-backup` or `~/.config/aval/private`, or if it contains your `HOME`.
- Before launching anything, it checks that every isolated variable points inside the sandbox.
- `aval dev` inside a dev shell is rejected; `--nested` allows it.
- `dev project` uses `git clone --local --no-hardlinks`, only reads the original, and removes the remote from the clone. It does not copy uncommitted changes.

## Limits

- The network is not blocked: `git fetch`, `npm`, `gh` and hooks that call out still go out. Environment tokens (`GH_TOKEN`, `SSH_AUTH_SOCK`) are inherited.
- The sandbox has no `.git`: `aval update` does not work inside it. It rebuilds on every entry; the repo is copied again, but what is already installed is updated with `aval --yes` or `aval dev reset`.
- Docker, Homebrew, the macOS Keychain and LaunchAgents are not isolated.

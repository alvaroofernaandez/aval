# Dev mode

`aval dev` lets you try aval without touching your machine: not your `HOME`, not `~/.claude`, not your config, not your projects. It builds **this** checkout inside a sandbox and runs everything there.

## Five commands

| Command | What it does |
| :--- | :--- |
| `aval dev` | Opens your `$SHELL` inside the sandbox (prompt `[aval dev:default]`). The first time, it runs `aval install --yes` and `aval sync` there. |
| `aval dev run -- <cmd>` | A single command in the same environment: `aval dev run -- aval doctor`. Returns its exit code. |
| `aval dev diff` | What `aval install --yes` and `aval sync` would change on your **real** machine. Read-only: it is the gate between "works in dev" and "safe to apply". |
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
- The sandbox has no `.git`: `aval update` does not work inside it. It rebuilds on every entry; the repo is copied again, but what is already installed is updated with `aval install --yes` or `aval dev reset`.
- Docker, Homebrew, the macOS Keychain and LaunchAgents are not isolated.

## `aval dev check`: what CI runs, locally

`aval dev check` runs, in order and with the same pinned tools, every quality step CI runs. The steps are one table, `internal/devmode/check/table.go`; `ci.yml` builds the binary and calls this command, so a green here is a green there. A test fails if a quality command shows up in `ci.yml` outside the table.

| Step | Runs | Where | `--fast` |
| :--- | :--- | :--- | :---: |
| `gofmt` | `gofmt -l .` (any output fails) | all | yes |
| `vet` | `go vet ./...` | all | yes |
| `test` | `go test ./... -race` | all; **no `-race` on Windows** (a table rule) | yes, as `go test ./... -short` |
| `npm` | `node --test npm/cli/lib/*.test.js` | all | yes |
| `actionlint` | `actionlint` over `.github/workflows` | all | yes |
| `agents` | `npx -y agnix@<pin> validate .` (needs the network) | not Windows | no |
| `gitleaks` | `gitleaks detect --source .` over the whole history | all | no |

```
aval dev check [--fast] [--only NAME] [--list] [--keep-going]
```

- `--list` prints every step with its tool, pinned version, the systems it runs on and whether it is in the fast set. Nothing runs.
- `--fast` is the edit loop: it leaves out the race detector, the agents lint and gitleaks, names what it skipped, and says that green is then not the CI verdict.
- `--only NAME` runs one step; an unknown name lists the valid ones. `--keep-going` runs every step even after one fails.
- The exit code is the failing step's own (the first one, with `--keep-going`). A failing step prints its captured output before the table; the last line is the verdict (`green` or `red`, `verde` or `rojo` in Spanish) with counts and total time.

**Pinned tools.** Versions live in one place, the constants of `internal/devmode/check/table.go` (Go `1.25`, Node `22`, actionlint `v1.7.12`, gitleaks `v8.30.1`, agnix `0.61.0`); `ci.yml`'s setup and install steps must carry the same values, which a test checks. Before running anything the command looks for every tool the steps need and, if one is missing, prints the exact command and installs nothing:

```
error: missing tools:
  actionlint v1.7.12: go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
```

**Changing a step.** Edit the table (`Name`, `Tool`, `Args`, `OnlyOS`/`SkipOS`, `Fast`, `FastArgs`, `OSArgs`); CI picks it up without touching the YAML. A new tool also needs its pin and an install line in `ci.yml`.

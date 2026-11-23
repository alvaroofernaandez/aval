# Changelog

All notable changes to aval. Versions follow [SemVer](https://semver.org): `vX.Y.Z`,
with `-rc.N` / `-beta.N` prereleases (see [docs/releasing.md](docs/releasing.md)).

Each release has hand-written notes in `docs/releases/`, in English (`vX.Y.Z.md`) and
Spanish (`vX.Y.Z.es.md`). The GitHub release body is those notes followed by the commit
list GoReleaser generates from conventional commits (see [docs/releasing.md](docs/releasing.md)).
This file keeps a short, dated summary per release and links the notes; the section
"Before v0.1.0" is the history before versions existed.

## v0.2.3 (2026-11-23)

[Release notes](docs/releases/v0.2.3.md) · [Notas en español](docs/releases/v0.2.3.es.md)

### Fixes

- **design:** the render reminder of the Stop hook blocks once per UI edit on Linux too

### Maintenance

- **deps:** a dependency budget in `docs/dependencies.md`, kept true by a test; two unused OpenCode npm dependencies removed (#58)
- **arch:** a one-page `ARCHITECTURE.md`, kept true by a test (#57)
- **code:** size and complexity limits with an allow-list that can only shrink (#59)

## v0.2.2 (2026-11-23)

[Release notes](docs/releases/v0.2.2.md) · [Notas en español](docs/releases/v0.2.2.es.md)

### Fixes

- **lessons:** trigger globs always compile, match non-ASCII literals, and bodies keep whole characters (#45, lessons slice)
- **sync:** permission globs match non-ASCII literals

## v0.2.1 (2026-11-23)

[Release notes](docs/releases/v0.2.1.md) · [Notas en español](docs/releases/v0.2.1.es.md)

### Fixes

- **release:** `aval release promote` tags the rc commit after `main` has moved on, refuses an rc that is not on `origin/main`, and lists the commits after the rc with a warning when one changes shipped code (#79)

## v0.2.0 (2026-11-23)

[Release notes](docs/releases/v0.2.0.md) · [Notas en español](docs/releases/v0.2.0.es.md)

First published as `v0.2.0-rc.1` the same day and checked by hand; the stable release ships the same code.

### Breaking changes

- **cli:** bare `aval` no longer installs; `aval install` is the explicit subcommand. The flag-only form still installs with a deprecation notice, and unknown commands fail

### Features

- **deck:** `aval deck build|check|example` renders a plan as a local PDF deck; the plan gate denies edits until the active plan has a deck, and the `plan-deck` skill teaches agents to write it
- **cli:** the repository resolves from `--repo`, `AVAL_REPO`, the location `aval install` records, the working directory or the executable

- **hub:** bare `aval` on a terminal opens a hub with the wordmark, a status panel (version from the update cache, repo, doctor, last backup, theme) and nine views in four groups: Install, Update, Sync, Doctor, Backups, Uninstall, Settings, Dev sandbox and About. Opening it writes nothing and makes no network calls
- **hub:** every write goes through preview, confirm, run and result; destructive confirms open on Cancel and name their safety net
- **hub:** interactive doctor with safe fixes, backups with a typed restore preview, an uninstall preview, an update view that fetches a preview and re-applies, and a sync preview
- **ui:** terminal wordmark generated from the brand geometry, with a Unicode and an ASCII tier
- **ui:** installer overhaul: select screen previews what happens to each file, inline failures with a graceful stop, a done screen with evidence and undo, and a help overlay generated from the key map
- **ui:** welcome screen that finds or clones the repository
- **ui:** shared frame, lockup and breakpoints; the colour is on the state glyph only
- **ui:** themes (aval brand, terminal, mono and community themes) and a settings view with live preview
- **i18n:** the hub, the installer and the human output of the CLI are translated into English and Spanish; `language = auto|en|es`, `--lang` and `AVAL_LANG` choose it, falling back to `LC_ALL`, `LC_MESSAGES` and `LANG`. `--json`, hooks, the statusline, logs and generated files stay English. Text that comes from the core packages (doctor check messages, plan labels, sync warnings, install error reasons) is not translated yet
- **config:** `aval config list|get|set|path`; theme, glyph, motion and update settings reach every surface
- **cli:** grouped, branded usage with a first-run hint, and styled status output for doctor and headless install
- **backup:** runs record their origin
- **hook:** `aval hook rate-guard` denies a Bash batch (loop, xargs/parallel, `-P`, more than 3 URLs or a URL list) of HTTP requests to hosts that ban the client IP after rapid requests, and is registered for Bash tool calls

### Fixes

- **install:** quitting `aval install` writes nothing, and without a terminal it stops with a clear error
- **cli:** sync rollback reports its backup id
- **ui:** safety lines wrap instead of truncating, the uninstall preview scrolls long lists, hub spinners honour the motion setting, and re-apply commits its backup run when it closes

## Before v0.1.0

The moving `latest` tag and its "Latest build" release, which every push to `main` used to
update, were deleted when v0.1.0 shipped. Only annotated `vX.Y.Z` tags remain.

### Breaking changes

- group internal packages by domain and move skills to the root
- port the bash test suites to go test
- port installers, scripts, skill helpers and doc generators to Go
- remove vendored upstream (gstack, p87, ui-ux-pro-max)
- **installer:** rename the installer to aval

### Features

- **update:** notice follows the build's npm channel; dev builds never nag
- **cli:** aval version and aval release (status, next, tag, patch|minor|major|rc|beta|promote)
- **release:** SemVer model, conventional-commit bumps, notes and guarded tag cutting
- **tmux:** port tmux helpers and Ghostty launcher to aval tmux
- **herdr:** diff-review mod - comment on diff lines and send them to the agent
- **hooks:** port Claude hooks and design-ack to aval hook subcommands
- **design:** native aval design gate and design ack commands
- **herdr:** version herdr-app keys; cmd-shift-g opens lazygit, cmd-shift-o the branch PR
- **claude:** aval statusline and SessionStart repo-profile hook
- **zsh:** wt (worktree per task with its own .env and port) and forrepos
- **claude:** /fix-ci, /pr-feedback, /debug and /gentests commands
- **install:** verify the downloaded binary against SHA256SUMS before running it
- **update:** add --dry-run preview of managed paths and manifest changes
- **doctor:** add --security checks with allow-list suppression
- **security:** deny secret reads and make the overlay unable to narrow denies
- **cli:** aval restore and aval uninstall
- **review:** aval review assess sizes a review from git alone
- **sync:** transactional sync with automatic restore and crash recovery
- **budget:** aval budget reports always-on context and hook latency
- **backup:** run directories with manifest, restore, pin and retention
- **lessons:** path-triggered lessons hook instead of always-on rules
- **sync:** add --check drift mode and warn on it in pre-push
- **cli:** aval doctor and aval sync commands
- **doctor:** offline health checks for links, hooks, agents, skills, plugins and settings
- **sync:** private overlay with a settings fragment and a links manifest
- **sync:** merge repo-managed Claude settings and mirror shared agent rules
- **macos:** version the productivity defaults
- **claude:** /progreso and /cierre commands
- **zsh:** up/down, waitfor, gp, rd and clear/exit typo aliases
- **macos:** copy every screenshot to the clipboard and keep the file
- **design:** verify UI work by rendering, with shot.mjs and a once-only Stop hook
- **design:** replace the transcript scan with a design-ack marker gate
- **omniwm:** drop the focus border and scroll columns with Hyper+Shift
- **omniwm:** version the config with Caps Lock as Hyper
- **tmux:** pick a Linux clipboard backend in the paste router
- **tmux:** render the status bar on Linux
- **brew:** version a Brewfile of everything installed on purpose
- **git:** refuse pushes that carry employer material or secrets
- **readme:** neutral grey logo cards with each logo adapted to them
- **brand:** present the hero as a brand lockup
- **brand:** rebuild the hero on a Swiss grid with the mark made of checks
- **brand:** rebuild the hero as a guilloché certificate
- **brand:** draw the aval mark and wordmark
- **scripts:** reap orphaned test processes left behind by killed runs
- **docker-space-optimizer:** fstrim the Kiri VM disk after pruning
- **kiri:** add idempotent kiri-up and bound the multiarch BuildKit cache
- **herdr:** version hc reviewer and implementer templates
- **skills:** add herdr skill and always-on Herdr workspace rule
- **skills:** replace OrbStack skills with Kiri
- **skills:** make Lightpanda the default headless browser
- **orca:** version Orca's declarative config, guard its secrets
- **docker:** make OrbStack the only Docker runtime
- **skills:** add readme-portal skill
- **commands:** add agy command to dispatch tasks to Antigravity CLI
- **skills:** sync gentle-ai shared skills and orchestration contracts
- **skills:** add mac-disk-cleanup skill
- **design:** ban native controls and vendor the refactor half of the pipeline
- **opencode:** port the design gate and share one blocking rule set
- **design:** replace the advisory pipeline with gates that measure
- **claude:** make the Laws of UX step 0 of the design pipeline
- **claude:** enforce the design pipeline with a PreToolUse gate
- **claude:** gate session end on the project's verify command
- **sdd:** challenge the premise and force alternatives before proposing
- **gstack:** vendor fifteen patched skills from the pin
- **gstack:** strip colliding doctrine out of upstream skills
- **npm:** publish the installer to npm on every push to main
- **installer:** cross-platform TUI for macOS, Linux and Windows
- **paste:** make cmd+v handle images and text alike
- **ghostty:** paste clipboard images with a cmd chord
- soften the diff colours and version Claude Code themes
- home-row and cycling shortcuts for moving between splits
- keep the sidebar tree in sync without depending on the shell
- version the remaining home configs
- version zshrc and terminal tool configs, with secrets split out
- guard against committed credentials, follow cwd in the sidebar
- open sidebar files in nano so Ctrl+X closes them
- close files opened from the sidebar with Alt+w or prefix+k
- coloured segment status bar with CPU, GPU, RAM, clock and date
- version Claude Code and OpenCode agent configuration
- add terminal environment dotfiles with tested tmux file sidebar

### Fixes

- **npm:** fail the package build when a platform binary is missing
- **tmux:** exec the editor in open-file windows so Linux panes report it
- **manifest:** drop the untracked config/opencode/plugins source
- **doctor:** a secret read at launch ("$(security ...)") is not an inline secret
- **install:** seed ~/.claude/settings.json as a file owned by aval sync, never a link
- **opencode:** mirror scoped lessons into AGENTS.md, since OpenCode has no lessons hook
- **doctor:** do not report support dirs as incomplete skills
- **agents:** use model aliases instead of retired or OpenCode-style IDs
- **design:** render with any installed Chromium browser; record what Lightpanda can and cannot draw
- **claude:** hide attribution, drop dead hooks, add rules for recurring failures
- **readme:** put the content hash in image file names
- **readme:** version image URLs by content hash
- **pins:** move pins out of vendor/, which Go reserves
- **kiri-health:** detect Docker Desktop's socket helper hijacking /var/run/docker.sock
- **tests:** bound the yazi introspection probe so it cannot hang in a headless pty
- **design:** scan tool inputs for the design-pipeline checklist
- **claude:** restore owner rules and move gentle-ai orchestration on demand
- **design:** close the user-authored checklist hole and un-rot ui-ux-pro-max
- **claude:** the design gate also scans subagent transcripts
- **claude:** disable the UX plugin that collides with frontend-design
- **tests:** give the tmux e2e blocks their own socket, not a killed one
- **tmux:** only set the Homebrew shell when that binary exists
- **tests:** stop the runner hanging on a suite that leaves a process behind
- **ci:** install gitleaks from its declared module path
- **ci:** install the dotfiles before running the shell suites
- **ci:** install sd, and fail loudly when a test tool is missing
- **npm:** pass publish paths as ./dir and skip existing versions
- **paste:** resolve tmux by path so the router survives run-shell
- **terminal:** restore Option characters and clipboard copy
- give each Ghostty tab its own tmux session
- each window's sidebar starts in its own directory
- reference the Engram token by env var instead of embedding it
- enable tmux passthrough for yazi, open files in a new window
- resolve the sidebar command by absolute path so the pane survives
- resolve tmux by absolute path in scripts, add clickable sidebar button
- move sidebar off M-e, which a Spanish layout never delivers
- resolve tmux by absolute path so Ghostty can start
- link skills individually and quarantine backups

### Performance

- **zsh:** cut interactive startup from 114 ms to 39 ms
- **claude:** disable the warp plugin and register the no-sdd suite
- **claude:** drop SDD hooks and stop re-indexing from blocking edits
- **claude:** move 35KB of always-on instructions into on-demand skills
- cut the per-session overhead by 75% for many-session use
- start tmux with Ghostty and cut status bar cost by 300x


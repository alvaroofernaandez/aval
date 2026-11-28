# guard: `aval hook guard`, a PreToolUse Bash guard for bypass mode

Issue: #24 (v0.3.0 · 01). Owner decision (2026-11-28, option A): the private
allow-list `~/.config/aval/private/guard.allow` is read ONLY after a rule has
matched, so the no-op path does no config read, no exec and no network.

Why: `config/claude/settings.json` sets `defaultMode: bypassPermissions`. The
only barrier against `rm -rf /` or `git push --force origin main` is prose in
CLAUDE.md. The guard is a deterministic net in Go, with no Node dependency.

## Behaviour

`aval hook guard` reads the PreToolUse JSON (`tool_name`, `cwd`,
`tool_input.command`), acts only on `Bash`, and writes
`hookSpecificOutput.permissionDecision = "deny"` plus a reason that names the
rule, the command and how to allow it. Allow is empty output, exit 0.

Pipeline: tokenizer (quotes, `;` `&&` `||` `|`, `$(...)`, backticks, heredocs,
redirections) -> unwrapper (`sh -c`, `bash -c`, `eval`, `env`, `sudo`, `nohup`,
`xargs`, `command`, `time`, up to 4 nested levels) -> rules over `[]string`.

Rules: rm outside the project and home roots (and `/`, `~`, `$HOME`, `.`), force
push to main/master, DROP DATABASE / TRUNCATE on a non-local host, docker
volume|system prune, kubectl delete, download piped into a shell, secrets read
through Bash (`~/.ssh`, `~/.aws`, `.env*` except `.env.example`).

Failure mode: a parse failure re-runs the rules on a lenient split (quotes
ignored) and denies only if a rule matches; a panic or any internal error
allows.

Allow-list: one full command per line, `#` comments; an entry matches the whole
command after whitespace normalisation, never a prefix. Read only after a match.

## Tasks

- [x] **G1 tokenizer**: simple commands, pipes, substitutions, heredocs, redirections, lenient mode.
- [x] **G2 unwrapper**: wrappers peeled, `sh -c` recursion bounded at depth 4.
- [x] **G3 rules**: rm, git push, SQL, docker/kubectl, curl|sh, secrets (AC-1..AC-6).
- [x] **G4 decision**: allow-list after a match, reason text, fail closed / open (AC-7..AC-10).
- [x] **G5 hook**: `Run`, `aval hook guard`, settings.json entry, hot-path tests.
- [x] **G6 budget**: `aval budget` measures the guard against 10 ms (AC-11), benchmark over 200 commands.
- [x] **G7 docs**: ADR 0009 (amends the hook rule: a matched deny may read the allow-list), AGENTS.md amendment, guide.md, CHANGELOG.

## Decisions taken

- Allow-list read after a match only (owner, option A).
- Unknown `$VAR` in an rm target is not resolved and is allowed (a false positive on every `rm -rf "$BUILD"` costs more than the gap).
- `.env.example`, `.env.sample`, `.env.template`, `.env.dist` are allowed; `.envrc` is not.
- A bare `git push --force` (no refspec) is allowed: the branch is unknown without running git.

## Not in scope

Sandboxing, semantic SQL analysis, tools other than Bash.

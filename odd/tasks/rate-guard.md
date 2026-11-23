# rate-guard: stop rapid-fire HTTP to hosts that ban

Status: approved by the owner on 2026-11-23. Requested by another session.

Why: a session sent about 45 back-to-back `curl` requests while crawling innohelp.es. Helice's protection then banned the office's public IP: ports 443 and 80 are refused, while ping still answers. The same server (142.93.110.132) serves innohelp.es, www.innohelp.es, innohelp.helice.app and www.helice.app.

The guard lives in aval, because `~/.claude/settings.json` is synced from `config/claude/settings.json`. A loose script there would be overwritten by the next sync.

## Behaviour

`aval hook rate-guard` reads the PreToolUse JSON on stdin. It acts only when `tool_name` is `Bash` and reads `tool_input.command`.

It denies the command when both of these hold:
- **The command contacts a guarded host** with an HTTP client: curl, wget, http/https (httpie), xh, aria2c, lynx, links, w3m, or wget2.
- **The command is a batch.** It counts as one when either is true:
  - (a) the request sits inside a loop or batch construct: `for`, `while`, `until`, `xargs`, GNU `parallel`, or a `-P` flag;
  - (b) the command names more than 3 URLs on guarded hosts, or pipes or reads a URL list into the client (`-i`, `--input-file`, `-K`, `--config`, `xargs`).

Everything else passes:
- One or a few one-off requests, such as `curl -sL https://www.innohelp.es/`.
- Loops that only contact hosts that are not guarded.

### Deny

A deny writes `hookSpecificOutput.permissionDecision = "deny"` with this reason:

> Helice (innohelp.es / helice.app) bans the IP after rapid requests. Do not crawl it: use the Helice panel via Claude in Chrome, or make 1-2 spaced requests.

For a guarded host the user added, the reason names that host and gives the same advice.

### Guarded hosts

A host matches itself and all of its subdomains. The list is the union of three sources:
- **Built-in seed:** `innohelp.es` and `helice.app`.
- **A file:** `${XDG_CONFIG_HOME:-~/.config}/aval/rate-guard-hosts` on Unix, `%APPDATA%\aval\rate-guard-hosts` on Windows. One host per line; `#` starts a comment.
- **The env var `AVAL_RATE_GUARD_HOSTS`:** a comma-separated list.

The hook never loads `~/.config/aval/config`. Hooks are hot paths, and `TestHotPathsNeverReadConfig` keeps it that way.

### Failure mode

The hook fails open. Bad JSON, a non-Bash tool, an unreadable host file or any internal error all produce empty output and exit 0. A bug in the hook never blocks work.

### Speed

The hook must stay under the existing hook budget, the 10 ms measured for `lessons`. It does no network access and no exec.

## Tasks

- [x] **R1 (opus): `internal/agent/rateguard` package.**
  - API: `Decide(command string, hosts []string) (deny bool, reason string)`, plus `Hosts(getenv, readFile)`.
  - RED cases, as one table test:
    - deny: a for loop with curl on innohelp.es;
    - deny: `cat urls | xargs curl` on helice.app;
    - deny: 4 innohelp URLs in one curl;
    - deny: a while-read loop with wget;
    - deny: `parallel` or `xargs -P` on a subdomain such as innohelp.helice.app;
    - allow: a single curl;
    - allow: 3 URLs in one curl;
    - allow: a loop over an unrelated host;
    - allow: `notinnohelp.es` (lookalike domain);
    - allow: a guarded host mentioned in an echo or grep with no HTTP client;
    - allow: an empty command.
  - Also tested: host parsing for the seed, the file (with comments) and the env var; subdomain matching.
- [x] **R2 (sonnet, after R1): `aval hook rate-guard` dispatch.**
  - Wire it in `cmd/aval/agent_hook.go`.
  - RED:
    - The exact deny JSON for a batch command.
    - Empty output on allow.
    - Fail-open: bad JSON, a non-Bash tool and an unreadable hosts file each give empty output and exit 0.
    - The hot-path test still passes.
    - A latency test in the style of `TestLessonsHookLatency`.
- [x] **R3 (sonnet, after R2): register the hook.**
  - Add a PreToolUse entry with matcher `Bash` in `config/claude/settings.json`, using the same `a=... || true` wrapper as `lessons`.
  - Add the OpenCode equivalent if it is cheap, following `config/opencode/plugin/design-gate.ts`.
  - Document it in docs/guide.md under hooks.
  - RED: a settings test asserting the entry exists with matcher `Bash`; follow existing tests over `config/claude/settings.json`.
- [x] **R4 (main session): sync and verify.**
  - Run `aval sync`.
  - Check that the live `~/.claude/settings.json` has the hook.
  - Pipe a sample payload into `aval hook rate-guard`.
  - Reply to the requesting session with the commit hash and how to verify.
  - Done on 2026-11-23: main at 3825044, binary rebuilt, synced, deny and allow verified.

## Later
- **False positive: inert quoted text.** A loop and a guarded URL inside a single-quoted string are denied even though nothing is fetched. One example is piping a test JSON payload through `printf` into the hook. It was reported by another session, and the main session hit it too while writing this very entry.
  - Fix: ignore single-quoted string contents, or require the HTTP client to be an actual command word.
- **False positive: `-P` as a flag value.** `wget -P dir` inside a loop counts as a batch, because any `-P` is treated as one.

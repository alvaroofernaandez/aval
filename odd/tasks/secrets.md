# secrets: `aval secrets` and the keychain migration of inline MCP tokens

Issue: #41 (v0.3.0 · 02). Closes #41.

Why: `aval doctor --security` only warns about an MCP server wired with a literal
token (`sec-mcp-inline-secrets`); the token stays in plaintext JSON that ends up
in backups and issues. `aval secrets` keeps tokens in the OS keychain and
`doctor --security --fix` moves the inline ones there.

## Behaviour

- `internal/core/secrets`: a `Store` port (`Get`, `Set`, `Delete`, `List`), an in-memory adapter for tests and one adapter per OS: macOS `security`, Linux `secret-tool`, Windows Credential Manager. No shell; the value never travels in a log or message.
- `aval secrets set NAME` (value from stdin or the TTY, never an argument), `get NAME [--show]` (without `--show` only the exit code), `list`, `rm NAME`, `exec NAME[,NAME2] -- cmd args` (value only in the child environment, real exec where the OS allows it, exit code and signals propagated).
- `aval doctor --security --fix`: each server with an inline value in `env` or `headers` has the value copied into the keychain and the entry rewritten to `aval secrets exec`. Backup first, atomic write, idempotent, no message ever carries a value.
- No keychain (Linux without Secret Service, Windows without access): a clear error, never a plaintext fallback.

## Tasks

- [ ] **S1 port**: `Store`, memory adapter, `Exec` environment builder (value only for the child).
- [ ] **S2 adapters**: macOS, Linux, Windows behind build tags; value over stdin where the binary allows it.
- [ ] **S3 CLI**: `aval secrets set|get|list|rm|exec`.
- [ ] **S4 migration**: `doctor --security --fix` with backup, atomic write, idempotent.
- [ ] **S5 docs**: ADR, guide, CHANGELOG.

## Decisions taken

- macOS `security add-generic-password` only takes the value as an argument or a prompt: `set` uses the prompt through `-w` with no value so it reads the TTY; documented as a limitation.
- Names are validated (`[A-Za-z0-9_.-]`) before they reach any adapter.

## Not in scope

Syncing secrets between machines, third-party managers, encrypting the private overlay.

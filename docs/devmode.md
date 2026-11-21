# Modo dev

`aval dev` prueba aval sin tocar tu máquina: ni tu `HOME`, ni `~/.claude`, ni tu config, ni tus proyectos. Compila **este** checkout dentro de un sandbox y ejecuta todo allí.

## Cinco órdenes

| Orden | Qué hace |
| :--- | :--- |
| `aval dev` | Abre tu `$SHELL` dentro del sandbox (prompt `[aval dev:default]`). La primera vez hace `aval --yes` y `aval sync` allí. |
| `aval dev run -- <cmd>` | Un solo comando en el mismo entorno: `aval dev run -- aval doctor`. Devuelve su código de salida. |
| `aval dev diff` | Qué cambiarían `aval --yes` y `aval sync` en tu máquina **real**. Solo lectura: es la puerta de "funciona en dev" a "es seguro aplicar". |
| `aval dev project <ruta>` | Clona un proyecto real dentro del sandbox para probar hooks y comandos con código de verdad. |
| `aval dev ls` · `reset` · `rm` | Lista, recrea desde cero o borra sandboxes. |

`--name N` elige otro sandbox (por defecto `default`). Viven en `${XDG_CACHE_HOME:-~/.cache}/aval/dev/<nombre>`.

## Qué queda aislado

Dentro del sandbox, cada una de estas variables apunta a una carpeta del sandbox:

`HOME` (y `USERPROFILE`, `APPDATA`, `LOCALAPPDATA` en Windows) · `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME` · `CLAUDE_CONFIG_DIR` · `CODEX_HOME` · `GEMINI_CLI_HOME` · `AVAL_PRIVATE_DIR` · `ENGRAM_DATA_DIR` (y un `ENGRAM_PORT` propio) · `GIT_CONFIG_GLOBAL` (solo `user.name` y `user.email`) · `TMPDIR` y `TMUX_TMPDIR` · `ZDOTDIR`.

Además se quitan `TMUX`, `GIT_DIR`, `GIT_WORK_TREE`, todo `AVAL_*` y todo `ENGRAM_*` (incluido el token de la nube), y `PATH` empieza por el `bin` del sandbox. `AVAL_DEV=1` marca que estás dentro.

El sandbox instala desde una **copia** del repo (sin `.git`), así que los enlaces (symlinks) apuntan al sandbox y nada puede escribir en tu checkout. Las cachés de Go se comparten a propósito (son por contenido).

## Garantías

- Se niega a crear el sandbox si su ruta cae dentro de `~/.claude`, `~/.ssh`, `~/.gnupg`, `~/.codex`, `~/.gemini`, `~/.engram`, `~/.aval-backup` o `~/.config/aval/private`, o si contiene tu `HOME`.
- Antes de lanzar nada comprueba que todas las variables aisladas apuntan dentro del sandbox.
- `aval dev` dentro de un shell dev se rechaza; `--nested` lo permite.
- `dev project` usa `git clone --local --no-hardlinks`, solo lee el original y quita el remoto del clon. No copia cambios sin confirmar.

## Límites

- La red no se bloquea: `git fetch`, `npm`, `gh` y los hooks que llamen fuera siguen saliendo. Los tokens del entorno (`GH_TOKEN`, `SSH_AUTH_SOCK`) se heredan.
- El sandbox no tiene `.git`: `aval update` no funciona dentro. Recompila con cada entrada; el repo se vuelve a copiar, pero lo ya instalado se actualiza con `aval --yes` o `aval dev reset`.
- No se aíslan Docker, Homebrew, el Keychain de macOS ni los LaunchAgents.

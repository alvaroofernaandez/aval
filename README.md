<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/hero-dark.6704cc71.svg">
  <img src="docs/assets/hero-light.100c6ed4.svg" alt="aval. Nada sin pruebas." width="100%">
</picture>

<br/>
<br/>

![macOS](https://img.shields.io/badge/-macOS-000000?style=flat-square&logo=apple&logoColor=white)
![Linux](https://img.shields.io/badge/-Linux-000000?style=flat-square&logo=linux&logoColor=white)
![Windows](https://img.shields.io/badge/-Windows-000000?style=flat-square&logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0id2hpdGUiPjxwYXRoIGQ9Ik0yIDJoOS41djkuNUgyek0xMi41IDJIMjJ2OS41aC05LjV6TTIgMTIuNWg5LjVWMjJIMnpNMTIuNSAxMi41SDIyVjIyaC05LjV6Ii8+PC9zdmc+&logoColor=white)

<br/>

[**Qué es**](#qué-es-aval) &nbsp;·&nbsp;
[**Garantías**](#garantías) &nbsp;·&nbsp;
[**Stack**](#stack) &nbsp;·&nbsp;
[**Instalar**](#instalar) &nbsp;·&nbsp;
[**Dentro**](#dentro) &nbsp;·&nbsp;
[**Guía completa →**](docs/guide.md)

</div>

<br/>

## Qué es aval

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/mark-dark.a2bb352f.svg">
  <img src="docs/assets/mark-light.22afeffc.svg" alt="Isotipo de aval: la a y el check en un solo trazo" width="96" align="right">
</picture>

aval es un **harness de ingeniería para agentes de IA**: las reglas, la
memoria, las skills, los hooks y los controles de calidad con los que Claude
Code y OpenCode trabajan, junto con el entorno de terminal donde se ejecutan.
Está versionado, cubierto por tests e instalable en macOS, Linux y Windows con
un solo comando.

Su principio está en el nombre: un aval es una **garantía**. Ninguna regla
depende de la buena voluntad del agente; cada una tiene detrás un mecanismo que
la verifica, ya sea un test, un hook o un control con resultados medidos. Por eso
la **v** del logotipo es un check.

<br/>

## Garantías

| Lo que se promete | Cómo se comprueba |
| :--- | :--- |
| Ningún secreto llega a git | `gitleaks` revisa el árbol **y todo el historial** en cada ejecución de los tests |
| Nada se implementa sin test | TDD estricto: el agente emite un `[tdd-gate]` con el test que falla antes de escribir código |
| Nada visual se aprueba a ojo | Un hook bloquea la edición de UI hasta pasar el pipeline de diseño; los gates **miden** el contraste |
| Una sola fuente de verdad | Los dos instaladores leen `install.manifest`; un test falla si divergen |
| Nada se borra al instalar | Lo que existiera se mueve a `~/.aval-backup/<fecha>/` antes de enlazar |
| El agente no olvida | Memoria persistente con Engram entre sesiones y compactaciones |

<br/>

## Stack

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/stack-dark.49b8b937.svg">
  <img src="docs/assets/stack-light.5a7a65d3.svg" alt="Ghostty, tmux, Zsh, Neovim, yazi, Herdr, Claude Code, OpenCode, Engram, RTK, gstack, Go, Node.js, npm, Bun, Python, Bash, Homebrew, Docker, Kiri, Lightpanda, Git, GitHub CLI, gitleaks, ripgrep, fd, bat, eza, fzf, zoxide, atuin, delta, macOS, Linux y Windows" width="100%">
</picture>

<sub>Logos oficiales de cada proyecto (<a href="docs/logos/SOURCES.md">procedencia</a>). ripgrep y delta no publican logo: llevan sus iniciales en vez de uno inventado.</sub>

<br/>

## Instalar

```sh
git clone https://github.com/alvaroofernaandez/aval.git ~/aval
cd ~/aval && ./install.sh --dry-run && ./install.sh
brew bundle --file=Brewfile   # macOS/Linux: todo lo instalado a propósito
```

Con Go instalado, el arranque de un comando compila la TUI multiplataforma:

```sh
curl -fsSL https://raw.githubusercontent.com/alvaroofernaandez/aval/main/scripts/install.sh | sh
aval update   # git pull + enlaza lo nuevo
aval doctor   # revisa enlaces, hooks, agentes y settings (--fix repara, --json para scripts)
aval sync     # vuelca lo que gestiona el repo en ~/.claude/settings.json y en Codex/Gemini (--dry-run)
```

Lo privado (nombres de empresa, skills de cliente) vive fuera del repo, en `~/.config/aval/private`
(o `$AVAL_PRIVATE_DIR`): un `settings.json` que `aval sync` fusiona encima y un `links.manifest` que enlaza.

> [!NOTE]
> `npx @alvaroofernaandez/aval` y los binarios precompilados se publican desde CI y aún no están disponibles con el nombre nuevo.

<br/>

## Dentro

| | Qué | Detalle |
| :--- | :--- | :--- |
| `config/claude` · `config/opencode` | Instrucciones, agentes, hooks, comandos | [Agentes](docs/guide.md#-agentes) |
| `shared/skills` | Skills compartidas por todos los agentes | [Skills](docs/guide.md#-skills-compartidas) |
| `config/ghostty` · `config/tmux` | Terminal, barra de estado de ~32 ms, sidebar de yazi | [Barra](docs/guide.md#-barra-de-estado) · [Sidebar](docs/guide.md#-barra-lateral-de-archivos) |
| `home/` | zsh, p10k, git | [Shell](docs/guide.md#-shell-y-herramientas) |
| `cmd/aval` · `internal/` · `npm/` | Instalador TUI en Go | [Instalación](docs/guide.md#-instalación) |
| `install.manifest` | Qué se instala y dónde: fuente única | [Mapa](docs/guide.md#️-mapa-del-repositorio) |
| `pins/` | Commit exacto de cada skill de terceros | [Agentes](docs/guide.md#-agentes) |
| `tests/` | Shell, Go y npm en una sola orden: `./tests/run-all.sh` | [Tests](docs/guide.md#-tests) |

<br/>

<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/wordmark-dark.3ba4fd49.svg">
  <img src="docs/assets/wordmark-light.8ec8a19c.svg" alt="aval" width="88">
</picture>

<sub>Imágenes generadas con <code>python3 docs/brand.py</code> · cada logo del stack es marca registrada de su proyecto</sub>

</div>

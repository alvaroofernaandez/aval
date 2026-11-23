package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The one place that pins the text a person reads. Everywhere else a test
// asserts which error happened (errors.Is, errors.As) or what was written, never
// the wording: the hygiene rule in test_hygiene_test.go fails on err.Error()
// comparisons outside this file.
//
// Every case runs the real binary in a throwaway HOME and compares stderr byte
// for byte, after the scratch paths are folded into placeholders: HOME for the
// home directory, DIR for the scratch directory the command runs in, CONFIG for
// aval's config file. tests/ imports nothing internal (ADR 0002), so the binary
// is the surface. Cases are grouped by surface, the unit a translation extends
// (odd/tasks/i18n.md): one table per surface, one row per message.
//
// The text inside the TUI (the welcome screen's path check, the dev screen) is
// pinned by the screen tests in internal/ui, which render the view.

// msgCase is one message: the command that prints it and the exact stderr.
type msgCase struct {
	name   string
	setup  func(t *testing.T, dir, home string) // fixtures under dir or home; nil for none
	args   []string                             // "DIR" in an argument is the scratch directory
	env    []string                             // extra KEY=VALUE; "DIR" in a value is the scratch directory
	stdin  string
	unix   bool // the command is Unix only: on Windows it prints the WSL refusal instead
	code   int
	stderr string
	stdout string // pinned exactly when set
}

// configFile is where aval keeps its settings under home, per OS.
func configFile(home string) string {
	// isoEnv sets XDG_CONFIG_HOME to home/.config on every OS, and the config
	// package honours it before APPDATA, so the file is here on Windows too.
	return filepath.Join(home, ".config", "aval", "config")
}

// runMessages runs each case and compares its stderr exactly.
func runMessages(t *testing.T, cases []msgCase) {
	t.Helper()
	aval := avalBin(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.unix && runtime.GOOS == "windows" {
				t.Skip("Unix only; the Windows refusal is pinned in TestMessagesPlatform")
			}
			dir, home := t.TempDir(), t.TempDir()
			if c.setup != nil {
				c.setup(t, dir, home)
			}
			sub := func(s string) string { return strings.ReplaceAll(s, "DIR", dir) }
			args := make([]string, len(c.args))
			for i, a := range c.args {
				args[i] = sub(a)
			}
			env := []string{"AVAL_NO_UPDATE_CHECK=1"}
			for _, e := range c.env {
				env = append(env, sub(e))
			}
			r := run(dir, isoEnv(home, env...), c.stdin, aval, args...)
			got := strings.NewReplacer(configFile(home), "CONFIG", dir, "DIR", home, "HOME").Replace(r.Stderr)
			got = filepath.ToSlash(osErrorText(got))
			eq(t, "exit code\n"+r.Out(), c.code, r.Code)
			eq(t, "stderr", c.stderr, got)
			if c.stdout != "" {
				out := strings.NewReplacer(configFile(home), "CONFIG", dir, "DIR", home, "HOME").Replace(r.Stdout)
				eq(t, "stdout", c.stdout, filepath.ToSlash(osErrorText(out)))
			}
		})
	}
}

// osErrorText folds the Windows wording of a missing path into the Unix one:
// aval prints the operating system's error verbatim, and the pins cover aval's
// words, not the C runtime's.
func osErrorText(s string) string {
	return strings.NewReplacer(
		"The system cannot find the path specified.", "no such file or directory",
		"The system cannot find the file specified.", "no such file or directory",
	).Replace(s)
}

// emptyDir creates DIR/<name> with nothing in it.
func emptyDir(name string) func(t *testing.T, dir, home string) {
	return func(t *testing.T, dir, _ string) {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMessagesUsage(t *testing.T) {
	runMessages(t, []msgCase{
		{name: "install with an unknown flag", args: []string{"install", "--force"}, code: 1,
			stderr: "error: usage: aval install [--dry-run] [--yes] [--copy]: flag provided but not defined: -force\n"},
		{name: "install with a stray argument", args: []string{"install", "extra"}, code: 1,
			stderr: "error: usage: aval install [--dry-run] [--yes] [--copy]: unexpected argument \"extra\"\n"},
		{name: "deck without a subcommand", args: []string{"deck"}, code: 1, stderr: "error: usage:\n  aval deck build <spec.json> -o <out.pdf> [--plan <odd/tasks/plan.md>] [--open]\n  aval deck check <spec.json>     validate the spec (layouts, required fields, text limits)\n  aval deck example               print a complete spec that uses every layout\n\nlayouts:\n  cards      2-4 side-by-side cards: title and text\n  checklist  2-6 things that are guaranteed or included\n  closing    last slide: title and an optional lede\n  compare    before and after: exactly 2 items\n  cover      first slide: title and an optional lede\n  flow       2-5 boxes joined by arrows, left to right\n  groups     1-4 groups: title, text and chips\n  section    divider between parts: title and an optional lede\n  statement  one big idea: title and lede\n  steps      2-5 numbered steps in order\n  table      2-4 columns and 1-8 rows of short cells\n  timeline   2-6 moments: tag is the time or label\n  tree       2-5 questions (title) leading to an answer (tag)\n\n"},
		{name: "design gate that does not exist", args: []string{"design", "gate", "nope"}, code: 1, stderr: "error: unknown gate \"nope\"\nusage:\n  aval design ack <surface>\n  aval design gate contrast <fg> <bg> [--large]\n  aval design gate tokens <file|dir>...\n  aval design gate pairs <colors.json> [--aaa]\n  aval design gate hardcodes [--ext .css,.tsx] [--no-tw] <path>...\n  aval design gate theme-refs --theme <theme.css>... <path>...\n  aval design gate component [--root <dir>]      (checks <dir>/components/*.md)\n  aval design gate emoji <path>...\n  aval design gate native <path>...              (native select/date/dialog/confirm)\n"},
		{name: "release without a kind", args: []string{"release"}, code: 1, stderr: "error: usage: aval release <command>\n\n  status                 latest tags, unreleased commits, next suggested version\n  patch | minor | major  cut a stable release\n  rc | beta              cut the next prerelease of the upcoming version\n  promote                turn the latest rc into the stable release (same commit)\n  environment <tag>      the GitHub environment the tag publishes from (used by CI)\n  npm-tag <tag>          the npm dist-tag the tag publishes under (used by CI)\n\n  cut commands take --yes (skip the prompt) and --dry-run (show only)\n"},
		{name: "deps with an unknown layer", args: []string{"deps", "--layer", "kernel"}, code: 1,
			stderr: "error: unknown layer \"kernel\"; use harness, agent or env\nusage: aval deps [--json] [--layer harness|agent|env]\n"},
		{name: "deps with an unknown layer, Spanish", args: []string{"--lang", "es", "deps", "--layer", "kernel"}, code: 1,
			stderr: "error: capa desconocida \"kernel\"; use harness, agent o env\nuso: aval deps [--json] [--layer harness|agent|env]\n"},
		{name: "config with an unknown subcommand", args: []string{"config", "bogus"}, code: 1, stderr: "error: unknown config command \"bogus\"; usage: aval config [list | get <key> | set <key> <value> | path]\n"},
		{name: "kiri without up", args: []string{"kiri"}, unix: true, code: 1, stderr: "error: usage: aval kiri up    (make Docker ready on Kiri; idempotent, deletes nothing)\n"},
		{name: "docker without cleanup", args: []string{"docker"}, unix: true, code: 1, stderr: "error: usage: aval docker cleanup [--dry-run]    (safe prunes on the kiri context only)\n"},
		{name: "pdf without a subcommand", args: []string{"pdf"}, code: 1, stderr: "error: usage:\n  aval pdf init <project-dir> [asset flags]    scaffold a PDF report project\n  aval pdf assets [project-dir] [asset flags]  build <project-dir>/assets/brand.css\nasset flags:\n  --brand-dir <dir>   logo, banner.png and <name>.* portraits (default: brand)\n  --people a,b        portrait names\n  --logo-file <name>  logo inside --brand-dir (default: logo.png)\n  --ink r,g,b         dark logo colourway (default: 23,20,31)\n  --accent r,g,b      branded logo colourway (default: 110,59,106)\n"},
	})
}

func TestMessagesPlatform(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the refusal of a Unix-only command prints on Windows only")
	}
	runMessages(t, []msgCase{
		{name: "kiri on windows", args: []string{"kiri"}, code: 1,
			stderr: "error: aval kiri is not supported on windows: Kiri is a macOS Docker runtime. Use WSL for it\n"},
	})
}

func TestMessagesConfig(t *testing.T) {
	unknownKey := func(t *testing.T, _, home string) { write(t, configFile(home), "bogus = 1\n") }
	const valid = "repo, theme, background, glyphs, motion, update_check, language"
	runMessages(t, []msgCase{
		{name: "get an unknown key, Spanish", args: []string{"config", "get", "bogus"}, env: []string{"AVAL_LANG=es"}, code: 1,
			stderr: "error: clave de configuración desconocida \"bogus\". Válidas: " + valid + "\n"},
		{name: "config with an unknown subcommand, Spanish", args: []string{"config", "bogus"}, env: []string{"AVAL_LANG=es"}, code: 1,
			stderr: "error: comando de config desconocido \"bogus\"; uso: aval config [list | get <key> | set <key> <value> | path]\n"},
		{name: "config with --lang es", args: []string{"--lang", "es", "config", "bogus"}, code: 1,
			stderr: "error: comando de config desconocido \"bogus\"; uso: aval config [list | get <key> | set <key> <value> | path]\n"},
		{name: "list with an unknown key in the file", setup: unknownKey, args: []string{"config", "list"},
			stdout: "repo                  default\ntheme          aval   default\nbackground     auto   default\nglyphs         auto   default\nmotion         auto   default\nupdate_check   off    env\nlanguage       en     env\n\nunknown key \"bogus\" (file:1)\n"},
		{name: "list with an unknown key in the file, Spanish", setup: unknownKey, args: []string{"config", "list"}, env: []string{"AVAL_LANG=es"},
			stdout: "repo                  default\ntheme          aval   default\nbackground     auto   default\nglyphs         auto   default\nmotion         auto   default\nupdate_check   off    env\nlanguage       es     env\n\nclave desconocida \"bogus\" (file:1)\n"},
		{name: "get an unknown key", args: []string{"config", "get", "bogus"}, code: 1, stderr: "error: unknown config key \"bogus\". Valid: repo, theme, background, glyphs, motion, update_check, language\n"},
		{name: "set a misspelt theme", args: []string{"config", "set", "theme", "catpuccin"}, code: 1, stderr: "error: config: CONFIG: theme = \"catpuccin\" is not a theme. Valid: aval, aval-dark, aval-light, terminal, mono, catppuccin, catppuccin-mocha, catppuccin-latte, catppuccin-frappe, catppuccin-macchiato, tokyo-night, tokyo-night-storm, tokyo-night-day, gruvbox, gruvbox-dark, gruvbox-light, dracula, dracula-alucard, nord, kanagawa, kanagawa-wave, kanagawa-dragon, kanagawa-lotus, rose-pine, rose-pine-moon, rose-pine-dawn, solarized, solarized-dark, solarized-light, one-dark, one-light, everforest, everforest-dark, everforest-light. Did you mean catppuccin?\n"},
		{name: "set a theme by its prefix", args: []string{"config", "set", "theme", "tokio"}, code: 1, stderr: "error: config: CONFIG: theme = \"tokio\" is not a theme. Valid: aval, aval-dark, aval-light, terminal, mono, catppuccin, catppuccin-mocha, catppuccin-latte, catppuccin-frappe, catppuccin-macchiato, tokyo-night, tokyo-night-storm, tokyo-night-day, gruvbox, gruvbox-dark, gruvbox-light, dracula, dracula-alucard, nord, kanagawa, kanagawa-wave, kanagawa-dragon, kanagawa-lotus, rose-pine, rose-pine-moon, rose-pine-dawn, solarized, solarized-dark, solarized-light, one-dark, one-light, everforest, everforest-dark, everforest-light. Did you mean tokyo-night?\n"},
		{name: "a bad --theme", args: []string{"--theme", "tokio-night", "doctor"}, code: 1, stderr: "error: invalid --theme: theme = \"tokio-night\" is not a theme. Valid: aval, aval-dark, aval-light, terminal, mono, catppuccin, catppuccin-mocha, catppuccin-latte, catppuccin-frappe, catppuccin-macchiato, tokyo-night, tokyo-night-storm, tokyo-night-day, gruvbox, gruvbox-dark, gruvbox-light, dracula, dracula-alucard, nord, kanagawa, kanagawa-wave, kanagawa-dragon, kanagawa-lotus, rose-pine, rose-pine-moon, rose-pine-dawn, solarized, solarized-dark, solarized-light, one-dark, one-light, everforest, everforest-dark, everforest-light. Did you mean tokyo-night?\n"},
	})
}

// The language setting. isoEnv pins AVAL_LANG=en, so a row that wants
// detection drops it and sets the locale itself.
func TestMessagesLanguage(t *testing.T) {
	badFile := func(t *testing.T, _, home string) { write(t, configFile(home), "language = esp\n") }
	detectSpanish := []string{"AVAL_LANG", "LC_ALL", "LC_MESSAGES", "LANG=es_ES.UTF-8"}
	runMessages(t, []msgCase{
		{name: "a bad language in the file", setup: badFile, args: []string{"config", "list"}, code: 0,
			stderr: "warning: config: CONFIG:1: language = \"esp\" is not a valid language. Valid: auto, en, es. Did you mean es?\n"},
		{name: "a bad language in the file, Spanish locale", setup: badFile, args: []string{"config", "list"}, env: detectSpanish, code: 0,
			stderr: "warning: config: CONFIG:1: language = \"esp\" no es un idioma válido. Valores válidos: auto, en, es. ¿Quiso decir es?\n"},
		{name: "a bad language in the file, AVAL_LANG=es", setup: badFile, args: []string{"config", "list"}, env: []string{"AVAL_LANG=es"}, code: 0,
			stderr: "warning: config: CONFIG:1: language = \"esp\" no es un idioma válido. Valores válidos: auto, en, es. ¿Quiso decir es?\n"},
		{name: "set a misspelt language", args: []string{"config", "set", "language", "esp"}, code: 1,
			stderr: "error: config: CONFIG: language = \"esp\" is not a valid language. Valid: auto, en, es. Did you mean es?\n"},
		{name: "a bad --lang", args: []string{"--lang", "esp", "doctor"}, code: 1,
			stderr: "error: invalid --lang: language = \"esp\" is not a valid language. Valid: auto, en, es. Did you mean es?\n"},
		{name: "a bad --lang on config", args: []string{"--lang", "esp", "config", "list"}, code: 1,
			stderr: "error: invalid --lang: language = \"esp\" is not a valid language. Valid: auto, en, es. Did you mean es?\n"},
		{name: "a bad --lang on sync", args: []string{"--lang", "esp", "sync"}, code: 1,
			stderr: "error: invalid --lang: language = \"esp\" is not a valid language. Valid: auto, en, es. Did you mean es?\n"},
	})
}

func TestMessagesRepository(t *testing.T) {
	writeCfg := func(t *testing.T, dir, home string) {
		emptyDir("empty")(t, dir, home)
		write(t, configFile(home), "repo = "+filepath.Join(dir, "empty")+"\n")
	}
	badManifest := func(t *testing.T, dir, _ string) {
		write(t, filepath.Join(dir, "repo", "install.manifest"), "[g]\nlabel = L\nplatforms = solaris\n")
	}
	runMessages(t, []msgCase{
		{name: "--repo points at no checkout", setup: emptyDir("empty"), args: []string{"doctor", "--repo", "DIR/empty"}, code: 1, stderr: "error: --repo points at DIR/empty, which is not an aval repository (no install.manifest)\n"},
		{name: "AVAL_REPO points at no checkout", setup: emptyDir("empty"), args: []string{"doctor"}, env: []string{"AVAL_REPO=DIR/empty"}, code: 1, stderr: "error: $AVAL_REPO points at DIR/empty, which is not an aval repository (no install.manifest)\n"},
		{name: "the config points at no checkout", setup: writeCfg, args: []string{"doctor"}, code: 1, stderr: "error: repo in config CONFIG points at DIR/empty, which is not an aval repository (no install.manifest)\n"},
		{name: "no checkout anywhere", args: []string{"doctor"}, code: 1, stderr: "error: cannot find the aval repository (install.manifest): run 'aval install --repo <path>' once from your checkout, or set AVAL_REPO\n"},
		{name: "a manifest with an unknown platform", setup: badManifest, args: []string{"install", "--yes", "--repo", "DIR/repo"}, code: 1, stderr: "error: manifest: line 3: unknown platform \"solaris\"\n"},
	})
}

func TestMessagesInstall(t *testing.T) {
	missingSource := func(t *testing.T, dir, _ string) {
		write(t, filepath.Join(dir, "repo", "install.manifest"), "[g]\nlabel = G\ndetail = d\nplatforms = all\n\ngone    .gone\n")
	}
	runMessages(t, []msgCase{
		{name: "install without a terminal", setup: missingSource, args: []string{"install", "--repo", "DIR/repo"}, code: 1, stderr: "error: aval install needs a terminal. For scripts use: aval install --yes\n"},
		{name: "an action fails", setup: missingSource, args: []string{"install", "--yes", "--repo", "DIR/repo"}, code: 1, stderr: "error: 1 action(s) failed\n"},
		{name: "install without a terminal, Spanish", setup: missingSource, args: []string{"install", "--repo", "DIR/repo"}, env: []string{"AVAL_LANG=es"}, code: 1,
			stderr: "error: aval install necesita una terminal. Para scripts use: aval install --yes\n"},
		{name: "an action fails, Spanish", setup: missingSource, args: []string{"install", "--yes", "--repo", "DIR/repo"}, env: []string{"AVAL_LANG=es"}, code: 1, stderr: "error: falló 1 acción\n"},
	})
}

// A spec that breaks one rule per line of the message.
const badDeck = `{"title":"x","slides":[
{"layout":"cover","title":"Hello"},
{"layout":"cards","title":"","items":[{"title":"a","text":"` + "TEXT161" + `","tone":"red"},{"title":"b"},{"title":"c"},{"title":"d"},{"title":"e"},{"title":"f"}]},
{"layout":"cards","title":"` + "TITLE81" + `","items":[{"title":"a"},{"title":"b"}]},
{"layout":"timeline","title":"A day","items":[{"title":"a"},{"title":"b","tag":"t"}]},
{"layout":"table","title":"Decisions","columns":["a","b"],"rows":[["1","2"],["3"]]},
{"layout":"nope","title":"x"}
]}`

func TestMessagesDeck(t *testing.T) {
	spec := func(name, body string) func(t *testing.T, dir, home string) {
		return func(t *testing.T, dir, _ string) {
			body = strings.NewReplacer("TEXT161", strings.Repeat("x", 161), "TITLE81", strings.Repeat("x", 81)).Replace(body)
			write(t, filepath.Join(dir, name), body)
		}
	}
	tooMany := `{"title":"x","slides":[{"layout":"cover","title":"Hello"}` + strings.Repeat(`,{"layout":"section","title":"More"}`, 40) + `]}`
	good := `{"title":"x","slides":[{"layout":"cover","title":"Hello"}]}`
	runMessages(t, []msgCase{
		{name: "a spec with problems", setup: spec("bad.json", badDeck), args: []string{"deck", "check", "DIR/bad.json"}, code: 1, stderr: "error: the deck spec has problems:\n  slide 2 (cards): title is required\n  slide 2 (cards): needs 2 to 4 items, got 6\n  slide 2 (cards) item 1: text is 161 characters, the limit is 160\n  slide 2 (cards) item 1: tone \"red\" is not one of \"\", \"good\", \"warn\", \"muted\"\n  slide 3 (cards): title is 81 characters, the limit is 80\n  slide 4 (timeline) item 1: tag is required\n  slide 5 (table) row 2: has 1 cells, the table has 2 columns\n  slide 6: unknown layout \"nope\" (one of: cards, checklist, closing, compare, cover, flow, groups, section, statement, steps, table, timeline, tree)\n"},
		{name: "a spec with too many slides", setup: spec("many.json", tooMany), args: []string{"deck", "check", "DIR/many.json"}, code: 1, stderr: "error: the deck spec has problems:\n  a deck has at most 40 slides, this one has 41: split it\n"},
		{name: "a spec with an unknown field", setup: spec("typo.json", `{"title":"x","slides":[{"layout":"cover","title":"t","subtitle":"oops"}]}`),
			args: []string{"deck", "check", "DIR/typo.json"}, code: 1, stderr: "error: reading the deck spec: json: unknown field \"subtitle\"\n"},
		{name: "build without -o", setup: spec("good.json", good), args: []string{"deck", "build", "DIR/good.json"}, code: 1, stderr: "error: aval deck build needs -o <out.pdf>: where the deck is saved\n"},
		{name: "build to a non-PDF", setup: spec("good.json", good), args: []string{"deck", "build", "DIR/good.json", "-o", "DIR/x.png"}, code: 1, stderr: "error: the output must be a .pdf file, got DIR/x.png\n"},
		{name: "AVAL_CHROME points nowhere", setup: spec("good.json", good), args: []string{"deck", "build", "DIR/good.json", "-o", "DIR/x.pdf"},
			env: []string{"AVAL_CHROME=DIR/nope"}, code: 1, stderr: "error: AVAL_CHROME points at DIR/nope, which does not exist\n"},
	})
}

func TestMessagesUpdate(t *testing.T) {
	needTool(t, "git")
	dirty := func(t *testing.T, dir, _ string) {
		origin, clone := filepath.Join(dir, "origin.git"), filepath.Join(dir, "repo")
		git(t, dir, "init", "-q", "--bare", origin)
		git(t, dir, "clone", "-q", origin, clone)
		write(t, filepath.Join(clone, "install.manifest"), "[g]\nlabel = G\ndetail = d\nplatforms = all\n")
		git(t, clone, "add", "-A")
		git(t, clone, "commit", "-q", "-m", "init")
		git(t, clone, "push", "-q", "origin", "HEAD")
		write(t, filepath.Join(clone, "local.txt"), "edit\n")
	}
	runMessages(t, []msgCase{
		{name: "update over uncommitted changes", setup: dirty, args: []string{"update", "--repo", "DIR/repo"}, code: 1, stderr: "error: update: the repository has uncommitted changes; commit or stash them first\n"},
		{name: "a dry run over uncommitted changes", setup: dirty, args: []string{"update", "--dry-run", "--repo", "DIR/repo"}, code: 1, stderr: "error: update: the repository has uncommitted changes; commit or stash them first\n"},
	})
}

func TestMessagesTools(t *testing.T) {
	noBlock := func(t *testing.T, dir, _ string) {
		write(t, filepath.Join(dir, "repo", "install.manifest"), "[g]\nlabel = G\ndetail = d\nplatforms = all\n")
		write(t, filepath.Join(dir, "repo", "config", "claude", "CLAUDE.md"), "no markers here\n")
	}
	runMessages(t, []msgCase{
		{name: "rules sync without the shared block", setup: noBlock, args: []string{"rules", "sync", "--repo", "DIR/repo"}, code: 1, stderr: "error: rules sync: no block found in DIR/repo/config/claude/CLAUDE.md\n"},
		{name: "pdf assets with a missing brand dir", setup: emptyDir("project"), args: []string{"pdf", "assets", "DIR/project", "--brand-dir", "DIR/gone"}, code: 1, stderr: "error: Brand image directory not found: DIR/gone\n"},
		{name: "dev inside a dev shell", args: []string{"dev"}, env: []string{"AVAL_DEV=1"}, code: 1, stderr: "error: already inside an aval dev shell (AVAL_DEV=1); exit it first, or pass --nested to stack another sandbox\n"},
	})
}

// The usage text. The version is "dev": the suite builds the binary without
// release stamps.
const usageEN = `aval dev
Nothing passes without evidence.

aval installs and maintains this machine's dotfiles and agent configs.

Usage:
  aval <command> [flags]

Setup
  aval install            link the repo's configs into HOME (TUI; --yes for none)
                            [--dry-run] [--yes] [--copy]
  aval update             pull the repo, then re-apply
                            [--dry-run]
  aval sync               merge repo-managed settings into the live config
                            [--dry-run]
  aval config             show or change aval settings (theme, glyphs, motion)
                            [list | get <key> | set <key> <value> | path]
  aval dev                sandboxed HOME for developing aval
                            [run | diff | reset | ls | rm | project]

Health
  aval doctor             check this machine against the repo
                            [--fix] [--json]
  aval budget             always-on context per agent and hook latency
                            [--json]
  aval deps               external tools aval relies on, and which are missing
                            [--json] [--layer harness|agent|env]
  aval version            version, commit, build date and channel
  aval help               show this help

Undo
  aval restore            list or restore backup runs
                            [--list | latest | <id> | --pin <id>]
  aval uninstall          remove aval's links and restore what they displaced
                            [--dry-run] [--yes]

Agent tooling
  aval rules              mirror CLAUDE.md's shared rules into OpenCode's AGENTS.md
                            sync [--check]
  aval review             git-only risk tier and reviewer lenses
                            assess [--base] [--json]
  aval design             design gates for the project in the current directory
                            <gate|ack> ...
  aval deck               turn a plan into a PDF slide deck, rendered locally
                            <build|check|example> ...
  aval pdf                scaffold PDF report projects and their brand assets
                            <init|assets> ...
  aval release            cut a release
                            <status|next|patch|minor|major|rc|beta|promote>

Plumbing
  Called by hooks and scripts, rarely by hand.
  aval hook               agent hooks and the git pre-push gate
                            <lessons|design-gate|pre-push|...>
  aval statusline         Claude Code status line
  aval tmux               tmux, Ghostty and zsh helpers
                            <helper>
  aval macos              macOS defaults and LaunchAgents
                            <helper>
  aval reap-orphan-tests  end orphaned test processes
                            [--min-age] [--dry-run]
  aval kiri               make Docker ready on Kiri
                            up
  aval docker             safe Docker prunes on the kiri context
                            cleanup [--dry-run]

Global flags:
  --repo <path>   the aval repository to use
  --theme <name>  colour theme for this run (aval, terminal, mono, catppuccin, ...)
  --plain         monochrome, ASCII glyphs and no motion
  --lang <code>   language for human output: auto, en or es

Settings: theme, background, glyphs, motion, update_check, language. Precedence
is flag, then AVAL_* environment, then ~/.config/aval/config; language then
follows the locale (LC_ALL, LC_MESSAGES, LANG). NO_COLOR turns colour off.

The repository is found from, in order: --repo, $AVAL_REPO, the location
'aval install' recorded in ~/.config/aval/config, then the current directory.
First time: clone the repo, cd into it and run 'aval install'.

Not set up yet. Run: aval install
`

const usageES = `aval dev
Nada pasa sin evidencia.

aval instala y mantiene los dotfiles y las configuraciones de agentes de este equipo.

Uso:
  aval <comando> [opciones]

Preparación
  aval install            enlaza las configuraciones del repositorio en HOME (TUI; --yes para omitirla)
                            [--dry-run] [--yes] [--copy]
  aval update             actualiza el repositorio y vuelve a aplicar
                            [--dry-run]
  aval sync               fusiona los ajustes del repositorio en la configuración activa
                            [--dry-run]
  aval config             muestra o cambia los ajustes de aval (tema, glifos, animación)
                            [list | get <key> | set <key> <value> | path]
  aval dev                HOME aislado para desarrollar aval
                            [run | diff | reset | ls | rm | project]

Salud
  aval doctor             comprueba este equipo frente al repositorio
                            [--fix] [--json]
  aval budget             contexto fijo por agente y latencia de los hooks
                            [--json]
  aval deps               herramientas externas de las que depende aval, y cuáles faltan
                            [--json] [--layer harness|agent|env]
  aval version            versión, commit, fecha de compilación y canal
  aval help               muestra esta ayuda

Deshacer
  aval restore            lista o restaura copias de seguridad
                            [--list | latest | <id> | --pin <id>]
  aval uninstall          quita los enlaces de aval y restaura lo que desplazaron
                            [--dry-run] [--yes]

Herramientas de agentes
  aval rules              refleja las reglas compartidas de CLAUDE.md en el AGENTS.md de OpenCode
                            sync [--check]
  aval review             nivel de riesgo según git y lentes de revisión
                            assess [--base] [--json]
  aval design             controles de diseño para el proyecto del directorio actual
                            <gate|ack> ...
  aval deck               convierte un plan en una presentación PDF, generada en local
                            <build|check|example> ...
  aval pdf                crea proyectos de informes PDF y sus recursos de marca
                            <init|assets> ...
  aval release            publica una versión
                            <status|next|patch|minor|major|rc|beta|promote>

Piezas internas
  Las llaman los hooks y los scripts, pocas veces a mano.
  aval hook               hooks de agentes y el control previo al push de git
                            <lessons|design-gate|pre-push|...>
  aval statusline         línea de estado de Claude Code
  aval tmux               utilidades de tmux, Ghostty y zsh
                            <helper>
  aval macos              ajustes de macOS y LaunchAgents
                            <helper>
  aval reap-orphan-tests  termina los procesos de pruebas huérfanos
                            [--min-age] [--dry-run]
  aval kiri               deja Docker listo en Kiri
                            up
  aval docker             limpiezas seguras de Docker en el contexto kiri
                            cleanup [--dry-run]

Opciones globales:
  --repo <path>   el repositorio de aval que se usa
  --theme <name>  tema de color para esta ejecución (aval, terminal, mono, catppuccin, ...)
  --plain         monocromo, glifos ASCII y sin animación
  --lang <code>   idioma de la salida para personas: auto, en o es

Ajustes: theme, background, glyphs, motion, update_check, language. La precedencia
es opción, luego variable AVAL_*, luego ~/.config/aval/config; language sigue
después la configuración regional (LC_ALL, LC_MESSAGES, LANG). NO_COLOR desactiva el color.

El repositorio se busca, en orden, en: --repo, $AVAL_REPO, la ubicación que
'aval install' guardó en ~/.config/aval/config y el directorio actual.
La primera vez: clone el repositorio, entre en él y ejecute 'aval install'.

Aún sin configurar. Ejecute: aval install
`

// The usage text. The language comes from --lang, AVAL_LANG, the config file
// or the locale; a machine with a config file has no first-run hint.
func TestMessagesUsageText(t *testing.T) {
	cfgES := func(t *testing.T, _, home string) { write(t, configFile(home), "language = es\n") }
	configured := func(t *testing.T, _, home string) { write(t, configFile(home), "theme = aval\n") }
	noHint := func(s, hint string) string { return strings.TrimSuffix(s, hint) }
	const hintEN, hintES = "\nNot set up yet. Run: aval install\n", "\nAún sin configurar. Ejecute: aval install\n"
	runMessages(t, []msgCase{
		{name: "help in English", args: []string{"help"}, stdout: usageEN},
		{name: "help with --lang es", args: []string{"--lang", "es", "help"}, stdout: usageES},
		{name: "help with AVAL_LANG=es", args: []string{"help"}, env: []string{"AVAL_LANG=es"}, stdout: usageES},
		{name: "help with a Spanish locale", args: []string{"help"}, env: []string{"AVAL_LANG", "LC_ALL", "LC_MESSAGES", "LANG=es_ES.UTF-8"}, stdout: usageES},
		{name: "--lang beats AVAL_LANG", args: []string{"--lang", "en", "help"}, env: []string{"AVAL_LANG=es"}, stdout: usageEN},
		{name: "help with language = es in the file", setup: cfgES, args: []string{"help"}, env: []string{"AVAL_LANG"}, stdout: noHint(usageES, hintES)},
		{name: "help on a configured machine", setup: configured, args: []string{"help"}, stdout: noHint(usageEN, hintEN)},
		{name: "a bare aval without a terminal", args: []string{}, stdout: usageEN},
		{name: "a bare aval without a terminal, Spanish", args: []string{"--lang", "es"}, stdout: usageES},
	})
}

// doctor's table: the header, the status words, the fix hint and the summary
// are translated; the message of each check is data from the checks. PATH is
// an empty directory so the deps row is the same on every machine: no git,
// which blocks.
func TestMessagesDoctor(t *testing.T) {
	bare := func(t *testing.T, dir, _ string) {
		write(t, filepath.Join(dir, "repo", "install.manifest"), "[g]\nlabel = G\ndetail = d\nplatforms = all\n")
	}
	runMessages(t, []msgCase{
		{name: "doctor in English", setup: bare, args: []string{"doctor", "--repo", "DIR/repo"}, env: []string{"PATH=DIR/empty"}, code: 1,
			stdout: "STATUS  CHECK           MESSAGE\n" +
				"OK      links           0 manifest links in place\n" +
				"OK      broken-links    no dangling links in agent directories\n" +
				"WARN    hooks-resolve   ~/.claude/settings.json is missing; hooks not checked (fixable: aval doctor --fix)\n" +
				"WARN    hooks-retired   ~/.claude/settings.json is missing; hooks not checked (fixable: aval doctor --fix)\n" +
				"FAIL    hooks-drift     ~/.claude/settings.json is missing (fixable: aval doctor --fix)\n" +
				"OK      agents          no ~/.claude/agents directory\n" +
				"OK      skills          no ~/.claude/skills directory\n" +
				"OK      plugins         no live settings.json to check\n" +
				"FAIL    settings-drift  sync: repo settings: open DIR/repo/config/claude/settings.json: no such file or directory\n" +
				"OK      overlay         absent (optional): ~/.config/aval/private\n" +
				"OK      config          no config file, using defaults\n" +
				"FAIL    deps            blocking tools missing: git (aval deps lists all)\n" +
				"\n7 ok, 2 warn, 3 fail\n"},
		{name: "doctor in Spanish", setup: bare, args: []string{"--lang", "es", "doctor", "--repo", "DIR/repo"}, env: []string{"PATH=DIR/empty"}, code: 1,
			stdout: "ESTADO  COMPROBACIÓN    MENSAJE\n" +
				"OK      links           0 manifest links in place\n" +
				"OK      broken-links    no dangling links in agent directories\n" +
				"AVISO   hooks-resolve   ~/.claude/settings.json is missing; hooks not checked (reparable: aval doctor --fix)\n" +
				"AVISO   hooks-retired   ~/.claude/settings.json is missing; hooks not checked (reparable: aval doctor --fix)\n" +
				"FALLO   hooks-drift     ~/.claude/settings.json is missing (reparable: aval doctor --fix)\n" +
				"OK      agents          no ~/.claude/agents directory\n" +
				"OK      skills          no ~/.claude/skills directory\n" +
				"OK      plugins         no live settings.json to check\n" +
				"FALLO   settings-drift  sync: repo settings: open DIR/repo/config/claude/settings.json: no such file or directory\n" +
				"OK      overlay         absent (optional): ~/.config/aval/private\n" +
				"OK      config          no config file, using defaults\n" +
				"FALLO   deps            blocking tools missing: git (aval deps lists all)\n" +
				"\n7 correctos, 2 avisos, 3 fallos\n"},
	})
}

// The commands that change or inspect HOME: sync, restore and uninstall. The
// words the core packages return (a restore step, an uninstall step) are
// translated when the catalog knows them and printed as they are otherwise.
func TestMessagesMaintenance(t *testing.T) {
	// A repository with one manifest link, the repo-managed settings and the
	// shared rules block; HOME holds a link into it.
	machine := func(t *testing.T, dir, home string) {
		repo := filepath.Join(dir, "repo")
		write(t, filepath.Join(repo, "install.manifest"), "[g]\nlabel = G\ndetail = d\nplatforms = all\n\nsrc/a .a\n")
		write(t, filepath.Join(repo, "src", "a"), "hi\n")
		write(t, filepath.Join(repo, "config", "claude", "settings.json"), `{"spinnerTipsEnabled": false}`)
		write(t, filepath.Join(repo, "config", "claude", "CLAUDE.md"), "<!-- shared:agent-rules -->\nr\n<!-- /shared:agent-rules -->\n")
		if err := os.Symlink(filepath.Join(repo, "src", "a"), filepath.Join(home, ".a")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	// The same machine with the live settings already in sync.
	synced := func(t *testing.T, dir, home string) {
		machine(t, dir, home)
		write(t, filepath.Join(home, ".claude", "settings.json"), `{"spinnerTipsEnabled": false}`)
	}
	bare := func(t *testing.T, dir, _ string) {
		write(t, filepath.Join(dir, "repo", "install.manifest"), "[g]\nlabel = G\ndetail = d\nplatforms = all\n")
	}
	es := []string{"AVAL_LANG=es"}
	runMessages(t, []msgCase{
		{name: "restore with no backups", setup: bare, args: []string{"restore", "--repo", "DIR/repo"}, stdout: "no backups yet\n"},
		{name: "restore with no backups, Spanish", setup: bare, args: []string{"restore", "--repo", "DIR/repo"}, env: es, stdout: "aún no hay copias de seguridad\n"},

		{name: "sync dry run", setup: machine, args: []string{"sync", "--dry-run", "--repo", "DIR/repo"},
			stdout: "settings  would change spinnerTipsEnabled\n" +
				"rules     ~/.codex/AGENTS.md skipped (agent not installed)\n" +
				"rules     ~/.gemini/GEMINI.md skipped (agent not installed)\n" +
				"\nDry run: nothing was written.\n"},
		{name: "sync dry run, Spanish", setup: machine, args: []string{"sync", "--dry-run", "--repo", "DIR/repo"}, env: es,
			stdout: "ajustes   cambiaría spinnerTipsEnabled\n" +
				"reglas    ~/.codex/AGENTS.md omitido (agente no instalado)\n" +
				"reglas    ~/.gemini/GEMINI.md omitido (agente no instalado)\n" +
				"\nSimulación: no se escribió nada.\n"},
		{name: "sync with nothing to change", setup: synced, args: []string{"sync", "--repo", "DIR/repo"},
			stdout: "settings  ~/.claude/settings.json is up to date\n" +
				"rules     ~/.codex/AGENTS.md skipped (agent not installed)\n" +
				"rules     ~/.gemini/GEMINI.md skipped (agent not installed)\n"},
		{name: "sync with nothing to change, Spanish", setup: synced, args: []string{"sync", "--repo", "DIR/repo"}, env: es,
			stdout: "ajustes   ~/.claude/settings.json está al día\n" +
				"reglas    ~/.codex/AGENTS.md omitido (agente no instalado)\n" +
				"reglas    ~/.gemini/GEMINI.md omitido (agente no instalado)\n"},
		{name: "sync check with drift", setup: machine, args: []string{"sync", "--check", "--repo", "DIR/repo"}, code: 1,
			stdout: "drift  settings  ~/.claude/settings.json would change: file missing; spinnerTipsEnabled\n\nRun `aval sync` to apply.\n"},
		{name: "sync check with drift, Spanish", setup: machine, args: []string{"sync", "--check", "--repo", "DIR/repo"}, env: es, code: 1,
			stdout: "desfase  ajustes   ~/.claude/settings.json cambiaría: archivo ausente; spinnerTipsEnabled\n\nEjecute `aval sync` para aplicar.\n"},
		{name: "sync check without drift", setup: synced, args: []string{"sync", "--check", "--repo", "DIR/repo"}, stdout: "sync: no drift\n"},
		{name: "sync check without drift, Spanish", setup: synced, args: []string{"sync", "--check", "--repo", "DIR/repo"}, env: es, stdout: "sync: sin desfase\n"},

		{name: "uninstall dry run", setup: machine, args: []string{"uninstall", "--dry-run", "--repo", "DIR/repo"},
			stdout: "~/.a                                                         would remove link\n\nDry run: nothing was written.\n"},
		{name: "uninstall dry run, Spanish", setup: machine, args: []string{"uninstall", "--dry-run", "--repo", "DIR/repo"}, env: es,
			stdout: "~/.a                                                         quitaría el enlace\n\nSimulación: no se escribió nada.\n"},
		{name: "uninstall declined", setup: machine, args: []string{"uninstall", "--repo", "DIR/repo"}, stdin: "n\n",
			stdout: "~/.a                                                         would remove link\n\nProceed? [y/N] aborted, nothing was changed\n"},
		{name: "uninstall declined, Spanish", setup: machine, args: []string{"uninstall", "--repo", "DIR/repo"}, env: es, stdin: "n\n",
			stdout: "~/.a                                                         quitaría el enlace\n\n¿Continuar? [s/N] cancelado, no se cambió nada\n"},
		{name: "uninstall confirmed in Spanish", setup: machine, args: []string{"uninstall", "--repo", "DIR/repo"}, env: es, stdin: "sí\n",
			stdout: "~/.a                                                         quitaría el enlace\n\n¿Continuar? [s/N] \n" +
				"~/.a                                                         enlace quitado\n\npara deshacer: aval restore latest\n"},
		{name: "nothing to uninstall", setup: bare, args: []string{"uninstall", "--dry-run", "--repo", "DIR/repo"},
			stdout: "nothing to uninstall: no manifest link points into this repo\n\nDry run: nothing was written.\n"},
	})
}

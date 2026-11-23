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
}

// configFile is where aval keeps its settings under home, per OS.
func configFile(home string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Roaming", "aval", "config")
	}
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
			got = filepath.ToSlash(got)
			eq(t, "exit code\n"+r.Out(), c.code, r.Code)
			eq(t, "stderr", c.stderr, got)
		})
	}
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
		{name: "release without a kind", args: []string{"release"}, code: 1, stderr: "error: usage: aval release <command>\n\n  status                 latest tags, unreleased commits, next suggested version\n  patch | minor | major  cut a stable release\n  rc | beta              cut the next prerelease of the upcoming version\n  promote                turn the latest rc into the stable release (same commit)\n\n  cut commands take --yes (skip the prompt) and --dry-run (show only)\n"},
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
	runMessages(t, []msgCase{
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

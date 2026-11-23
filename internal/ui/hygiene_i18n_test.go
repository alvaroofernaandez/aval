package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The i18n hygiene rule (ADR 0006): every string a person reads in the TUI
// comes from the catalog, so a literal in this package that is not a catalog
// key, a technical token or an excused exception fails the build. Like
// archExceptions in tests/architecture_test.go the exceptions are a ratchet:
// each one needs a reason, a stale one fails, and the list has a cap.

// uiLiteral is one string literal of non-test code in this package.
type uiLiteral struct {
	file string
	line int
	text string
}

func (l uiLiteral) String() string { return fmt.Sprintf("%s:%d %q", l.file, l.line, l.text) }

var (
	hasWord = regexp.MustCompile(`[A-Za-z]{2}`)
	hexRGB  = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)
	// pathLike is a path or a URL: no space, and it starts with ~, a dot, a
	// slash or a scheme. A command is "aval ..." and is shown verbatim.
	pathLike = regexp.MustCompile(`^(~|\.|/|https?://)\S*$`)
	// labelToken is a key label resolved through the glyph tier: {updown}.
	labelToken = regexp.MustCompile(`^\{[a-z]+\}$`)
)

// uiLiterals returns the candidate user-facing literals of the non-test Go
// files directly in dir. These are skipped by structure, because they are not
// text a person reads: import paths, struct tags, the key of a message
// lookup (T, N, Key) and the names of its placeholders (i18n.Args), a
// programmer error (panic, errors.New, fmt.Errorf), the keys and label fields
// of a key binding (tea key names and key caps are not translated), a value
// that is compared or switched on, never shown (==, !=, case), the argument
// of os.Getenv and regexp.MustCompile, paths, URLs and aval commands, colour
// values, key label tokens, the rows of settingRows (config keys and values),
// and strings with no word in them.
func uiLiterals(dir string) ([]uiLiteral, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	fset := token.NewFileSet()
	var out []uiLiteral
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		skip := map[*ast.BasicLit]bool{}
		mark := func(e ast.Expr) {
			ast.Inspect(e, func(n ast.Node) bool {
				if l, ok := n.(*ast.BasicLit); ok {
					skip[l] = true
				}
				return true
			})
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.ImportSpec:
				skip[x.Path] = true
			case *ast.Field:
				if x.Tag != nil {
					skip[x.Tag] = true
				}
			case *ast.KeyValueExpr:
				if id, ok := x.Key.(*ast.Ident); ok && (id.Name == "keys" || id.Name == "label") {
					mark(x.Value)
				}
			case *ast.CompositeLit:
				switch t := x.Type.(type) {
				case *ast.SelectorExpr:
					// i18n.Args{"count": n}: the keys are placeholder names.
					if t.Sel.Name == "Args" {
						for _, e := range x.Elts {
							if kv, ok := e.(*ast.KeyValueExpr); ok {
								mark(kv.Key)
							}
						}
					}
				case *ast.ArrayType:
					// []settingRow{...}: config keys and values.
					if id, ok := t.Elt.(*ast.Ident); ok && id.Name == "settingRow" {
						mark(x)
					}
				}
			case *ast.CaseClause:
				for _, e := range x.List {
					mark(e)
				}
			case *ast.BinaryExpr:
				if x.Op == token.EQL || x.Op == token.NEQ {
					mark(x.X)
					mark(x.Y)
				}
			case *ast.CallExpr:
				if len(x.Args) == 0 {
					break
				}
				switch fn := x.Fun.(type) {
				case *ast.SelectorExpr:
					name := fn.Sel.Name
					pkg, _ := fn.X.(*ast.Ident)
					switch {
					case name == "T" || name == "N" || name == "Key":
						mark(x.Args[0])
					case pkg != nil && pkg.Name == "errors" && name == "New",
						pkg != nil && pkg.Name == "fmt" && name == "Errorf",
						pkg != nil && pkg.Name == "os" && name == "Getenv",
						pkg != nil && pkg.Name == "regexp" && name == "MustCompile":
						mark(x.Args[0])
					}
				case *ast.Ident:
					if fn.Name == "panic" {
						mark(x.Args[0])
					}
				}
			}
			return true
		})
		ast.Inspect(f, func(n ast.Node) bool {
			l, ok := n.(*ast.BasicLit)
			if !ok || l.Kind != token.STRING || skip[l] {
				return true
			}
			v, err := strconv.Unquote(l.Value)
			if err != nil || !hasWord.MatchString(v) || hexRGB.MatchString(v) || pathLike.MatchString(v) || labelToken.MatchString(v) || strings.HasPrefix(v, "aval ") {
				return true
			}
			out = append(out, uiLiteral{file: filepath.Base(p), line: fset.Position(l.Pos()).Line, text: v})
			return true
		})
	}
	return out, nil
}

// uiRatchet splits lits into the ones no exception covers (fresh) and the
// exceptions no literal needs (stale). An exception is keyed by the literal
// text, by "file.go:text" to excuse it in one file only, or by "glob:*" to
// excuse a whole file of data (glyph tiers, palettes, theme names).
func uiRatchet(lits []uiLiteral, exc map[string]string) (fresh []uiLiteral, stale []string) {
	used := map[string]bool{}
	for _, l := range lits {
		if key, ok := matchException(exc, l); ok {
			used[key] = true
			continue
		}
		fresh = append(fresh, l)
	}
	for k := range exc {
		if !used[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(stale)
	return fresh, stale
}

// matchException is the exception that covers l, if any.
func matchException(exc map[string]string, l uiLiteral) (string, bool) {
	for _, k := range []string{l.file + ":" + l.text, l.text} {
		if _, ok := exc[k]; ok {
			return k, true
		}
	}
	for k := range exc {
		if glob, ok := strings.CutSuffix(k, ":*"); ok {
			if m, _ := filepath.Match(glob, l.file); m {
				return k, true
			}
		}
	}
	return "", false
}

// uiMaxExceptions caps uiLiteralExceptions. It may go down, never up: a new
// string a person reads is a catalog key, not an exception.
const uiMaxExceptions = 30

// uiLiteralExceptions are the literals that are words but not user text.
var uiLiteralExceptions = map[string]string{
	// Whole files of data.
	"glyphs.go:*":   "glyph tiers and their names: symbols and identifiers",
	"palette*.go:*": "palette tokens and colour names",
	"theme.go:*":    "theme identifiers, the same in every language",
	// Names that are the same in every language.
	"aval":    "the brand name",
	"macOS":   "an operating system's proper name (PlatformLabel)",
	"Linux":   "an operating system's proper name (PlatformLabel)",
	"Windows": "an operating system's proper name (PlatformLabel)",
	"space":   "a key cap, like enter and esc, is the same in every language",
	// Identifiers and commands that programs read.
	"install":       "a view id and a backup run origin",
	"update":        "a view id",
	"sync":          "a view id and a backup run origin",
	"doctor":        "a view id",
	"backups":       "a view id",
	"uninstall":     "a view id",
	"settings":      "a view id",
	"dev":           "a view id",
	"about":         "a view id",
	"reset":         "the `aval dev reset` subcommand",
	"exec ":         "the shell command that reloads the shell, shown verbatim",
	"git clone ":    "the command the clone runs, shown verbatim",
	"[aval dev:%s]": "the sandbox marker, the same text in every language",
	// The config file's own values, shown as written.
	"ascii":                         "a glyph tier value of the config file",
	"unicode":                       "a glyph tier value of the config file",
	"on":                            "a value of the config file",
	"off":                           "a value of the config file",
	"screen_settings.go:repo":       "a config key",
	"screen_settings.go:theme":      "a config key",
	"screen_settings.go:glyphs":     "a config key",
	"screen_settings.go:background": "a config key",
	"screen_settings.go:language":   "a config key",
}

func TestNoUserFacingLiteralsOutsideTheCatalog(t *testing.T) {
	lits, err := uiLiterals(".")
	if err != nil {
		t.Fatal(err)
	}
	fresh, stale := uiRatchet(lits, uiLiteralExceptions)
	if len(fresh) > 0 {
		lines := make([]string, len(fresh))
		for i, l := range fresh {
			lines[i] = l.String()
		}
		t.Errorf("%d string literals outside the i18n catalog (use tr.T/tr.N, or excuse a technical token in uiLiteralExceptions with a reason):\n  %s",
			len(fresh), strings.Join(lines, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("uiLiteralExceptions lists literals that no longer exist; delete them:\n  %s", strings.Join(stale, "\n  "))
	}
	if n := len(uiLiteralExceptions); n > uiMaxExceptions {
		t.Errorf("uiLiteralExceptions has %d entries, over the cap of %d: translate the string instead", n, uiMaxExceptions)
	}
	for k, why := range uiLiteralExceptions {
		if strings.TrimSpace(why) == "" {
			t.Errorf("exception %q has no reason", k)
		}
	}
}

// The rule must find a planted bad case, honour an exception, and flag a
// stale one; a rule that stops finding anything fails here.
func TestHygieneRuleFindsPlantedLiterals(t *testing.T) {
	dir := t.TempDir()
	src := `package ui

import "errors"

var bad = "Nothing was installed"
var tech = "linked"
var okKey = tr.T("hub.title", nil)
var cap = binding{keys: []string{"enter"}, label: "enter", desc: i18n.Key("help.move")}
var err1 = errors.New("programmer error here")
var colour = "#a0b1c2"
var cmp = a == "compared" || b != "also compared"
var path = "~/.aval-backup"
var cmd = "aval restore latest"
var args = i18n.Args{"count": 1}
var env = os.Getenv("SHELL")
func f(k string) { switch k { case "up": } }
`
	if err := os.WriteFile(filepath.Join(dir, "planted.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "planted_test.go"), []byte("package ui\nvar x = \"Test only text\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lits, err := uiLiterals(dir)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, l := range lits {
		texts = append(texts, l.text)
	}
	if got, want := strings.Join(texts, "|"), "Nothing was installed|linked"; got != want {
		t.Fatalf("uiLiterals = %q, want %q (keys, caps, errors, colours and test files are not user text)", got, want)
	}

	fresh, stale := uiRatchet(lits, map[string]string{"linked": "state id", "planted.go:gone": "deleted"})
	if len(fresh) != 1 || fresh[0].text != "Nothing was installed" {
		t.Errorf("fresh = %v, want only the sentence", fresh)
	}
	if len(stale) != 1 || stale[0] != "planted.go:gone" {
		t.Errorf("stale = %v, want planted.go:gone", stale)
	}
}

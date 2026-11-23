package ui

import (
	"fmt"
	"sort"
	"strings"
)

// Palette maps the theme tokens to colours for one variant. A value is a hex
// colour ("#rrggbb"), an ANSI index ("0" to "15") or "" for the terminal's
// default. Attributes (bold strong, key and fail; reverse cursor) are fixed by
// the engine, not the palette, so every theme keeps them.
//
// Background is never painted. It is the colour the variant was designed on,
// and the one its contrast tests measure against.
type Palette struct {
	Name       string
	Background string

	Text, Strong, Muted, Rule string
	OK, Warn, Fail            string
	Key                       string
}

func (p Palette) all() map[string]string {
	m := p.meaningful()
	m["rule"] = p.Rule
	return m
}

// meaningful is every token that carries information; rule is decorative.
func (p Palette) meaningful() map[string]string {
	return map[string]string{
		"text": p.Text, "strong": p.Strong, "muted": p.Muted,
		"ok": p.OK, "warn": p.Warn, "fail": p.Fail, "key": p.Key,
	}
}

// Family groups variants under a bare name. A bare name follows the
// background; a variant name ("aval-light") pins one. A family with a single
// variant sets Dark and Light to the same name.
type Family struct {
	Name        string
	Dark, Light string
}

var (
	palettes = map[string]Palette{}
	families = map[string]Family{}
)

// register adds a family and its variants. It panics on a duplicate or a
// family pointing at a missing variant: both are programming errors that the
// package tests hit at init.
func register(f Family, variants ...Palette) {
	for _, p := range variants {
		if _, dup := palettes[p.Name]; dup {
			panic("ui: duplicate palette " + p.Name)
		}
		palettes[p.Name] = p
	}
	for _, v := range []string{f.Dark, f.Light} {
		if _, ok := palettes[v]; !ok {
			panic("ui: family " + f.Name + " names missing variant " + v)
		}
	}
	if _, dup := families[f.Name]; dup {
		panic("ui: duplicate theme family " + f.Name)
	}
	families[f.Name] = f
}

func init() {
	// DESIGN.md section 10: pure ink for text and chrome, colour only for state.
	register(Family{Name: "aval", Dark: "aval-dark", Light: "aval-light"},
		Palette{
			Name: "aval-dark", Background: "#000000",
			Text: "#ffffff", Strong: "#ffffff", Key: "#ffffff",
			Muted: "#a3a3a3", Rule: "#2e2e2e",
			OK: "#98BB6C", Warn: "#E6C384", Fail: "#E46876",
		},
		Palette{
			Name: "aval-light", Background: "#ffffff",
			Text: "#000000", Strong: "#000000", Key: "#000000",
			Muted: "#525252", Rule: "#d4d4d4",
			OK: "#5a7339", Warn: "#77713f", Fail: "#C34043",
		},
	)
	// ANSI 16 only, so it inherits the user's own terminal theme. muted is the
	// default colour: never faint, never bright black.
	register(Family{Name: "terminal", Dark: "terminal", Light: "terminal"},
		Palette{Name: "terminal", OK: "2", Warn: "3", Fail: "1"},
	)
	// Attributes only. NO_COLOR resolves here whatever theme was asked for.
	register(Family{Name: "mono", Dark: "mono", Light: "mono"},
		Palette{Name: "mono"},
	)
}

// LookupPalette resolves a family name (following dark) or a pinned variant.
func LookupPalette(name string, dark bool) (Palette, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if f, ok := families[key]; ok {
		if dark {
			key = f.Dark
		} else {
			key = f.Light
		}
	}
	if p, ok := palettes[key]; ok {
		return p, nil
	}
	return Palette{}, unknownTheme(name)
}

// ThemeNames lists every name a user may set: families and variants, sorted.
func ThemeNames() []string {
	seen := map[string]bool{}
	for n := range families {
		seen[n] = true
	}
	for n := range palettes {
		seen[n] = true
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func unknownTheme(name string) error {
	names := ThemeNames()
	best, bestD := "", -1
	for _, n := range names {
		if d := editDistance(strings.ToLower(name), n); bestD < 0 || d < bestD {
			best, bestD = n, d
		}
	}
	return fmt.Errorf("unknown theme %q: did you mean %q? (themes: %s)", name, best, strings.Join(names, ", "))
}

// editDistance is the Levenshtein distance with adjacent transpositions
// counted as one edit, so "aval-drak" lands on "aval-dark".
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}

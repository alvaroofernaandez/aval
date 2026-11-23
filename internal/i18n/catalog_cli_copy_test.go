package i18n

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// DESIGN.md section 10 for the CLI keys, in every language: no exclamation
// marks (Spanish ones included), no em dash, no escape codes.
func cliCopyViolations(cats map[Lang]catalog) []string {
	var out []string
	for _, l := range Languages() {
		for k, m := range cats[l] {
			if !strings.HasPrefix(k, "cli.") {
				continue
			}
			for _, form := range []string{m.one, m.other} {
				if strings.ContainsAny(form, "!¡—\x1b") {
					out = append(out, string(l)+": "+k)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestCLICopyRules(t *testing.T) {
	planted := map[Lang]catalog{English: {"cli.a": {other: "Done!"}}, Spanish: {"cli.a": {other: "¡Hecho"}, "cli.b": {other: "a — b"}, "hub.c": {other: "No!"}}}
	if got := cliCopyViolations(planted); len(got) != 3 {
		t.Errorf("planted catalog: violations = %q, want 3 (hub.* is not a CLI key)", got)
	}
	if got := cliCopyViolations(catalogs); len(got) > 0 {
		t.Errorf("CLI copy breaks DESIGN.md section 10:\n  %s", strings.Join(got, "\n  "))
	}
}

// voseo matches the Rioplatense verb forms and the pronoun (\\b is ASCII-only in RE2, so
// the accented endings need explicit letter boundaries); the catalog is
// neutral Spanish, addressing the reader as usted or impersonally.
var voseo = regexp.MustCompile(`(?i)(^|[^\pL])(vos|tenés|querés|podés|sabés|elegí|pulsá|presioná|ejecutá|escribí|hacé|mirá|probá|revisá|configurá|confirmá|volvé|usá)($|[^\pL])`)

// spanishCopyViolations checks every Spanish key, UI and CLI alike: no
// exclamation marks, no em dash, no voseo, and every question that opens with
// a question mark is closed (and the reverse).
func spanishCopyViolations(cat catalog) []string {
	var out []string
	for k, m := range cat {
		for _, form := range []string{m.one, m.other} {
			bad := strings.ContainsAny(form, "!¡—\x1b") || voseo.MatchString(form) ||
				strings.Count(form, "¿") != strings.Count(form, "?")
			if bad {
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestSpanishCopyRules(t *testing.T) {
	planted := catalog{
		"a": {other: "¡Hecho"},
		"b": {other: "a — b"},
		"c": {other: "Elegí una opción"},
		"d": {other: "Continuar?"},
		"e": {other: "¿Continuar? Sí"},
		"f": {one: "ok", other: "Listo!"},
	}
	if got := spanishCopyViolations(planted); len(got) != 5 {
		t.Errorf("planted catalog: violations = %q, want 5 (a, b, c, d, f)", got)
	}
	if got := spanishCopyViolations(catalogs[Spanish]); len(got) > 0 {
		t.Errorf("Spanish copy breaks DESIGN.md section 10:\n  %s", strings.Join(got, "\n  "))
	}
}

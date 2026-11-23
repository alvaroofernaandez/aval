package i18n

import (
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

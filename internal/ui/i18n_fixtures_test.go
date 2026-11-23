package ui

import (
	"testing"

	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// testTr is the language the fixtures speak. It is English unless a test runs
// under forEachLang, which sets it per language. Tests do not run in
// parallel, so a package variable is enough.
var testTr = i18n.New(i18n.English)

// forEachLang runs f once per catalog language with the fixtures speaking it:
// the width, height and em dash invariants and the goldens hold in both
// (ADR 0006: Spanish runs longer).
func forEachLang(t *testing.T, f func(t *testing.T)) {
	t.Helper()
	for _, lang := range i18n.Languages() {
		t.Run(string(lang), func(t *testing.T) {
			prev := testTr
			testTr = i18n.New(lang)
			t.Cleanup(func() { testTr = prev })
			f(t)
		})
	}
}

// goldenName is a golden's file name in the current language. English keeps
// the names it had before the TUI was translated; the others add a suffix.
func goldenName(base string) string {
	if testTr.Lang() == i18n.English {
		return base
	}
	return base + "_" + string(testTr.Lang())
}

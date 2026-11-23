package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// resolveLang picks the language for human output: --lang, then AVAL_LANG,
// then the language key in the file, then the locale (LC_ALL, LC_MESSAGES,
// LANG), then English. A bad --lang is the caller's to reject first; a bad
// AVAL_LANG is skipped like any bad env value. Hooks, the statusline and tmux
// never call it: they never read config and their output is for programs.
func resolveLang(f *config.File, flag string, getenv func(string) string) i18n.Lang {
	v, _ := f.Resolve("language", map[string]string{"language": flag}, getenv)
	return i18n.Select(v, getenv)
}

// configWarnings renders config load warnings for a person, in tr's language.
// The config package reports the bad language as data because it cannot know
// the language yet: that value is the one being read. The other keys keep the
// config package's English text until their strings move to the catalog.
func configWarnings(tr i18n.Translator, path string, warns []config.Warning) []string {
	out := make([]string, 0, len(warns))
	for _, w := range warns {
		bad := w.Invalid
		if bad == nil || bad.Key != "language" {
			out = append(out, w.String())
			continue
		}
		msg := tr.T("config.invalid_language", i18n.Args{"value": strconv.Quote(bad.Value), "valid": strings.Join(bad.Valid, ", ")})
		if bad.Suggestion != "" {
			msg += " " + tr.T("config.did_you_mean", i18n.Args{"suggestion": bad.Suggestion})
		}
		out = append(out, fmt.Sprintf("config: %s:%d: %s", path, w.Line, msg))
	}
	return out
}

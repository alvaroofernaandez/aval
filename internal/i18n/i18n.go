// Package i18n is aval's message catalog: one table per language, keyed by
// stable dotted keys, with named placeholders and one/other plural forms.
//
// English is the source of truth and Spanish covers every key; the catalog
// tests fail on a key missing from a language, a placeholder that differs
// between languages, a plural key without both forms, and a key no Go code
// looks up. Only human output goes through here: --json, hooks, the status
// line, logs and generated files stay English (ADR 0006).
//
// A leaf package: it imports nothing internal (ADR 0002).
package i18n

import (
	"fmt"
	"regexp"
)

// Lang is a language aval speaks.
type Lang string

const (
	English Lang = "en"
	Spanish Lang = "es"
)

// Languages lists every language with a catalog, English first.
func Languages() []Lang { return []Lang{English, Spanish} }

// Args are the values of a message's named placeholders: {path} takes
// Args{"path": p}.
type Args map[string]any

// message is one entry. A plain message uses other only; a plural message
// sets both, and N picks one for a count of 1 and other for the rest.
type message struct{ one, other string }

// catalog maps keys to messages for one language.
type catalog map[string]message

// catalogs is every language's table.
var catalogs = map[Lang]catalog{English: english, Spanish: spanish}

// Translator looks messages up in one language. The zero value is English.
type Translator struct {
	lang Lang
	cats map[Lang]catalog // nil: the package catalogs
}

// New returns the Translator for l; an unknown language is English.
func New(l Lang) Translator {
	if _, ok := catalogs[l]; !ok {
		l = English
	}
	return Translator{lang: l}
}

// Lang is the language t speaks.
func (t Translator) Lang() Lang {
	if t.lang == "" {
		return English
	}
	return t.lang
}

// T returns the message for key with its placeholders filled from args. A key
// the language lacks falls back to English, and an unknown key to the key
// itself; the catalog tests keep both from shipping.
func (t Translator) T(key string, args Args) string {
	return format(t.lookup(key).other, args)
}

// N returns the plural message for key: the one form for a count of 1, the
// other form for the rest, with {count} set to count.
func (t Translator) N(key string, count int, args Args) string {
	m := t.lookup(key)
	text := m.other
	if count == 1 && m.one != "" {
		text = m.one
	}
	filled := Args{"count": count}
	for k, v := range args {
		filled[k] = v
	}
	return format(text, filled)
}

func (t Translator) lookup(key string) message {
	cats := t.cats
	if cats == nil {
		cats = catalogs
	}
	if m, ok := cats[t.Lang()][key]; ok {
		return m
	}
	if m, ok := cats[English][key]; ok {
		return m
	}
	return message{other: key}
}

// placeholder matches a named placeholder: {path}, {count}.
var placeholder = regexp.MustCompile(`\{([a-z_]+)\}`)

// format fills each placeholder that args names; the rest stay as written.
func format(text string, args Args) string {
	if len(args) == 0 {
		return text
	}
	return placeholder.ReplaceAllStringFunc(text, func(p string) string {
		if v, ok := args[p[1:len(p)-1]]; ok {
			return fmt.Sprint(v)
		}
		return p
	})
}

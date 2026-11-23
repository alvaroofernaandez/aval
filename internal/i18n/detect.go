package i18n

import "strings"

// Detect reads the locale the way POSIX does: the first non-empty of LC_ALL,
// LC_MESSAGES and LANG. "es" or "es_*" (with any encoding or modifier) is
// Spanish; anything else, including C and POSIX, is English.
func Detect(getenv func(string) string) Lang {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := getenv(name)
		if v == "" {
			continue
		}
		lang, _, _ := strings.Cut(v, "_")
		lang, _, _ = strings.Cut(lang, ".")
		lang, _, _ = strings.Cut(lang, "@")
		if strings.EqualFold(lang, string(Spanish)) {
			return Spanish
		}
		return English
	}
	return English
}

// Select turns a language setting into a language: "en" and "es" are
// themselves, and anything else ("auto", empty) is Detect.
func Select(setting string, getenv func(string) string) Lang {
	switch Lang(setting) {
	case English, Spanish:
		return Lang(setting)
	}
	return Detect(getenv)
}

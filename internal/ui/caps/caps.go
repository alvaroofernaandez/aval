// Package caps detects what the terminal can render. Detect is pure: the
// environment is injected, and the terminal is never queried.
package caps

import (
	"strings"

	"github.com/muesli/termenv"
)

// Inputs is everything Detect looks at.
type Inputs struct {
	Getenv        func(string) string
	GOOS          string
	IsTTY         bool
	Width, Height int

	Glyphs     string // auto | unicode | ascii
	Motion     string // auto | off
	Background string // auto | dark | light
	ColorOff   bool
}

// Caps is the detected capability set.
type Caps struct {
	Profile       termenv.Profile
	Unicode       bool
	Motion        bool
	Dark          bool
	Width, Height int
}

// Detect resolves capabilities from inputs. Explicit overrides win over
// detection; background "auto" falls back to dark.
func Detect(in Inputs) Caps {
	get := in.Getenv
	if get == nil {
		get = func(string) string { return "" }
	}
	term := get("TERM")
	dumb := term == "dumb"

	return Caps{
		Profile: profile(in, get, term, dumb),
		Unicode: unicode(in, get, term, dumb),
		Motion:  in.IsTTY && in.Motion != "off",
		Dark:    in.Background != "light",
		Width:   in.Width,
		Height:  in.Height,
	}
}

func profile(in Inputs, get func(string) string, term string, dumb bool) termenv.Profile {
	switch {
	case in.ColorOff, get("NO_COLOR") != "", dumb, !in.IsTTY:
		return termenv.Ascii
	}
	switch ct := strings.ToLower(get("COLORTERM")); {
	case ct == "truecolor" || ct == "24bit":
		return termenv.TrueColor
	case strings.Contains(term, "256color"):
		return termenv.ANSI256
	}
	return termenv.ANSI
}

func unicode(in Inputs, get func(string) string, term string, dumb bool) bool {
	switch in.Glyphs {
	case "unicode":
		return true
	case "ascii":
		return false
	}
	if dumb {
		return false
	}
	if in.GOOS == "windows" {
		return get("WT_SESSION") != "" || get("TERM_PROGRAM") != "" || get("ConEmuANSI") != ""
	}
	if term == "linux" {
		return false
	}
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := get(k); v != "" {
			v = strings.ToLower(v)
			return strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
		}
	}
	return false
}

package gate

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

const allowMarker = "ds-allow-hardcode"

var (
	codeExts = []string{".css", ".scss", ".tsx", ".jsx", ".ts", ".js", ".vue", ".svelte", ".swift", ".kt", ".dart", ".html"}

	hexRe      = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	hexValueRe = regexp.MustCompile(`[:=(,]\s*["']?#[0-9a-fA-F]{3,8}\b`)
	pxRe       = regexp.MustCompile(`\d+(?:\.\d+)?px\b`)
	timeRe     = regexp.MustCompile(`\d+(?:\.\d+)?m?s\b`)
	twRe       = regexp.MustCompile(`(?:bg|text|border|ring|ring-offset|fill|stroke|from|via|to|divide|outline|decoration|accent|caret|placeholder|shadow)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-(?:50|100|200|300|400|500|600|700|800|900|950)\b`)
	fontRe     = regexp.MustCompile(`font-family\s*:\s*`)
	tokenDef   = regexp.MustCompile(`--[\w\-]+\s*:`)
	themeFn    = regexp.MustCompile(`theme\(|tokens?[./]|\{[\w.\-]+\}`)
)

// Hit is one hardcoded value found on a line.
type Hit struct{ Kind, Value string }

// matches returns re's matches in s whose preceding rune is not rejected by
// bad. RE2 has no lookbehind, so the boundary is checked by hand.
func matches(re *regexp.Regexp, s string, bad func(rune) bool) []string {
	var out []string
	for _, loc := range re.FindAllStringIndex(s, -1) {
		if bad(prevRune(s, loc[0])) {
			continue
		}
		out = append(out, s[loc[0]:loc[1]])
	}
	return out
}

func wordOrAmp(r rune) bool  { return r == '&' || isWord(r) }
func wordOrDot(r rune) bool  { return r == '.' || isWord(r) }
func wordOrDash(r rune) bool { return r == '-' || isWord(r) }
func never(rune) bool        { return false }

// maskVarRefs blanks every var(...) span, nesting and fallbacks included: a
// token reference is not a hardcode, but whatever sits beside it still is.
func maskVarRefs(line string) string {
	var out strings.Builder
	for i := 0; ; {
		j := strings.Index(line[i:], "var(")
		if j < 0 {
			out.WriteString(line[i:])
			return out.String()
		}
		j += i
		out.WriteString(line[i:j])
		depth, k := 0, j+3
		for ; k < len(line); k++ {
			if line[k] == '(' {
				depth++
			} else if line[k] == ')' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		i = k + 1
		if i > len(line) {
			return out.String()
		}
	}
}

// stripBlockComment returns the code outside /* */ and whether a comment is
// still open at the end of the line.
func stripBlockComment(line string, in bool) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(line); {
		if in {
			end := strings.Index(line[i:], "*/")
			if end < 0 {
				return out.String(), true
			}
			i, in = i+end+2, false
		} else {
			start := strings.Index(line[i:], "/*")
			if start < 0 {
				out.WriteString(line[i:])
				return out.String(), false
			}
			out.WriteString(line[i : i+start])
			i, in = i+start+2, true
		}
	}
	return out.String(), in
}

// LintLine returns the hardcoded values on one line. cssScope=false means the
// line is markup or prose: a wait time in a table cell is content, not style.
func LintLine(line string, tailwind, cssScope bool) []Hit {
	if strings.Contains(line, allowMarker) || tokenDef.MatchString(line) || themeFn.MatchString(line) {
		return nil
	}
	code := maskVarRefs(line)
	t := strings.TrimSpace(line)
	for _, p := range []string{"//", "*", "/*", "#", "<!--"} {
		if strings.HasPrefix(t, p) {
			return nil
		}
	}
	var hits []Hit
	if cssScope {
		for _, m := range matches(hexRe, code, wordOrAmp) {
			hits = append(hits, Hit{"hex", m})
		}
		if !strings.Contains(line, "@media") && !strings.Contains(line, "@container") {
			for _, m := range matches(pxRe, code, wordOrDot) {
				if m != "0px" && m != "1px" {
					hits = append(hits, Hit{"px", m})
				}
			}
			for _, m := range matches(timeRe, code, wordOrDot) {
				hits = append(hits, Hit{"time", m})
			}
		}
	} else {
		for _, m := range hexValueRe.FindAllString(code, -1) {
			hits = append(hits, Hit{"hex", strings.TrimLeft(m, ":=(, \"'")})
		}
	}
	if tailwind {
		for _, m := range matches(twRe, line, wordOrDash) {
			hits = append(hits, Hit{"tailwind-palette", m})
		}
	}
	// font-family is read off the original line: its test is "not followed by
	// var(", and masking would remove the very thing it looks for.
	if locs := fontRe.FindAllStringIndex(line, -1); len(locs) > 0 {
		if !strings.Contains(line[locs[len(locs)-1][1]:], "var(") {
			hits = append(hits, Hit{"font-family", "literal font-family"})
		}
	}
	return hits
}

// HardcodeOpts tunes Hardcodes. A nil Exts means the default code extensions.
type HardcodeOpts struct {
	Exts     map[string]bool
	Tailwind bool
}

// Hardcodes lints component source for raw colours, lengths, durations,
// Tailwind palette utilities and literal font stacks that should be tokens.
func Hardcodes(w io.Writer, paths []string, o HardcodeOpts) int {
	if m := missingPaths(paths); len(m) > 0 {
		fmt.Fprintln(w, "ERROR: path(s) not found: "+strings.Join(m, ", "))
		return 1
	}
	ok := extSet(codeExts...)
	if o.Exts != nil {
		ok = func(e string) bool { return o.Exts[e] }
	}
	files := walk(paths, ok)
	if len(files) == 0 {
		fmt.Fprintf(w, "ERROR: no lintable file(s) under %s\n", strings.Join(paths, ", "))
		return 1
	}

	violations, unreadable := 0, 0
	for _, f := range files {
		text, err := readText(f)
		if err != nil {
			// A file never opened must not let the run read as clean.
			fmt.Fprintf(w, "ERROR: could not read %s: %v\n", f, err)
			unreadable++
			continue
		}
		ext := strings.ToLower(filepath.Ext(f))
		markup := ext == ".html" || ext == ".htm" || ext == ".vue" || ext == ".svelte" || ext == ".astro"
		inAllow, inStyle, inComment := false, false, false
		for n, raw := range lines(text) {
			// Markers are read off the RAW line: an exception is itself a
			// comment, so stripping comments first would revoke it.
			if strings.Contains(raw, allowMarker+":start") {
				inAllow = true
				continue
			}
			if strings.Contains(raw, allowMarker+":end") {
				inAllow = false
				continue
			}
			if inAllow || strings.Contains(raw, allowMarker) {
				continue
			}
			cssHere := true
			if markup {
				low := strings.ToLower(raw)
				opens, closes := strings.Contains(low, "<style"), strings.Contains(low, "</style>")
				cssHere = inStyle || opens || strings.Contains(low, "style=")
				if opens && !closes {
					inStyle = true
				}
				if closes {
					inStyle = false
				}
			}
			line := raw
			if cssHere {
				line, inComment = stripBlockComment(raw, inComment)
			} else {
				inComment = false
			}
			for _, h := range LintLine(line, o.Tailwind, cssHere) {
				fmt.Fprintf(w, "%s:%d: hardcoded %s '%s' - use a token\n", f, n+1, h.Kind, h.Value)
				violations++
			}
		}
	}

	fmt.Fprintf(w, "\nScanned %d of %d candidate file(s).\n", len(files)-unreadable, len(files))
	if unreadable > 0 {
		fmt.Fprintf(w, "FAIL: %d file(s) could not be read, so this run cannot report clean. %d hardcoded value(s) in the rest.\n", unreadable, violations)
		return 1
	}
	if violations > 0 {
		fmt.Fprintf(w, "FAIL: %d hardcoded value(s). Map each to a token, or add a '%s' comment for a justified exception.\n", violations, allowMarker)
		return 1
	}
	fmt.Fprintln(w, "OK: no hardcoded values found.")
	return 0
}

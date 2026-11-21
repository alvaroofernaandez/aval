package gate

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

var (
	defRe = regexp.MustCompile(`(--[A-Za-z0-9_-]+)\s*:`)
	refRe = regexp.MustCompile(`var\(\s*(--[A-Za-z0-9_-]+)\s*(,[^)]*)?\)`) // group 2 is the fallback
)

// ThemeRefs verifies that every CSS variable the scanned code references is
// defined in a theme file or in the scanned file itself. A reference with a
// fallback cannot render wrong, so it is reported and not failed.
func ThemeRefs(w io.Writer, themes, paths []string) int {
	if len(themes) == 0 || len(paths) == 0 {
		fmt.Fprintln(w, "ERROR: give at least one --theme and one path to scan.")
		return 1
	}
	if m := missingPaths(append(append([]string{}, themes...), paths...)); len(m) > 0 {
		fmt.Fprintln(w, "ERROR: path(s) not found: "+strings.Join(m, ", "))
		return 1
	}
	shared := map[string]bool{}
	for _, t := range themes {
		if b, err := os.ReadFile(t); err == nil {
			for _, m := range defRe.FindAllStringSubmatch(string(b), -1) {
				shared[m[1]] = true
			}
		}
	}
	if len(shared) == 0 {
		fmt.Fprintf(w, "ERROR: no CSS variables defined in theme (%s).\n", strings.Join(themes, ", "))
		return 1
	}
	files := walk(paths, extSet(".css", ".scss", ".tsx", ".jsx", ".ts", ".js", ".vue", ".svelte", ".html", ".astro"))
	if len(files) == 0 {
		fmt.Fprintf(w, "ERROR: no scannable file(s) under %s\n", strings.Join(paths, ", "))
		return 1
	}

	var missing []string
	fallbacks, unreadable, scanned := 0, 0, 0
	for _, f := range files {
		text, err := readText(f)
		if err != nil {
			fmt.Fprintf(w, "ERROR: could not read %s: %v\n", f, err)
			unreadable++
			continue
		}
		scanned++
		defined := map[string]bool{}
		for k := range shared {
			defined[k] = true
		}
		for _, m := range defRe.FindAllStringSubmatch(text, -1) {
			defined[m[1]] = true
		}
		for n, line := range lines(text) {
			for _, m := range refRe.FindAllStringSubmatch(line, -1) {
				switch {
				case defined[m[1]]:
				case m[2] != "":
					fallbacks++
				default:
					missing = append(missing, fmt.Sprintf("%s:%d: var(%s) resolves to nothing", f, n+1, m[1]))
				}
			}
		}
	}

	fmt.Fprintf(w, "Theme defines %d tokens; scanned %d of %d file(s).\n", len(shared), scanned, len(files))
	if fallbacks > 0 {
		fmt.Fprintf(w, "  %d reference(s) to an undefined token carry a fallback - not a failure.\n", fallbacks)
	}
	if unreadable > 0 {
		fmt.Fprintf(w, "FAIL: %d file(s) could not be read, so this run cannot report clean.\n", unreadable)
		return 1
	}
	if len(missing) > 0 {
		fmt.Fprintf(w, "\nFAIL: %d reference(s) resolve to nothing:\n", len(missing))
		for _, m := range missing {
			fmt.Fprintln(w, "  x "+m)
		}
		return 1
	}
	fmt.Fprintln(w, "OK: every token reference resolves, in the theme or in the file itself.")
	return 0
}

package designgate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Emoji and dingbat pictographs. Arrows (U+2190-21FF) and box drawing are
// legitimate typographic notation and stay allowed.
var (
	emojiRe = regexp.MustCompile(`[\x{1F000}-\x{1FAFF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}\x{2B00}-\x{2BFF}\x{FE0F}\x{20E3}]`)
	dashRe  = regexp.MustCompile(`[\x{2014}\x{2013}]`) // em/en dash: an AI-pattern tell in UI copy
)

var (
	emojiExts = extSet(".md", ".mdx", ".html", ".htm", ".tsx", ".jsx", ".ts", ".js", ".vue", ".svelte", ".css", ".scss", ".astro", ".json")
	uiExts    = extSet(".html", ".htm", ".tsx", ".jsx", ".vue", ".svelte")
)

// Emoji fails when an emoji or pictograph appears in the given files, and an
// em/en dash in UI markup. Green means "looked and found none": a missing
// path or an empty scan is an error.
func Emoji(w io.Writer, paths []string) int {
	if len(paths) == 0 {
		fmt.Fprintln(w, "ERROR: give at least one file or directory to scan")
		return 1
	}
	if m := missingPaths(paths); len(m) > 0 {
		fmt.Fprintln(w, "ERROR: path(s) not found: "+strings.Join(m, ", "))
		return 1
	}
	// A file named explicitly is scanned whatever its extension.
	var files []string
	for _, p := range paths {
		if st, _ := os.Stat(p); st != nil && !st.IsDir() {
			files = append(files, p)
		} else {
			files = append(files, walk([]string{p}, emojiExts)...)
		}
	}
	if len(files) == 0 {
		fmt.Fprintf(w, "ERROR: no scannable file(s) under %s\n", strings.Join(paths, ", "))
		return 1
	}

	var hits []string
	for _, f := range files {
		text, err := readText(f)
		if err != nil {
			continue
		}
		ui := uiExts(filepath.Ext(f))
		for n, line := range lines(text) {
			for _, m := range emojiRe.FindAllString(line, -1) {
				hits = append(hits, fmt.Sprintf("%s:%d: emoji/pictograph '%s' - use lucide / plain text", f, n+1, m))
			}
			if ui {
				for _, m := range dashRe.FindAllString(line, -1) {
					hits = append(hits, fmt.Sprintf("%s:%d: em/en-dash '%s' - AI-pattern tell; use a period, comma, or hyphen", f, n+1, m))
				}
			}
		}
	}
	fmt.Fprintf(w, "Scanned %d file(s).\n", len(files))
	if len(hits) > 0 {
		fmt.Fprintf(w, "\nFAIL: %d emoji/pictograph(s) found:\n", len(hits))
		for _, h := range hits {
			fmt.Fprintln(w, "  x "+h)
		}
		return 1
	}
	fmt.Fprintln(w, "OK: no emoji or UI dashes found.")
	return 0
}

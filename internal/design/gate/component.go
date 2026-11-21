package gate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type section struct {
	name string
	re   *regexp.Regexp
}

var (
	secAnatomy   = section{"anatomy", regexp.MustCompile(`(?i)anatom|structure|\blayout\b|composition`)}
	secVariants  = section{"variants", regexp.MustCompile(`(?i)variant`)}
	secStates    = section{"states", regexp.MustCompile(`(?i)\bstate`)}
	secTokens    = section{"token mapping", regexp.MustCompile(`(?i)token|\{[\w.\-]+\}|semantic\.|sizing\.|spacing|breakpoint|grid|\.json`)}
	secA11y      = section{"accessibility", regexp.MustCompile(`(?i)accessib|a11y|aria|wcag`)}
	fullSections = []section{secAnatomy, secVariants, secStates, secTokens, secA11y}
	// icon-system.md is a subsystem doc, not a single-component spec.
	relaxedSections = []section{secTokens, secA11y}
)

// Component checks that every spec in <root>/components/*.md documents
// anatomy, variants, states, token mapping and accessibility.
func Component(w io.Writer, root string) int {
	dir := filepath.Join(root, "components")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		fmt.Fprintf(w, "ERROR: %s not found\n", dir)
		return 1
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	sort.Strings(files)
	problems := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(w, "ERROR: could not read %s: %v\n", f, err)
			return 1
		}
		req := fullSections
		if filepath.Base(f) == "icon-system.md" {
			req = relaxedSections
		}
		var missing []string
		for _, s := range req {
			if !s.re.Match(b) {
				missing = append(missing, s.name)
			}
		}
		if len(missing) > 0 {
			fmt.Fprintf(w, "  MISS %s  -> missing: %s\n", filepath.Base(f), strings.Join(missing, ", "))
			problems++
		} else {
			fmt.Fprintf(w, "  OK   %s\n", filepath.Base(f))
		}
	}
	fmt.Fprintf(w, "\nChecked %d component spec(s).\n", len(files))
	if problems > 0 {
		fmt.Fprintf(w, "FAIL: %d spec(s) missing required sections.\n", problems)
		return 1
	}
	fmt.Fprintln(w, "OK: all component specs document anatomy, variants, states, tokens, and a11y.")
	return 0
}

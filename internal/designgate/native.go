package designgate

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Standing rule: a native date picker, a raw <select>, a browser <dialog> or a
// confirm() is never acceptable in a product surface. They cannot be styled
// consistently, look different on every OS and break the shared primitive layer
// the design system depends on. This gate is where that rule is enforced.

type nativeRule struct {
	re   *regexp.Regexp
	what string
	fix  string
}

// bare matches a global call (alert(...)) but not x.alert(...) or myalert(...).
// RE2 has no lookbehind, so the previous character is part of the match.
func bare(fn string) string {
	return `(?:^|[^.\w])` + fn + `\s*\(`
}

func inputType(t string) string { return `<input\b[^>]*type=["']` + t + `["']` }

var nativeRules = func() []nativeRule {
	r := func(p, what, fix string) nativeRule { return nativeRule{regexp.MustCompile(p), what, fix} }
	return []nativeRule{
		r(`<select\b`, "native <select>", "shadcn Select (Radix Select) or Combobox for search"),
		r(`<option\b`, "native <option>", "SelectItem inside a shadcn Select"),
		r(`<datalist\b`, "native <datalist>", "shadcn Combobox (Command + Popover)"),
		r(inputType("date"), "native date input", "shadcn Calendar in a Popover (DatePicker)"),
		r(inputType("datetime-local"), "native datetime input", "shadcn Calendar + time field"),
		r(inputType("time"), "native time input", "a custom time field built on Radix"),
		r(inputType("color"), "native colour input", "a custom colour picker"),
		r(inputType("range"), "native range input", "shadcn Slider (Radix Slider)"),
		r(inputType("checkbox"), "native checkbox", "shadcn Checkbox (Radix Checkbox)"),
		r(inputType("radio"), "native radio", "shadcn RadioGroup (Radix RadioGroup)"),
		r(inputType("file"), "native file input", "a styled trigger delegating to a hidden input"),
		r(`<dialog\b`, "native <dialog>", "shadcn Dialog / AlertDialog (Radix Dialog)"),
		r(`<details\b`, "native <details>", "shadcn Accordion or Collapsible (Radix)"),
		r(`<summary\b`, "native <summary>", "AccordionTrigger"),
		r(`<progress\b`, "native <progress>", "shadcn Progress (Radix Progress)"),
		r(`<meter\b`, "native <meter>", "a token-driven meter component"),
		r(`\bwindow\.alert\s*\(|`+bare("alert"), "window.alert()", "shadcn AlertDialog or a toast"),
		r(`\bwindow\.confirm\s*\(|`+bare("confirm"), "window.confirm()", "shadcn AlertDialog"),
		r(`\bwindow\.prompt\s*\(|`+bare("prompt"), "window.prompt()", "a shadcn Dialog with a real field"),
	}
}()

var (
	nativeExts = extSet(".tsx", ".jsx", ".vue", ".svelte", ".astro", ".html", ".htm")
	// A shadcn component file legitimately wraps the native element it
	// replaces: ui/select.tsx IS the Radix wrapper. Linting those would make
	// the rule unsatisfiable.
	nativeExempt = regexp.MustCompile(`(?i)(^|/)(components/ui|ui)/`)
	nativeSkip   = map[string]bool{"node_modules": true, "dist": true, "build": true, ".next": true, ".git": true}
)

func nativeFiles(target string) []string {
	st, err := os.Stat(target)
	if err != nil {
		return nil
	}
	if !st.IsDir() {
		if nativeExts(strings.ToLower(filepath.Ext(target))) {
			return []string{target}
		}
		return nil
	}
	var out []string
	_ = filepath.WalkDir(target, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !nativeExts(strings.ToLower(filepath.Ext(p))) {
			return nil
		}
		for _, part := range strings.Split(filepath.ToSlash(p), "/") {
			if nativeSkip[part] {
				return nil
			}
		}
		out = append(out, p)
		return nil
	})
	return out
}

// Native fails when a file uses a native interactive element that must be a
// custom shadcn/Radix component. Exit 0 clean, 1 findings, 2 bad usage.
func Native(w io.Writer, paths []string) int {
	if len(paths) == 0 {
		fmt.Fprintln(w, "usage: aval design gate native <file-or-dir>...")
		return 2
	}
	type finding struct {
		file         string
		line         int
		what, fix, s string
	}
	var found []finding
	checked := 0
	for _, t := range paths {
		if _, err := os.Stat(t); err != nil {
			fmt.Fprintf(w, "native: no such path: %s\n", t)
			return 2
		}
		for _, f := range nativeFiles(t) {
			if nativeExempt.MatchString(filepath.ToSlash(f)) {
				continue
			}
			checked++
			text, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			for i, line := range lines(strings.ToValidUTF8(string(text), "�")) {
				trimmed := strings.TrimLeft(line, " \t\f\v")
				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "#") {
					continue
				}
				for _, r := range nativeRules {
					if r.re.MatchString(line) {
						snippet := []rune(strings.TrimSpace(line))
						if len(snippet) > 90 {
							snippet = snippet[:90]
						}
						found = append(found, finding{f, i + 1, r.what, r.fix, string(snippet)})
					}
				}
			}
		}
	}
	if len(found) == 0 {
		fmt.Fprintf(w, "native: clean (%d file(s) checked)\n", checked)
		return 0
	}
	fmt.Fprintf(w, "native: %d native element(s) found\n\n", len(found))
	for _, f := range found {
		fmt.Fprintf(w, "  %s:%d\n    %s  ->  use %s\n    | %s\n", f.file, f.line, f.what, f.fix, f.s)
	}
	fmt.Fprintln(w, "\nNative interactive elements are not permitted in this setup.")
	fmt.Fprintln(w, "See reference/native-to-shadcn.md for the replacement for each one.")
	return 1
}

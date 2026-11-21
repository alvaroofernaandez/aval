package gate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type pair struct {
	fg, bg string
	min    float64
	label  string
}

// Required pairs fail the gate: readable text and actions (WCAG 1.4.3) and
// the border that identifies a control (1.4.11).
var requiredPairs = []pair{
	{"semantic.text.primary", "semantic.surface.page", 4.5, "body text on page"},
	{"semantic.text.primary", "semantic.surface.card", 4.5, "body text on card"},
	{"semantic.text.secondary", "semantic.surface.page", 4.5, "secondary text on page"},
	{"semantic.text.link", "semantic.surface.page", 4.5, "link on page"},
	{"semantic.text.on-action", "semantic.action.primary", 4.5, "text on primary action"},
	{"semantic.border.strong", "semantic.surface.page", 3.0, "essential control border (WCAG 1.4.11)"},
}

// Advisory pairs are reported, never failed: deliberately de-emphasised text
// and decoration.
var advisoryPairs = []pair{
	{"semantic.text.tertiary", "semantic.surface.page", 4.5, "tertiary/incidental text on page"},
	{"semantic.text.secondary", "semantic.surface.raised", 4.5, "secondary text on raised surface"},
	{"semantic.border.default", "semantic.surface.page", 3.0, "default border on page (decorative)"},
}

func lookup(data any, dotted string) any {
	node := data
	for _, k := range strings.Split(dotted, ".") {
		m, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		if node, ok = m[k]; !ok {
			return nil
		}
	}
	return node
}

// resolveValue follows {alias} chains to a final, valid hex string.
func resolveValue(data, val any) string {
	for i := 0; i < 12; i++ {
		s, ok := val.(string)
		if !ok {
			return ""
		}
		if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
			if strings.HasPrefix(s, "#") {
				if _, err := ParseHex(s); err == nil {
					return s
				}
			}
			return ""
		}
		ref := strings.TrimSpace(s[1 : len(s)-1])
		for strings.HasPrefix(ref, "../") || strings.HasPrefix(ref, "./") {
			if strings.HasPrefix(ref, "../") {
				ref = ref[3:]
			} else {
				ref = ref[2:]
			}
		}
		node, ok := lookup(data, ref).(map[string]any)
		if !ok {
			return ""
		}
		if val, ok = node["$value"]; !ok {
			return ""
		}
	}
	return ""
}

// resolveToken resolves a semantic path to hex, honouring a dark override map.
func resolveToken(data any, dotted string, overrides any) string {
	if overrides != nil && strings.HasPrefix(dotted, "semantic.") {
		if ov, ok := lookup(overrides, dotted[len("semantic."):]).(map[string]any); ok {
			if v, ok := ov["$value"]; ok {
				return resolveValue(data, v)
			}
		}
	}
	node, ok := lookup(data, dotted).(map[string]any)
	if !ok {
		return ""
	}
	v, ok := node["$value"]
	if !ok {
		return ""
	}
	return resolveValue(data, v)
}

func checkPairs(w io.Writer, data, overrides any, mode string, pairs []pair, aaa, required bool) []string {
	kind := "advisory"
	if required {
		kind = "required"
	}
	fmt.Fprintf(w, "\n=== %s (%s) ===\n", mode, kind)
	var issues []string
	for _, p := range pairs {
		fg, bg := resolveToken(data, p.fg, overrides), resolveToken(data, p.bg, overrides)
		if fg == "" || bg == "" {
			// A required pair whose tokens are absent measures nothing, so
			// it fails rather than skipping to a false "all pass".
			m := "?"
			if required {
				m = "FAIL"
				issues = append(issues, fmt.Sprintf("%s: %s - token missing (%s=%q, %s=%q)", mode, p.label, p.fg, fg, p.bg, bg))
			}
			fmt.Fprintf(w, "  %s %s: token missing (%s=%q, %s=%q)\n", m, p.label, p.fg, fg, p.bg, bg)
			continue
		}
		r, _ := Ratio(fg, bg)
		ok := r >= p.min
		m := "ok"
		switch {
		case required && ok:
			m = "PASS"
		case required:
			m = "FAIL"
		case !ok:
			m = "warn"
		}
		extra := ""
		if aaa && p.min >= 4.5 {
			extra = "  AAA(7:1): no"
			if r >= 7 {
				extra = "  AAA(7:1): pass"
			}
		}
		fmt.Fprintf(w, "  %s %s: %.2f:1 (need %g)  [%s on %s]%s\n", m, p.label, r, p.min, fg, bg, extra)
		if !ok && required {
			issues = append(issues, fmt.Sprintf("%s: %s %.2f:1 < %g", mode, p.label, r, p.min))
		}
	}
	return issues
}

// Pairs checks the semantic colour pairs of a DTCG colours file in light and,
// when it has a top-level "dark" override section, in dark too.
func Pairs(w io.Writer, file string, aaa bool) int {
	b, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(w, "ERROR: %s not found\n", file)
		return 1
	}
	fmt.Fprintf(w, "Source: %s\n", file)
	var data any
	if err := json.Unmarshal(b, &data); err != nil {
		fmt.Fprintf(w, "ERROR: invalid JSON - %v\n", err)
		return 1
	}
	var dark any
	if m, ok := data.(map[string]any); ok {
		if d, ok := m["dark"].(map[string]any); ok {
			dark = d
		}
	}
	fails := checkPairs(w, data, nil, "LIGHT", requiredPairs, aaa, true)
	if dark != nil {
		fails = append(fails, checkPairs(w, data, dark, "DARK", requiredPairs, aaa, true)...)
	}
	checkPairs(w, data, nil, "LIGHT", advisoryPairs, aaa, false)
	if dark != nil {
		checkPairs(w, data, dark, "DARK", advisoryPairs, aaa, false)
	} else {
		fmt.Fprintln(w, "\n(no dark section found - skipping dark checks)")
	}
	fmt.Fprintln(w)
	if len(fails) > 0 {
		fmt.Fprintf(w, "FAIL: %d required pair(s) below WCAG minimum:\n", len(fails))
		for _, f := range fails {
			fmt.Fprintln(w, "  x "+f)
		}
		return 1
	}
	fmt.Fprintln(w, "OK: all required contrast pairs pass WCAG 2.2 minimums.")
	return 0
}

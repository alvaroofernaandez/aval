// Package designgate holds aval's design gates: WCAG contrast, DTCG token
// checks, hardcoded-value lint, theme-reference check, component-spec check
// and the no-emoji check. Every gate prints measured numbers and returns a
// process exit code: 0 clean, 1 a check failed, 2 bad usage.
package designgate

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// ParseHex reads "#rgb", "#rrggbb" or the same without the hash.
func ParseHex(s string) ([3]int, error) {
	h := strings.TrimLeft(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	var rgb [3]int
	if len(h) != 6 {
		return rgb, fmt.Errorf("bad hex: %q", h)
	}
	for i := range rgb {
		v, err := strconv.ParseUint(h[i*2:i*2+2], 16, 8)
		if err != nil {
			return rgb, fmt.Errorf("bad hex: %q", h)
		}
		rgb[i] = int(v)
	}
	return rgb, nil
}

func lin(c int) float64 {
	f := float64(c) / 255
	if f <= 0.03928 {
		return f / 12.92
	}
	return math.Pow((f+0.055)/1.055, 2.4)
}

func luminance(rgb [3]int) float64 {
	return 0.2126*lin(rgb[0]) + 0.7152*lin(rgb[1]) + 0.0722*lin(rgb[2])
}

// Ratio is the WCAG 2.2 contrast ratio between two hex colours.
func Ratio(fg, bg string) (float64, error) {
	a, err := ParseHex(fg)
	if err != nil {
		return 0, err
	}
	b, err := ParseHex(bg)
	if err != nil {
		return 0, err
	}
	l1, l2 := luminance(a), luminance(b)
	return (math.Max(l1, l2) + 0.05) / (math.Min(l1, l2) + 0.05), nil
}

func mark(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

// Contrast prints the ratio and the AA/AAA verdicts. The exit code follows AA
// for normal text, or for large text with large=true.
func Contrast(w io.Writer, fg, bg string, large bool) int {
	r, err := Ratio(fg, bg)
	if err != nil {
		fmt.Fprintf(w, "ERROR: %v\n", err)
		return 2
	}
	fmt.Fprintf(w, "Contrast %s on %s: %.2f:1\n\n", fg, bg, r)
	fmt.Fprintf(w, "  Normal text  AA  (4.5:1): %s\n", mark(r >= 4.5))
	fmt.Fprintf(w, "  Normal text  AAA (7.0:1): %s\n", mark(r >= 7.0))
	fmt.Fprintf(w, "  Large text   AA  (3.0:1): %s\n", mark(r >= 3.0))
	fmt.Fprintf(w, "  Large text   AAA (4.5:1): %s\n", mark(r >= 4.5))
	fmt.Fprintf(w, "  UI / graphics    (3.0:1): %s\n", mark(r >= 3.0))
	need := 4.5
	if large {
		need = 3.0
	}
	if r >= need {
		return 0
	}
	return 1
}

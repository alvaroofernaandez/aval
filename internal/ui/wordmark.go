package ui

import "github.com/alvaroofernaandez/aval/internal/ui/caps"

//go:generate go run ../../tools/brand --wordmark-term wordmark_gen.go

// wordmarkMinWidth is the size rule of DESIGN.md section 10: from 40 columns
// the hub shows the wordmark (stacked under 80, beside the panel from 80);
// compact, under 40, never does.
const wordmarkMinWidth = 40

// wordmark returns the hub wordmark tier for c: quadrant blocks when the
// terminal renders Unicode, # otherwise. It returns nil below the size rule
// or when the height cannot hold every row, so the caller falls back to the
// one-line lockup. The rows are a copy, unstyled; the hub renders them bold
// in ink.
func wordmark(c caps.Caps, width, height int) []string {
	tier := wordmarkASCII
	if c.Unicode {
		tier = wordmarkUnicode
	}
	if width < wordmarkMinWidth || height < len(tier) {
		return nil
	}
	return append([]string(nil), tier...)
}

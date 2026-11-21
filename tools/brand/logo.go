package main

import (
	"fmt"
	"math"
	"strings"
)

// The aval mark and wordmark, drawn from geometry (port of docs/logo.py).
// Everything is a stroked primitive, never a font.

var ink = map[string]string{"light": "#000000", "dark": "#ffffff"}

// Wordmark geometry, in units of the x-height.
const (
	wordH   = 100.0 // x-height
	wordS   = 22.0  // stroke
	wordASC = 150.0 // ascender
	wordGap = 30.0  // space between letters, edge to edge
	checkW  = 112.0
)

// Isotype geometry on a 512 canvas.
const (
	markM  = 512
	markMS = 64
)

var (
	ring     = [3]int{184, 314, 112} // cx, cy, r
	checkTip = [2]int{440, 86}
)

type glyph func(x, base float64) ([]string, float64)

func glyphA(x, base float64) ([]string, float64) {
	r := float64(wordH-wordS) / 2
	cx, cy := float64(x+wordS/2)+r, base-wordH/2
	stem := cx + r
	return []string{
		fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f"/>`, cx, cy, r),
		fmt.Sprintf(`<path d="M %.1f %.1f V %.1f"/>`, stem, base-wordH+wordS/2, base-wordS/2),
	}, wordH
}

func glyphCheck(x, base float64) ([]string, float64) {
	w, top := checkW, wordASC
	p0 := [2]float64{x + wordS/2, base - wordH + wordS/2}
	p1 := [2]float64{x + float64(w*0.42), base - wordS/2}
	p2 := [2]float64{x + w - wordS/2, base - top + wordS/2}
	d := fmt.Sprintf("M %.1f %.1f L %.1f %.1f L %.1f %.1f", p0[0], p0[1], p1[0], p1[1], p2[0], p2[1])
	return []string{fmt.Sprintf(`<path d="%s"/>`, d)}, w
}

func glyphL(x, base float64) ([]string, float64) {
	cx := x + wordS/2
	return []string{fmt.Sprintf(`<path d="M %.1f %.1f V %.1f"/>`, cx, base-wordASC+wordS/2, base-wordS/2)}, wordS
}

// wordPaths returns the four letters as SVG elements and the total width.
func wordPaths(x, base float64) ([]string, float64) {
	var parts []string
	cursor := x
	steps := []struct {
		g   glyph
		gap float64
	}{
		{glyphA, wordGap}, {glyphCheck, float64(wordGap * 0.55)}, {glyphA, wordGap}, {glyphL, wordGap},
	}
	for _, s := range steps {
		elems, w := s.g(cursor, base)
		parts = append(parts, elems...)
		cursor += w + s.gap
	}
	return parts, cursor - wordGap - x
}

func wordGroup(inkColor string, x, y, scale float64) string {
	elems, _ := wordPaths(0, wordASC)
	return fmt.Sprintf(`<g transform="translate(%.1f %.1f) scale(%.4f)" fill="none" `+
		`stroke="%s" stroke-width="%s" stroke-linecap="round" `+
		`stroke-linejoin="round">`, x, y, scale, inkColor, pyFloat(wordS)) + strings.Join(elems, "") + "</g>"
}

func wordSize() (float64, float64) {
	_, w := wordPaths(0, wordASC)
	return w, wordASC
}

func wordmark(theme string) string {
	w, h := wordSize()
	pad := float64(wordS)
	vw, vh := w+pad*2, h+pad*2
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" `+
		`width="%.0f" height="%.0f" role="img" aria-label="aval">`+"\n", vw, vh, vw, vh) +
		wordGroup(ink[theme], pad, pad, 1.0) + "\n</svg>\n"
}

func mark(theme string) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" `+
		`width="%d" height="%d" role="img" aria-label="aval">`+"\n", markM, markM, markM, markM) +
		markGroup(ink[theme]) + "\n</svg>\n"
}

// markGroup is the isotype on the 512 canvas: the a's ring and the check,
// drawn as one stroke.
func markGroup(inkColor string) string {
	cx, cy, r := ring[0], ring[1], ring[2]
	tx, ty := checkTip[0], checkTip[1]
	jx, jy := cx+r+40, cy+r
	return fmt.Sprintf(`<g fill="none" stroke="%s" stroke-width="%d" stroke-linecap="round" stroke-linejoin="round">`, inkColor, markMS) +
		fmt.Sprintf(`<circle cx="%d" cy="%d" r="%d"/>`, cx, cy, r) +
		fmt.Sprintf(`<path d="M %d %d L %d %d L %d %d"/>`, cx+r, cy, jx, jy, tx, ty) + "</g>"
}

// The desktop app (the future aval shell and window): the isotype inside an
// app frame, followed by a terminal cursor. Same stroke weight as the mark.
const (
	deskInset = 32
	deskScale = 0.56
	deskCX    = 236.0
	deskCY    = 240.0
)

func desktopGroup(inkColor string) string {
	size := markM - 2*deskInset
	sw := float64(markMS) * deskScale
	tx, ty := deskCX-float64(markM/2)*deskScale, deskCY-float64(markM/2)*deskScale
	return fmt.Sprintf(`<g data-part="desktop" fill="none" stroke="%s" stroke-linecap="round" stroke-linejoin="round">`, inkColor) +
		fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" rx="112" stroke-width="%.1f"/>`, deskInset, deskInset, size, size, sw) +
		fmt.Sprintf(`<g transform="translate(%.1f %.1f) scale(%.2f)">`, tx, ty, deskScale) + markGroup(inkColor) + "</g>" +
		fmt.Sprintf(`<path d="M %d %d H %d" stroke-width="%.1f"/>`, 314, 388, 384, sw) + "</g>"
}

func desktop(theme string) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" `+
		`width="%d" height="%d" role="img" aria-label="aval desktop">`+"\n", markM, markM, markM, markM) +
		desktopGroup(ink[theme]) + "\n</svg>\n"
}

// pyFloat renders an integral float like Python's str(float): 22.0, 20.0.
func pyFloat(f float64) string { return fmt.Sprintf("%.1f", f) }

func pow(x, y float64) float64 { return math.Pow(x, y) }

func minf(a, b float64) float64 { return math.Min(a, b) }

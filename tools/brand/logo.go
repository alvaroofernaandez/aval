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

// prim is one stroked primitive of the wordmark: a circle (a ring) or an
// open polyline. The SVG and the terminal raster both draw from these, so the
// geometry lives in one place.
type prim struct {
	circle bool
	pts    [][2]float64 // circle: the centre; polyline: its vertices
	r      float64
}

// svg renders the primitive; a vertical step is written as V, as the
// published assets have always been.
func (p prim) svg() string {
	if p.circle {
		return fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f"/>`, p.pts[0][0], p.pts[0][1], p.r)
	}
	d := fmt.Sprintf("M %.1f %.1f", p.pts[0][0], p.pts[0][1])
	for i := 1; i < len(p.pts); i++ {
		if p.pts[i][0] == p.pts[i-1][0] {
			d += fmt.Sprintf(" V %.1f", p.pts[i][1])
		} else {
			d += fmt.Sprintf(" L %.1f %.1f", p.pts[i][0], p.pts[i][1])
		}
	}
	return fmt.Sprintf(`<path d="%s"/>`, d)
}

type glyph func(x, base float64) ([]prim, float64)

// glyphA is a ring plus a stem on its right edge.
func glyphA(x, base float64) ([]prim, float64) {
	r := float64(wordH-wordS) / 2
	cx, cy := float64(x+wordS/2)+r, base-wordH/2
	stem := cx + r
	return []prim{
		{circle: true, pts: [][2]float64{{cx, cy}}, r: r},
		{pts: [][2]float64{{stem, base - wordH + wordS/2}, {stem, base - wordS/2}}},
	}, wordH
}

// glyphCheck is the v: a check whose long arm rises to the ascender.
func glyphCheck(x, base float64) ([]prim, float64) {
	w, top := checkW, wordASC
	p0 := [2]float64{x + wordS/2, base - wordH + wordS/2}
	p1 := [2]float64{x + float64(w*0.42), base - wordS/2}
	p2 := [2]float64{x + w - wordS/2, base - top + wordS/2}
	return []prim{{pts: [][2]float64{p0, p1, p2}}}, w
}

func glyphL(x, base float64) ([]prim, float64) {
	cx := x + wordS/2
	return []prim{{pts: [][2]float64{{cx, base - wordASC + wordS/2}, {cx, base - wordS/2}}}}, wordS
}

// wordPrims returns the four letters as primitives and the total width.
func wordPrims(x, base float64) ([]prim, float64) {
	var prims []prim
	cursor := x
	steps := []struct {
		g   glyph
		gap float64
	}{
		{glyphA, wordGap}, {glyphCheck, float64(wordGap * 0.55)}, {glyphA, wordGap}, {glyphL, wordGap},
	}
	for _, s := range steps {
		ps, w := s.g(cursor, base)
		prims = append(prims, ps...)
		cursor += w + s.gap
	}
	return prims, cursor - wordGap - x
}

// wordPaths returns the four letters as SVG elements and the total width.
func wordPaths(x, base float64) ([]string, float64) {
	prims, w := wordPrims(x, base)
	parts := make([]string, len(prims))
	for i, p := range prims {
		parts[i] = p.svg()
	}
	return parts, w
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

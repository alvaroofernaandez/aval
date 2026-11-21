package main

import (
	"fmt"
	"math"
	"strings"
)

// The README hero: the aval mark built out of passed checks (port of docs/hero.py).

const (
	heroW, heroH = 1200, 520
	heroPad      = 56
	cells        = 20
	cell         = 17
	matrixX      = heroW - heroPad - cells*cell
)

type heroTheme struct{ bg, ink, muted, faint string }

var heroThemes = map[string]heroTheme{
	"light": {"#ffffff", "#000000", "#525252", "#d4d4d4"},
	"dark":  {"#000000", "#ffffff", "#a3a3a3", "#2e2e2e"},
}

const (
	monoFont = "ui-monospace,SFMono-Regular,Menlo,Consolas,monospace"
	sansFont = "ui-sans-serif,-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif"
)

func segDist(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := math.Max(0.0, math.Min(1.0, (float64((px-ax)*dx)+float64((py-ay)*dy))/(float64(dx*dx)+float64(dy*dy))))
	return math.Hypot(px-(ax+float64(t*dx)), py-(ay+float64(t*dy)))
}

// onMark reports whether (u, v), in the logo's 512 canvas, is on the mark's stroke.
func onMark(u, v float64) bool {
	rx, ry, r := float64(ring[0]), float64(ring[1]), float64(ring[2])
	tx, ty := float64(checkTip[0]), float64(checkTip[1])
	jx, jy := rx+r+40, ry+r
	half := float64(markMS) / 2
	if math.Abs(math.Hypot(u-rx, v-ry)-r) <= half {
		return true
	}
	return segDist(u, v, rx+r, ry, jx, jy) <= half || segDist(u, v, jx, jy, tx, ty) <= half
}

func miniCheck(x, y float64, on bool, t heroTheme) string {
	p := [][2]float64{{0.24, 0.54}, {0.43, 0.72}, {0.78, 0.30}}
	segs := make([]string, len(p))
	for i, ab := range p {
		segs[i] = fmt.Sprintf("%.1f %.1f", x+float64(ab[0]*cell), y+float64(ab[1]*cell))
	}
	d := "M " + strings.Join(segs, " L ")
	if on {
		return fmt.Sprintf(`<path class="ck on" d="%s" stroke="%s" stroke-width="2.2"/>`, d, t.ink)
	}
	return fmt.Sprintf(`<path class="ck" d="%s" stroke="%s" stroke-width="1.3"/>`, d, t.faint)
}

func matrix(t heroTheme, top float64) []string {
	scale := 512.0 / float64(cells*cell)
	out := []string{`<g data-part="matrix" fill="none" stroke-linecap="round" stroke-linejoin="round">`}
	for row := 0; row < cells; row++ {
		for col := 0; col < cells; col++ {
			u := float64((float64(col)+0.5)*cell) * scale
			v := float64((float64(row)+0.5)*cell) * scale
			out = append(out, miniCheck(float64(matrixX+col*cell), top+float64(row*cell), onMark(u, v), t))
		}
	}
	return append(out, "</g>")
}

func heroBuild(theme string) string {
	t := heroThemes[theme]
	scale := 1.45
	_, wh := wordSize()
	const claimGap, claimSize, descGap = 74.0, 36, 42.0
	block := float64(wh*scale) + claimGap + descGap
	top := (heroH - block) / 2
	claimY := top + float64(wh*scale) + claimGap
	descY := claimY + descGap

	parts := []string{
		fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" `+
			`width="%d" height="%d" role="img" `+
			`aria-label="aval. Nada sin pruebas. Harness personal para agentes de IA.">`, heroW, heroH, heroW, heroH),
		fmt.Sprintf(`<rect width="%d" height="%d" fill="%s"/>`, heroW, heroH, t.bg),
		wordGroup(t.ink, heroPad-4, top, scale),
		fmt.Sprintf(`<text x="%d" y="%.0f" font-family="%s" font-size="%d" `+
			`font-weight="600" letter-spacing="-0.8" fill="%s">Nada sin pruebas.</text>`,
			heroPad, claimY, sansFont, claimSize, t.ink),
		fmt.Sprintf(`<text x="%d" y="%.0f" font-family="%s" font-size="13" `+
			`letter-spacing="3" fill="%s">HARNESS PERSONAL PARA AGENTES DE IA</text>`,
			heroPad+2, descY, monoFont, t.muted),
	}
	parts = append(parts, matrix(t, float64(heroH-cells*cell)/2)...)
	parts = append(parts, "</svg>")
	return strings.Join(parts, "\n")
}

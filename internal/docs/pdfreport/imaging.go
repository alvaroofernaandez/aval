package pdfreport

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// lanczos3 is the Lanczos kernel with a = 3, the filter Pillow's LANCZOS uses.
func lanczos3(x float64) float64 {
	if x == 0 {
		return 1
	}
	if x <= -3 || x >= 3 {
		return 0
	}
	px := math.Pi * x
	return 3 * math.Sin(px) * math.Sin(px/3) / (px * px)
}

// resize scales src to w x h with a separable Lanczos filter on premultiplied
// alpha, so a transparent edge never bleeds its colour into the result.
func resize(src image.Image, w, h int) *image.NRGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	// premultiplied, 16-bit, as floats
	pix := make([][4]float64, sw*sh)
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			pix[y*sw+x] = [4]float64{float64(r), float64(g), float64(bl), float64(a)}
		}
	}
	horiz := resizeAxis(pix, sw, sh, w, true)
	both := resizeAxis(horiz, w, sh, h, false)

	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i, p := range both {
		a := clamp16(p[3])
		if a == 0 {
			continue
		}
		af := float64(a)
		out.Pix[i*4+0] = to8(float64(clamp16(p[0])) / af)
		out.Pix[i*4+1] = to8(float64(clamp16(p[1])) / af)
		out.Pix[i*4+2] = to8(float64(clamp16(p[2])) / af)
		out.Pix[i*4+3] = uint8(math.Round(af / 257))
	}
	return out
}

func clamp16(v float64) uint32 {
	if v < 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint32(math.Round(v))
}

func to8(ratio float64) uint8 {
	v := math.Round(ratio * 255)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// resizeAxis resamples a w x h grid of pixels along one axis.
func resizeAxis(pix [][4]float64, w, h, to int, horizontal bool) [][4]float64 {
	from, other := h, w
	if horizontal {
		from, other = w, h
	}
	scale := float64(from) / float64(to)
	fs := math.Max(scale, 1)
	support := 3 * fs
	type tap struct {
		start int
		wt    []float64
	}
	taps := make([]tap, to)
	for o := 0; o < to; o++ {
		center := (float64(o) + 0.5) * scale
		lo := int(math.Max(math.Floor(center-support+0.5), 0))
		hi := int(math.Min(math.Floor(center+support+0.5), float64(from)))
		wt := make([]float64, 0, hi-lo)
		sum := 0.0
		for i := lo; i < hi; i++ {
			k := lanczos3((float64(i) - center + 0.5) / fs)
			wt = append(wt, k)
			sum += k
		}
		if sum != 0 {
			for i := range wt {
				wt[i] /= sum
			}
		}
		taps[o] = tap{lo, wt}
	}
	var out [][4]float64
	if horizontal {
		out = make([][4]float64, to*other)
	} else {
		out = make([][4]float64, other*to)
	}
	for o := 0; o < to; o++ {
		for k := 0; k < other; k++ {
			var acc [4]float64
			for i, wv := range taps[o].wt {
				var p [4]float64
				if horizontal {
					p = pix[k*w+taps[o].start+i]
				} else {
					p = pix[(taps[o].start+i)*w+k]
				}
				for c := 0; c < 4; c++ {
					acc[c] += p[c] * wv
				}
			}
			if horizontal {
				out[k*to+o] = acc
			} else {
				out[o*other+k] = acc
			}
		}
	}
	return out
}

// recolour keeps the alpha silhouette of m and replaces its colour.
func recolour(m *image.NRGBA, c [3]uint8) *image.NRGBA {
	out := image.NewNRGBA(m.Bounds())
	for i := 0; i < len(m.Pix); i += 4 {
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = c[0], c[1], c[2], m.Pix[i+3]
	}
	return out
}

// flatten drops the alpha channel, keeping the colour channels as they are.
func flatten(m *image.NRGBA) *image.RGBA {
	out := image.NewRGBA(m.Bounds())
	for i := 0; i < len(m.Pix); i += 4 {
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = m.Pix[i], m.Pix[i+1], m.Pix[i+2], 255
	}
	return out
}

// circularPortrait crops to the face, masks to a circle and bakes in the ring.
// Head-and-shoulders portraits sit above the geometric centre, so the square
// is centred at 44% of the height. Baking the circle into the asset keeps the
// crop out of the hands of whatever renders the PDF.
func circularPortrait(src image.Image, size int) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	side := int(float64(min(w, h)) * 0.86)
	cx, cy := w/2, int(float64(h)*0.44)
	left := max(0, min(w-side, cx-side/2))
	top := max(0, min(h-side, cy-side/2))
	sub := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(sub, sub.Bounds(), src, image.Pt(b.Min.X+left, b.Min.Y+top), draw.Src)
	im := resize(sub, size, size)

	const ss = 4 // coverage sampled at 4x for clean antialiased edges
	r := float64(size) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			in := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					dx := float64(x) + (float64(sx)+0.5)/ss - r
					dy := float64(y) + (float64(sy)+0.5)/ss - r
					if dx*dx+dy*dy <= r*r {
						in++
					}
				}
			}
			i := im.PixOffset(x, y)
			im.Pix[i+3] = uint8(math.Round(255 * float64(in) / (ss * ss)))
		}
	}
	// A 3px translucent white ring just inside the edge.
	outer := r - 1
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)+0.5-r, float64(y)+0.5-r)
			if d <= outer && d > outer-3 {
				im.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 110})
			}
		}
	}
	return im
}

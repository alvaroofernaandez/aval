// Command brand generates every image the README shows. It is a dev tool and
// is not part of the shipped aval binary.
//
//	go run ./tools/brand              # writes into docs/assets/
//	go run ./tools/brand --out DIR    # writes somewhere else (tests use this)
//	go run ./tools/brand --list       # prints the tool labels, one per line
//	go run ./tools/brand --no-logo    # prints the tools with no published logo
//
// Outputs come in a light and a dark variant: hero-*, mark-*, wordmark-*,
// desktop-* (the future desktop app) and stack-* (today's tools and what
// replaces them on the roadmap). Logos are vendored under docs/logos/ (provenance in
// docs/logos/SOURCES.md) and embedded as data URIs, because GitHub renders a
// README SVG as an <img>, which cannot load files outside itself.
// tests/readme_test.go fails if the committed images drift from this output.
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type tool struct{ label, logo, mono string }

type theme struct{ fg, muted, card, edge, mono string }

var themes = map[string]theme{
	"light": {"#1f2328", "#59636e", "#ededed", "#d4d4d4", "#1f2328"},
	"dark":  {"#f0f6fc", "#9198a1", "#262626", "#3a3a3a", "#f0f0f0"},
}

// Generation order (Python dict order); output is order-independent.
var themeOrder = []string{"light", "dark"}

var (
	darkAdapt = map[string]bool{"apple.svg": true, "tmux.svg": true}
	darkFiles = map[string]string{}
	tiles     = map[string]bool{"engram.png": true, "rtk.png": true, "opencode.svg": true, "herdr.svg": true}
)

const lightInk = "#f0f0f0"

const (
	cols    = 8
	tileW   = 128
	stackW  = cols * tileW
	card    = 88
	logoSz  = 60
	head    = 30
	hexExpr = `#([0-9a-fA-F]{6}|[0-9a-fA-F]{3})\b`
)

var (
	hexRe     = regexp.MustCompile(hexExpr)
	svgOpenRe = regexp.MustCompile(`<svg\b`)
	viewBoxRe = regexp.MustCompile(`viewBox="[\d.\-]+[ ,]+[\d.\-]+[ ,]+([\d.]+)[ ,]+([\d.]+)"`)
)

func luminance(hex6 string) float64 {
	c := func(v float64) float64 {
		v /= 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return pow((v+0.055)/1.055, 2.4)
	}
	ch := func(i int) float64 {
		n, _ := strconv.ParseUint(hex6[i:i+2], 16, 8)
		return float64(n)
	}
	return float64(0.2126*c(ch(0))) + float64(0.7152*c(ch(2))) + float64(0.0722*c(ch(4)))
}

// lightenDarks turns the dark tones of a mark light and leaves brand colours.
func lightenDarks(svg string) string {
	svg = hexRe.ReplaceAllStringFunc(svg, func(m string) string {
		h := m[1:]
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if luminance(h) < 0.05 {
			return lightInk
		}
		return m
	})
	done := false
	return svgOpenRe.ReplaceAllStringFunc(svg, func(m string) string {
		if done {
			return m
		}
		done = true
		return `<svg fill="` + lightInk + `"`
	})
}

type gen struct{ logos string }

// aspect is width / height of a vendored logo, from the PNG header or SVG viewBox.
func (g gen) aspect(name string) (float64, error) {
	b, err := os.ReadFile(filepath.Join(g.logos, name))
	if err != nil {
		return 0, err
	}
	if filepath.Ext(name) == ".png" {
		return float64(binary.BigEndian.Uint32(b[16:20])) / float64(binary.BigEndian.Uint32(b[20:24])), nil
	}
	m := viewBoxRe.FindStringSubmatch(string(b))
	if m == nil {
		return 1.0, nil
	}
	w, _ := strconv.ParseFloat(m[1], 64)
	h, _ := strconv.ParseFloat(m[2], 64)
	return w / h, nil
}

func (g gen) dataURI(name, th string) (string, error) {
	if th == "dark" {
		if alt, ok := darkFiles[name]; ok {
			name = alt
		}
	}
	data, err := os.ReadFile(filepath.Join(g.logos, name))
	if err != nil {
		return "", err
	}
	mime := map[string]string{".svg": "image/svg+xml", ".png": "image/png"}[filepath.Ext(name)]
	if th == "dark" && darkAdapt[name] {
		data = []byte(lightenDarks(string(data)))
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func (g gen) tile(x, y int, tl tool, t theme, th string) ([]string, error) {
	cx := float64(x) + float64(tileW)/2
	top := y + head
	out := []string{fmt.Sprintf(`<rect class="card" x="%.1f" y="%d" width="%d" height="%d" `+
		`rx="20" fill="%s" stroke="%s" stroke-width="1"/>`, cx-float64(card)/2, top, card, card, t.card, t.edge)}
	if tl.logo != "" {
		off := float64(card-logoSz) / 2
		ix, iy := cx-float64(card)/2+off, float64(top)+off
		extra := ""
		if tiles[tl.logo] {
			cid := "tile-" + strings.SplitN(tl.logo, ".", 2)[0]
			r, err := g.aspect(tl.logo)
			if err != nil {
				return nil, err
			}
			cw, ch := float64(logoSz), float64(logoSz)/r
			if r < 1 {
				cw, ch = float64(logoSz)*r, float64(logoSz)
			}
			cx0, cy0 := ix+(float64(logoSz)-cw)/2, iy+(float64(logoSz)-ch)/2
			rad := minf(14, minf(cw, ch)*0.25)
			out = append(out, fmt.Sprintf(`<clipPath id="%s"><rect x="%.1f" y="%.1f" width="%.1f" `+
				`height="%.1f" rx="%.1f"/></clipPath>`, cid, cx0, cy0, cw, ch, rad))
			extra += fmt.Sprintf(` clip-path="url(#%s)"`, cid)
		}
		if th == "dark" && (darkAdapt[tl.logo] || darkFiles[tl.logo] != "") {
			extra += ` data-adapted="dark"`
		}
		uri, err := g.dataURI(tl.logo, th)
		if err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf(`<image class="logo" width="%d" height="%d" `+
			`x="%.1f" y="%.1f"%s `+
			`preserveAspectRatio="xMidYMid meet" href="%s"/>`, logoSz, logoSz, ix, iy, extra, uri))
	} else {
		out = append(out, fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" `+
			`font-family="%s" font-size="26" font-weight="700" `+
			`fill="%s">%s</text>`, cx, float64(top)+float64(card)/2+9, monoFont, t.mono, esc(tl.mono)))
	}
	out = append(out, fmt.Sprintf(`<text x="%.1f" y="%d" text-anchor="middle" `+
		`font-family="%s" font-size="15" font-weight="500" `+
		`fill="%s">%s</text>`, cx, top+card+28, sansFont, t.fg, esc(tl.label)))
	return out, nil
}

func contentName(name, svg string) string {
	return fmt.Sprintf("%s.%x.svg", name, sha256.Sum256([]byte(svg)))[:len(name)+1+8] + ".svg"
}

type image struct{ name, svg string }

// writeAssets writes each image under its content name and deletes older versions.
func writeAssets(out string, images []image) (map[string]string, error) {
	names := map[string]string{}
	for _, im := range images {
		olds, _ := filepath.Glob(filepath.Join(out, im.name+".*.svg"))
		olds = append(olds, filepath.Join(out, im.name+".svg"))
		for _, old := range olds {
			if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
		fname := contentName(im.name, im.svg)
		if err := os.WriteFile(filepath.Join(out, fname), []byte(im.svg), 0o644); err != nil {
			return nil, err
		}
		names[im.name] = fname
	}
	return names, nil
}

// versionReadme points every README image URL at the current content-addressed file.
func versionReadme(readme string, names map[string]string) error {
	b, err := os.ReadFile(readme)
	if err != nil {
		return err
	}
	alts := make([]string, 0, len(names))
	for n := range names {
		alts = append(alts, regexp.QuoteMeta(n))
	}
	re := regexp.MustCompile(`docs/assets/(` + strings.Join(alts, "|") + `)(?:\.[0-9a-f]{8})?\.svg(?:\?v=[0-9a-f]+)?`)
	text := string(b)
	n := re.ReplaceAllStringFunc(text, func(m string) string {
		return "docs/assets/" + names[re.FindStringSubmatch(m)[1]]
	})
	if n != text {
		return os.WriteFile(readme, []byte(n), 0o644)
	}
	return nil
}

// findRepoRoot returns the nearest ancestor of dir (inclusive) holding go.mod.
func findRepoRoot(dir string) (string, error) {
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", errors.New("no go.mod found in any parent directory; pass --docs DIR")
		}
		d = parent
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("brand", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outFlag := fs.String("out", "", "output directory (default: <docs>/assets)")
	docsFlag := fs.String("docs", "", "docs directory (default: <repo root>/docs)")
	list := fs.Bool("list", false, "print the tool labels, one per line")
	noLogo := fs.Bool("no-logo", false, "print the tools with no published logo")
	termOut := fs.String("wordmark-term", "", "write the terminal wordmark Go file to `PATH` (internal/ui/wordmark_gen.go) and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *termOut != "" {
		return os.WriteFile(*termOut, wordmarkTermSource(), 0o644)
	}

	if *list || *noLogo {
		for _, tl := range allTools() {
			if *list || tl.logo == "" {
				fmt.Fprintln(stdout, tl.label)
			}
		}
		return nil
	}

	docs := *docsFlag
	if docs == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		root, err := findRepoRoot(wd)
		if err != nil {
			return err
		}
		docs = filepath.Join(root, "docs")
	}
	docs, _ = filepath.Abs(docs)
	assets := filepath.Join(docs, "assets")
	out := *outFlag
	if out == "" {
		out = assets
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	g := gen{logos: filepath.Join(docs, "logos")}
	var images []image
	for _, th := range themeOrder {
		st, err := g.stack(th)
		if err != nil {
			return err
		}
		images = append(images,
			image{"hero-" + th, heroBuild(th) + "\n"},
			image{"stack-" + th, st},
			image{"mark-" + th, mark(th)},
			image{"wordmark-" + th, wordmark(th)},
			image{"desktop-" + th, desktop(th)},
		)
	}
	names, err := writeAssets(out, images)
	if err != nil {
		return err
	}

	// Only the committed assets are referenced by the README.
	if sameDir(out, assets) {
		return versionReadme(filepath.Join(filepath.Dir(docs), "README.md"), names)
	}
	return nil
}

func sameDir(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "brand:", err)
		}
		os.Exit(1)
	}
}

// Package pdfreport scaffolds a print-grade A4 PDF project (the pdf-report
// skill) and builds the self-contained brand stylesheet it renders with.
package pdfreport

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	// Brand images may be PNG or JPEG.
	_ "image/gif"
)

// AssetOptions drive `aval pdf assets`.
type AssetOptions struct {
	Project  string   // the report project; assets/brand.css is written inside it
	BrandDir string   // holds the logo, banner.png and one <name>.* per portrait
	LogoFile string   // logo filename inside BrandDir
	People   []string // portrait names
	Ink      [3]uint8 // dark logo colourway
	Accent   [3]uint8 // branded logo colourway
	Get      func(url, userAgent string) ([]byte, error)
	Out      io.Writer
}

// DefaultInk and DefaultAccent are the skill's neutral defaults.
var (
	DefaultInk    = [3]uint8{23, 20, 31}
	DefaultAccent = [3]uint8{110, 59, 106}
)

// ParseRGB parses "r,g,b" with three 0-255 components.
func ParseRGB(s string) ([3]uint8, error) {
	parts := strings.Split(s, ",")
	var c [3]uint8
	if len(parts) != 3 {
		return c, fmt.Errorf("expected three 0-255 components, got: %s", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 || n > 255 {
			return c, fmt.Errorf("expected three 0-255 components, got: %s", s)
		}
		c[i] = uint8(n)
	}
	return c, nil
}

// HTTPGet is the real network fetch.
func HTTPGet(url, userAgent string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func dataURI(raw []byte, mime string) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

func pngURI(m image.Image) (string, error) {
	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&b, m); err != nil {
		return "", err
	}
	return dataURI(b.Bytes(), "image/png"), nil
}

func jpgURI(m image.Image, quality int) (string, error) {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, m, &jpeg.Options{Quality: quality}); err != nil {
		return "", err
	}
	return dataURI(b.Bytes(), "image/jpeg"), nil
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// Google Fonts serves one @font-face per unicode subset. Taking only the first
// one silently drops accents, n-tilde and the euro sign onto a fallback font,
// which is invisible in thumbnails and obvious in print. Embed every subset a
// Spanish-language document needs, with its unicode-range.
var fontQueries = []struct{ family, query string }{
	{"Fraunces", "Fraunces:opsz,wght@9..144,400;9..144,600;9..144,700;9..144,900"},
	{"Archivo", "Archivo:wght@400;500;600;700"},
	{"JetBrains Mono", "JetBrains+Mono:wght@400;500;700"},
}

const fontUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

var (
	wantedSubsets = map[string]bool{"latin": true, "latin-ext": true}
	faceRe        = regexp.MustCompile(`/\*\s*([\w-]+)\s*\*/\s*@font-face\s*\{([^}]+)\}`)
	woff2Re       = regexp.MustCompile(`url\((https://[^)]+\.woff2)\)`)
	commentRe     = regexp.MustCompile(`(?s)<!--.*?-->`)
)

func cssField(body, name string) string {
	m := regexp.MustCompile(name + `:\s*([^;]+);`).FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// Lucide icons ship as stroked SVG. Embedding them as CSS mask-images lets a
// single rule inherit currentColor and font-size, so an icon always matches
// the text it sits next to.
var icons = []string{
	"triangle-alert", "circle-check", "circle-x", "clock", "euro", "users", "layers",
	"database", "shopping-cart", "smartphone", "file-pen-line", "search", "shield-check",
	"trending-up", "git-branch", "zap", "target", "lightbulb", "lock", "handshake",
	"wrench", "gauge", "map", "arrow-right", "scan-barcode", "store", "credit-card",
	"ban", "calendar-clock", "hourglass", "signature", "circle-help", "flag", "route",
	"chart-column", "package", "banknote", "milestone", "square-check-big", "minus",
}

const lucideURL = "https://cdn.jsdelivr.net/npm/lucide-static@latest/icons/%s.svg"

// Assets writes <Project>/assets/brand.css: webfonts (all wanted unicode
// subsets), the logo in three colourways, the banner, circular portraits and
// the Lucide icon set as CSS masks, all base64. The report HTML then renders
// identically offline and inside headless Chrome.
func Assets(o AssetOptions) error {
	if o.Get == nil {
		o.Get = HTTPGet
	}
	st, err := os.Stat(o.BrandDir)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("Brand image directory not found: %s", o.BrandDir)
	}
	project, err := filepath.Abs(o.Project)
	if err != nil {
		return err
	}
	outDir := filepath.Join(project, "assets")

	logo, err := loadImage(filepath.Join(o.BrandDir, o.LogoFile))
	if err != nil {
		return err
	}
	banner, err := loadImage(filepath.Join(o.BrandDir, "banner.png"))
	if err != nil {
		return err
	}
	mark := resize(logo, 256, 256)

	type asset struct{ key, uri string }
	var assets []asset
	add := func(key, uri string, err error) error {
		if err != nil {
			return err
		}
		assets = append(assets, asset{key, uri})
		return nil
	}
	uri, e := pngURI(mark)
	if err := add("logo-white", uri, e); err != nil {
		return err
	}
	uri, e = pngURI(recolour(mark, o.Ink))
	if err := add("logo-ink", uri, e); err != nil {
		return err
	}
	uri, e = pngURI(recolour(mark, o.Accent))
	if err := add("logo-accent", uri, e); err != nil {
		return err
	}
	uri, e = jpgURI(flatten(resize(banner, 1200, 300)), 88)
	if err := add("banner", uri, e); err != nil {
		return err
	}
	for _, person := range o.People {
		matches, _ := filepath.Glob(filepath.Join(o.BrandDir, person+".*"))
		sort.Strings(matches)
		if len(matches) == 0 {
			fmt.Fprintf(o.Out, "  ! portrait not found, skipping: %s\n", person)
			continue
		}
		img, err := loadImage(matches[0])
		if err != nil {
			return err
		}
		uri, e = pngURI(circularPortrait(img, 340))
		if err := add("photo-"+person, uri, e); err != nil {
			return err
		}
	}

	lines := []string{"/* Generated by `aval pdf assets`. Do not edit by hand. */", ""}
	faces := 0
	for _, fq := range fontQueries {
		cssText, err := o.Get("https://fonts.googleapis.com/css2?family="+fq.query+"&display=block", fontUA)
		if err != nil {
			return fmt.Errorf("fetching %s: %w", fq.family, err)
		}
		for _, m := range faceRe.FindAllStringSubmatch(string(cssText), -1) {
			subset, body := m[1], m[2]
			if !wantedSubsets[subset] {
				continue
			}
			src := woff2Re.FindStringSubmatch(body)
			if src == nil {
				continue
			}
			raw, err := o.Get(src[1], "")
			if err != nil {
				return fmt.Errorf("fetching %s %s: %w", fq.family, subset, err)
			}
			weight, style := cssField(body, "font-weight"), cssField(body, "font-style")
			if weight == "" {
				weight = "400"
			}
			if style == "" {
				style = "normal"
			}
			lines = append(lines,
				fmt.Sprintf("/* %s · %s */", fq.family, subset),
				"@font-face {",
				fmt.Sprintf("  font-family: '%s';", fq.family),
				fmt.Sprintf("  src: url(%s) format('woff2');", dataURI(raw, "font/woff2")),
				fmt.Sprintf("  font-weight: %s;", weight),
				fmt.Sprintf("  font-style: %s;", style),
				fmt.Sprintf("  unicode-range: %s;", cssField(body, "unicode-range")),
				"  font-display: block;",
				"}",
				"")
			faces++
		}
	}

	var iconVars []string
	missing := map[string]bool{}
	var missingNames []string
	for _, name := range icons {
		raw, err := o.Get(fmt.Sprintf(lucideURL, name), "")
		if err != nil {
			missing[name] = true
			missingNames = append(missingNames, name)
			continue
		}
		// A mask uses the alpha channel, so the stroke needs a solid colour.
		svg := strings.ReplaceAll(string(raw), "currentColor", "#000")
		svg = strings.TrimSpace(commentRe.ReplaceAllString(svg, ""))
		iconVars = append(iconVars, fmt.Sprintf("  --ic-%s: url(%s);", name, dataURI([]byte(svg), "image/svg+xml")))
	}

	lines = append(lines, ":root {")
	for _, a := range assets {
		lines = append(lines, fmt.Sprintf("  --img-%s: url(%s);", a.key, a.uri))
	}
	lines = append(lines, iconVars...)
	lines = append(lines, "}")

	// One class per icon, all sharing the mask mechanics.
	lines = append(lines, "", ".ico {",
		"  display: inline-block;",
		"  width: 1em; height: 1em;",
		"  background-color: currentColor;",
		"  -webkit-mask-size: contain; mask-size: contain;",
		"  -webkit-mask-repeat: no-repeat; mask-repeat: no-repeat;",
		"  -webkit-mask-position: center; mask-position: center;",
		"  vertical-align: -0.135em;",
		"  flex: 0 0 auto;",
		"}", "")
	for _, name := range icons {
		if !missing[name] {
			lines = append(lines, fmt.Sprintf(".ico-%s { -webkit-mask-image: var(--ic-%s); mask-image: var(--ic-%s); }", name, name, name))
		}
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(outDir, "brand.css")
	if err := os.WriteFile(target, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return err
	}
	fi, _ := os.Stat(target)
	fmt.Fprintf(o.Out, "%s: %.0f KB · %d images · %d font faces · %d/%d icons\n",
		target, float64(fi.Size())/1024, len(assets), faces, len(icons)-len(missingNames), len(icons))
	if len(missingNames) > 0 {
		fmt.Fprintln(o.Out, "MISSING ICONS:", strings.Join(missingNames, ", "))
	}
	return nil
}

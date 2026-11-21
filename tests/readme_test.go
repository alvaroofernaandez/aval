package tests

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// The README and the images it is built from. Two things can go wrong with a
// public README and neither shows in a diff review: it leaks something (a token
// in an example, a home path, an email), or its images drift from the generator
// so nobody can rebuild them. The generator (tools/brand) runs into a scratch
// dir and both are checked.
//
// Already unit-tested in tools/brand and not repeated here: the committed
// images are byte-identical to the generator's output, the tools with no
// published logo are exactly ripgrep and delta, and stale versions are pruned.

var imageNames = []string{"hero-light", "hero-dark", "stack-light", "stack-dark", "mark-light", "mark-dark", "wordmark-light", "wordmark-dark", "desktop-light", "desktop-dark"}

var (
	brandOnce sync.Once
	brandDir  string
	brandErr  string
)

// generated runs the generator once and returns the scratch directory.
func generated(t *testing.T) string {
	t.Helper()
	needTool(t, "go")
	brandOnce.Do(func() {
		brandDir, _ = os.MkdirTemp("", "aval-brand-")
		r := run(repo, nil, "", "go", "run", "./tools/brand", "--docs", p("docs"), "--out", brandDir)
		if r.Code != 0 {
			brandErr = r.Out()
		}
	})
	if brandErr != "" {
		t.Fatalf("tools/brand failed:\n%s", brandErr)
	}
	return brandDir
}

// asset is the one content-addressed file for name in dir ("" when absent).
func asset(dir, name string) string {
	m, _ := filepath.Glob(filepath.Join(dir, name+".*.svg"))
	sort.Strings(m)
	if len(m) == 0 {
		return ""
	}
	return m[0]
}

func svg(t *testing.T, dir, name string) string {
	t.Helper()
	f := asset(dir, name)
	if f == "" {
		t.Fatalf("no %s.*.svg in %s", name, dir)
	}
	return readAbs(t, f)
}

func tools(t *testing.T) []string {
	t.Helper()
	r := run(repo, nil, "", "go", "run", "./tools/brand", "--docs", p("docs"), "--list")
	var out []string
	for _, l := range strings.Split(r.Stdout, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// Images are content-addressed: <name>.<sha256[:8]>.svg. The hash is in the
// file name, not a query string, because github.com/<repo>/raw/... 302-redirects
// and DROPS the query, so an updated image kept showing the old one.
func TestReadmeGeneratorEmitsContentAddressedImages(t *testing.T) {
	out := generated(t)
	for _, n := range imageNames {
		f := asset(out, n)
		st, err := os.Stat(f)
		if f == "" || err != nil || st.Size() == 0 {
			t.Errorf("generator emits %s: missing or empty", n)
			continue
		}
		sum := sha256.Sum256([]byte(readAbs(t, f)))
		eq(t, n+" is named by its content hash", n+"."+hex.EncodeToString(sum[:])[:8]+".svg", filepath.Base(f))
	}
}

func TestReadmeHeroAndMark(t *testing.T) {
	out := generated(t)
	// The brand is drawn, not typeset: a wordmark set in a font renders in
	// whatever fallback the viewer has, so the logo would change per machine.
	eq(t, "wordmark is drawn with paths, not text", 0, count(`<text`, svg(t, out, "wordmark-light")))
	hero := svg(t, out, "hero-light")
	mustHave(t, `aria-label="aval`, hero, "hero carries the aval wordmark")
	// The hero draws the isotype as a matrix of tiny checks; inked cells trace it.
	mustHave(t, `data-part="matrix"`, hero, "hero builds the mark from a check matrix")
	inked := count(`class="ck on"`, hero)
	isTrue(t, "the matrix inks enough cells to trace the mark (>= 25, is "+strconv.Itoa(inked)+")", inked >= 25)
	// Presented as a brand, not as a document: a claim, no labelled fields.
	// The visible text, not just the aria-label that repeats it.
	mustHave(t, `>Nothing without proof\.<`, hero, "hero carries the brand claim")
	eq(t, "hero is in English", 0, count(`[áéíóúñ]|Nada sin|HARNESS PERSONAL`, hero))
	eq(t, "hero has no document labels", 0, count(`DEFINICIÓN|TESIS|>PARA<|NOMBRE MASCULINO`, hero))
	eq(t, "hero drops the old guilloché and suminagashi", 0, count(`(?i)guilloche|suminagashi|id="vig"`, hero))
	eq(t, "hero no longer says Dotfiles", 0, count(`(?i)dotfiles`, hero))

	// Legible as a 16px favicon: the stroke must be at least 1/8 of the canvas.
	mark := svg(t, out, "mark-light")
	w, err := strconv.ParseFloat(regexpFind(`viewBox="0 0 ([\d.]+)`, mark), 64)
	if err != nil {
		t.Fatalf("mark-light has no viewBox: %v", err)
	}
	widths := regexp.MustCompile(`stroke-width="([\d.]+)"`).FindAllStringSubmatch(mark, -1)
	isTrue(t, "the mark declares stroke widths", len(widths) > 0)
	for _, m := range widths {
		v, _ := strconv.ParseFloat(m[1], 64)
		if v < w/8 {
			t.Errorf("mark stroke %v is under 1/8 of the %v canvas (unreadable at 16px)", v, w)
		}
	}
}

// The hero is monochrome: every colour literal must be a pure grey (r=g=b). The
// logo wall is not: it shows each tool's real, coloured mark.
func TestReadmeHeroIsMonochrome(t *testing.T) {
	out := generated(t)
	re := regexp.MustCompile(`#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b`)
	bad := map[string]bool{}
	for _, prefix := range []string{"hero-", "mark-", "wordmark-", "desktop-"} {
		for _, theme := range []string{"light", "dark"} {
			for _, c := range re.FindAllString(svg(t, out, prefix+theme), -1) {
				h := strings.ToLower(c[1:])
				var r, g, b string
				if len(h) == 3 {
					r, g, b = h[0:1], h[1:2], h[2:3]
				} else {
					r, g, b = h[0:2], h[2:4], h[4:6]
				}
				if r != g || g != b {
					bad[c] = true
				}
			}
		}
	}
	var list []string
	for c := range bad {
		list = append(list, c)
	}
	sort.Strings(list)
	eq(t, "non-grey colours in the hero, mark and wordmark", "", strings.Join(list, " "))
}

// Real logos, embedded, for every tool that publishes one. Embedded because a
// README image cannot load external files: an <image href="docs/..."> inside an
// SVG renders as a blank square on GitHub.
func TestReadmeStackEmbedsRealLogos(t *testing.T) {
	out := generated(t)
	stack := svg(t, out, "stack-light")
	embedded := count(`<image `, stack)
	eq(t, "every other tool embeds its logo (tools minus ripgrep and delta)", len(tools(t))-2, embedded)
	eq(t, "logos are not loaded from outside the SVG", 0, count(`href="(https?:|\.\.?/|docs/)`, stack))
	size := 0
	for _, m := range regexp.MustCompile(`class="logo" width="([0-9]+)"`).FindAllStringSubmatch(stack, -1) {
		if v, _ := strconv.Atoi(m[1]); size == 0 || v < size {
			size = v
		}
	}
	isTrue(t, "logos render at least 56px (smallest is "+strconv.Itoa(size)+")", size >= 56)
}

// Cards are a neutral grey (r=g=b) in each theme, and the logos are adapted to
// them: black marks are lightened on the dark card, app icons are clipped.
func TestReadmeStackCardsAndAdaptedLogos(t *testing.T) {
	out := generated(t)
	for _, theme := range []string{"light", "dark"} {
		var tint []string
		for _, m := range regexp.MustCompile(`class="card"[^>]*fill="#([0-9a-f]{6})"`).FindAllStringSubmatch(svg(t, out, "stack-"+theme), -1) {
			if h := m[1]; h[0:2] != h[2:4] || h[2:4] != h[4:6] {
				tint = append(tint, h)
			}
		}
		eq(t, "stack-"+theme+" cards that are not neutral grey", "", strings.Join(tint, " "))
	}
	eq(t, "dark stack adapts the six logos that vanish on dark grey", 6, count(`data-adapted="dark"`, svg(t, out, "stack-dark")))
	eq(t, "light stack keeps every logo original", 0, count(`data-adapted=`, svg(t, out, "stack-light")))
	isTrue(t, "app-icon logos are clipped to a rounded tile (>= 7)", count(`clip-path="url\(#tile`, svg(t, out, "stack-light")) >= 7)
}

// Every vendored logo says where it came from; one labelled tile per tool so
// the image is not a mystery to a screen reader.
func TestReadmeLogosAreSourcedAndTilesLabelled(t *testing.T) {
	sources := read(t, "docs/logos/SOURCES.md")
	files, _ := filepath.Glob(p("docs", "logos", "*.*"))
	var unsourced []string
	for _, f := range files {
		b := filepath.Base(f)
		if b != "SOURCES.md" && !strings.Contains(sources, "`"+b+"`") {
			unsourced = append(unsourced, b)
		}
	}
	eq(t, "logos without a source", "", strings.Join(unsourced, " "))

	list := tools(t)
	isTrue(t, "generator lists at least 20 tools (is "+strconv.Itoa(len(list))+")", len(list) >= 20)
	stack := svg(t, generated(t), "stack-light")
	var missing []string
	for _, tool := range list {
		if !strings.Contains(stack, ">"+tool+"<") {
			missing = append(missing, tool)
		}
	}
	eq(t, "tools without a labelled tile", "", strings.Join(missing, ", "))
}

func TestReadmeShowsTheGeneratedImages(t *testing.T) {
	readme := read(t, "README.md")
	for _, theme := range []string{"light", "dark"} {
		mustHave(t, `docs/assets/hero-`+theme+`\.[0-9a-f]{8}\.svg`, readme, "README shows hero-"+theme)
		mustHave(t, `docs/assets/stack-`+theme+`\.[0-9a-f]{8}\.svg`, readme, "README shows stack-"+theme)
	}
	mustContain(t, readme, "prefers-color-scheme: dark", "README switches images with the GitHub theme")
	mustContain(t, readme, "(docs/guide.md", "README links the full guide")
	st, err := os.Stat(p("docs/guide.md"))
	isTrue(t, "the full guide exists", err == nil && st.Size() > 0)

	// The brand: the README introduces aval, says what the word means, and the
	// old installer name is gone everywhere a reader would copy a command from.
	isTrue(t, "README shows the aval mark",
		has(`docs/assets/mark-dark\.[0-9a-f]{8}\.svg`, readme) && has(`docs/assets/mark-light\.[0-9a-f]{8}\.svg`, readme))
	mustHave(t, `(?i)guarantee`, readme, "README explains what aval means")
	eq(t, "README is in English", 0, count(`[áéíóúñ¿¡]`, readme))
	isTrue(t, "README shows the desktop app logo",
		has(`docs/assets/desktop-dark\.[0-9a-f]{8}\.svg`, readme) && has(`docs/assets/desktop-light\.[0-9a-f]{8}\.svg`, readme))
	for _, doc := range []string{"README.md", "docs/guide.md", "npm/cli/README.md"} {
		eq(t, doc+": no old installer name", 0, count(`dotfiles-installer|\.dotfiles-backup|DOTFILES_`, read(t, doc)))
	}
}

// Cache busting: every generated image the README shows is the current
// content-addressed file, and no stale or query-versioned URL remains.
func TestReadmeCacheBusting(t *testing.T) {
	readme := read(t, "README.md")
	assets := p("docs", "assets")
	var stale, extra []string
	for _, n := range imageNames {
		if !has(`docs/assets/`+n+`\.[0-9a-f]{8}\.svg`, readme) {
			continue
		}
		cur := filepath.Base(asset(assets, n))
		if !strings.Contains(readme, "docs/assets/"+cur) {
			stale = append(stale, n)
		}
	}
	eq(t, "README images that point at a stale version", "", strings.Join(stale, " "))
	eq(t, "README uses no query-string versions", 0, count(`docs/assets/[^"]*\?v=`, readme))
	for _, n := range imageNames {
		if m, _ := filepath.Glob(filepath.Join(assets, n+".*.svg")); len(m) > 1 || exists(filepath.Join(assets, n+".svg")) {
			extra = append(extra, n)
		}
	}
	eq(t, "stale image versions committed", "", strings.Join(extra, " "))
}

func TestReadmeHasNoBrokenRelativeLinks(t *testing.T) {
	readme := read(t, "README.md")
	var broken []string
	for _, m := range regexp.MustCompile(`(?:\]\(|src="|srcset=")([^")#][^")]*)`).FindAllStringSubmatch(readme, -1) {
		ref := m[1]
		if strings.HasPrefix(ref, "http:") || strings.HasPrefix(ref, "https:") || strings.HasPrefix(ref, "mailto:") {
			continue
		}
		path := strings.SplitN(strings.SplitN(ref, "#", 2)[0], "?", 2)[0]
		if path != "" && !exists(p(path)) {
			broken = append(broken, path)
		}
	}
	eq(t, "relative links and images that do not resolve", "", strings.Join(broken, " "))
}

func TestReadmeLeaks(t *testing.T) {
	for _, doc := range []string{"README.md", "docs/guide.md"} {
		if !exists(p(doc)) {
			continue
		}
		t.Run(doc, func(t *testing.T) {
			text := read(t, doc)
			var personal []string
			for _, re := range []string{`/Users/[a-z]`, `/home/[a-z]`, `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}`,
				`\b[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\b`} {
				personal = append(personal, regexp.MustCompile(re).FindAllString(text, -1)...)
			}
			eq(t, doc+": home paths, emails or IPs", "", strings.Join(personal, " "))
			needTool(t, "gitleaks")
			r := run(repo, nil, "", "gitleaks", "detect", "--no-git", "--no-banner", "--source", p(doc))
			eq(t, doc+": gitleaks exit (0 = nothing found)\n"+r.Out(), 0, r.Code)
		})
	}
}

// Badges: only the three platforms, each with a minimal monochrome icon.
func TestReadmeBadges(t *testing.T) {
	readme := read(t, "README.md")
	badges := regexp.MustCompile(`img\.shields\.io/badge/[^)]*`).FindAllString(readme, -1)
	var names []string
	white := 0
	for _, b := range badges {
		if m := regexp.MustCompile(`badge/-?([A-Za-z]+)-`).FindStringSubmatch(b); m != nil {
			names = append(names, m[1])
		}
		if has(`logo=[^&]+.*logoColor=white`, b) {
			white++
		}
	}
	eq(t, "badges are exactly macOS, Linux and Windows", "macOS Linux Windows", strings.Join(names, " "))
	eq(t, "every badge carries a white icon", 3, white)
}

// The stack image explains the roadmap: each lane shows what runs today and
// the native aval piece that replaces it, with the version that ships it; the
// tools aval keeps driving are shown apart, so nobody reads them as going away.
func TestReadmeStackShowsTheRoadmap(t *testing.T) {
	stack := svg(t, generated(t), "stack-light")
	lanes := count(`data-part="lane"`, stack)
	isTrue(t, "the stack has at least 4 replacement lanes (is "+strconv.Itoa(lanes)+")", lanes >= 4)
	eq(t, "every lane names its aval replacement", lanes, count(`data-target="aval[^"]*"`, stack))
	eq(t, "every lane names the version that ships it", lanes, count(`data-version="v[0-9]+\.[0-9]+\.[0-9]+`, stack))
	mustHave(t, `data-part="kept"`, stack, "the tools aval keeps are a separate section")
	mustHave(t, `data-target="aval desktop"`, stack, "the terminal window lane ends in the desktop app")
	eq(t, "stack is in English", 0, count(`[áéíóúñ]|AGENTES|PLATAFORMAS|Herramientas`, stack))
}

package deck

import (
	"embed"
	"encoding/base64"
	"fmt"
	"strings"
)

// fontFS holds the brand fonts and their licences: the variable fonts exactly
// as google/fonts ships them (see assets/fonts/SOURCES.md), never subset or
// converted, because IBM Plex reserves its name for unmodified files.
//
//go:embed assets/fonts/*.ttf assets/fonts/OFL-*.txt
var fontFS embed.FS

// brandFonts maps each family to its file and the weight range its wght axis
// covers.
var brandFonts = []struct {
	family, file string
	min, max     int
}{
	{"Space Grotesk", "assets/fonts/SpaceGrotesk-wght.ttf", 300, 700},
	{"IBM Plex Sans", "assets/fonts/IBMPlexSans-wdth-wght.ttf", 100, 700},
}

// fontFaces returns the @font-face rules that inline the brand fonts, so a
// deck renders the same with no network.
func fontFaces() string {
	var b strings.Builder
	for _, f := range brandFonts {
		data, err := fontFS.ReadFile(f.file)
		if err != nil {
			continue // the embed pattern guarantees the file; fall back to system fonts
		}
		fmt.Fprintf(&b, "@font-face { font-family: %q; font-style: normal; font-weight: %d %d; src: url(data:font/ttf;base64,%s) format(\"truetype\"); }\n",
			f.family, f.min, f.max, base64.StdEncoding.EncodeToString(data))
	}
	return b.String()
}

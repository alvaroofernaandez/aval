package ui

import (
	"io"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alvaroofernaandez/aval/internal/design/gate"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// approvedWordmark is the owner-approved art, copied from DESIGN.md section
// 10, "### Wordmark". The generator must reproduce it exactly.
var approvedWordmark = []string{
	"                     ▗██              ██",
	"                    ▗██▘              ██",
	"   ▄▄▄▄▄▄▄▖  ▗▄▖    ▟█▘    ▗▄▄▄▄▄▄▄   ██",
	" ▗██▀▀▀▜██▌  ▝██▖  ▟█▛    ▟█▛▀▀▀███   ██",
	" ██▘    ▜█▌   ▝██▖▟█▛    ▐█▛    ▝██   ██",
	" ▜█▙▖  ▄██▌    ▝███▛     ▝██▄  ▗▟██   ██",
	"  ▀▜████▜█▌     ▝█▛       ▝▀████▛██   ██",
}

// approvedWordmarkASCII is the ASCII tier from DESIGN.md section 10: each
// cell of the same quadrant raster, mapped by how many quarters are filled.
var approvedWordmarkASCII = []string{
	"                     .##              ##",
	"                    .##'              ##",
	"   ,,,,,,,.  .,.    ##'    .,,,,,,,   ##",
	" .##\"\"\"####  '##.  ###    ###\"\"\"###   ##",
	" ##'    ###   '##.###    ###    '##   ##",
	" ###.  ,###    '####     '##,  .###   ##",
	"  \"########     '##       '\"#######   ##",
}

func TestWordmarkASCIIMatchesApprovedArt(t *testing.T) {
	got := strings.Join(wordmarkASCII, "\n")
	want := strings.Join(approvedWordmarkASCII, "\n")
	if got != want {
		t.Fatalf("ascii wordmark differs from DESIGN.md:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWordmarkUnicodeMatchesApprovedArt(t *testing.T) {
	got := strings.Join(wordmarkUnicode, "\n")
	want := strings.Join(approvedWordmark, "\n")
	if got != want {
		t.Fatalf("unicode wordmark differs from DESIGN.md:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWordmarkRowsSameWidth(t *testing.T) {
	for name, tier := range map[string][]string{"unicode": wordmarkUnicode, "ascii": wordmarkASCII} {
		if len(tier) != 7 {
			t.Errorf("%s: want 7 rows, got %d", name, len(tier))
			continue
		}
		w := utf8.RuneCountInString(tier[0])
		for i, row := range tier {
			if n := utf8.RuneCountInString(row); n != w {
				t.Errorf("%s row %d: width %d, want %d", name, i, n, w)
			}
		}
	}
}

func TestWordmarkASCIIIsPureASCII(t *testing.T) {
	if len(wordmarkASCII) == 0 {
		t.Fatal("ascii tier is empty")
	}
	for i, row := range wordmarkASCII {
		for _, r := range row {
			if !strings.ContainsRune(" #\"',.", r) {
				t.Errorf("ascii row %d has %q; only space, #, \", ', and . are allowed", i, r)
			}
		}
	}
}

// Column spans of the letters in the 40-column art.
var wordmarkSpans = struct{ a1, check, a2, l [2]int }{
	a1: [2]int{0, 11}, check: [2]int{11, 25}, a2: [2]int{25, 36}, l: [2]int{36, 40},
}

func inked(row string, span [2]int) bool {
	rs := []rune(row)
	for i := span[0]; i < span[1] && i < len(rs); i++ {
		if rs[i] != ' ' {
			return true
		}
	}
	return false
}

// The x-height rhythm: the check's arm and the l are ascenders and reach the
// top row; the a bowls sit at x-height and stay clear of the top two rows.
func TestWordmarkRhythm(t *testing.T) {
	for name, tier := range map[string][]string{"unicode": wordmarkUnicode, "ascii": wordmarkASCII} {
		if len(tier) < 3 {
			t.Errorf("%s: too few rows: %d", name, len(tier))
			continue
		}
		if !inked(tier[0], wordmarkSpans.check) || !inked(tier[0], wordmarkSpans.l) {
			t.Errorf("%s: the check and the l must reach row 1: %q", name, tier[0])
		}
		for r := 0; r < 2; r++ {
			if inked(tier[r], wordmarkSpans.a1) || inked(tier[r], wordmarkSpans.a2) {
				t.Errorf("%s: an a bowl reaches row %d: %q", name, r+1, tier[r])
			}
		}
		if !inked(tier[2], wordmarkSpans.a1) || !inked(tier[2], wordmarkSpans.a2) {
			t.Errorf("%s: the bowls must start at row 3: %q", name, tier[2])
		}
	}
}

func TestWordmarkEmojiGate(t *testing.T) {
	for _, f := range []string{"wordmark_gen.go", "wordmark.go"} {
		if _, err := os.Stat(f); err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		if code := gate.Emoji(&out, []string{f}); code != 0 {
			t.Errorf("emoji gate failed on %s:\n%s", f, out.String())
		}
	}
	_ = io.Discard
}

func TestWordmarkTier(t *testing.T) {
	uni := wordmark(caps.Caps{Unicode: true}, 80, 24)
	if len(uni) == 0 || strings.Join(uni, "\n") != strings.Join(wordmarkUnicode, "\n") {
		t.Fatalf("unicode caps: got %q", uni)
	}
	asc := wordmark(caps.Caps{Unicode: false}, 80, 24)
	if strings.Join(asc, "\n") != strings.Join(wordmarkASCII, "\n") || len(asc) == 0 {
		t.Errorf("ascii caps: got %q", asc)
	}
	uni[0] = "mutated"
	if wordmarkUnicode[0] == "mutated" {
		t.Error("wordmark must return a copy")
	}
}

// DESIGN.md: from 40 columns up it shows (stacked or beside the panel);
// compact, under 40, never shows it. It also needs its seven rows.
func TestWordmarkSizeRule(t *testing.T) {
	for _, u := range []bool{true, false} {
		c := caps.Caps{Unicode: u}
		for _, sz := range [][2]int{{39, 24}, {20, 24}, {0, 0}, {80, 6}, {40, 6}} {
			if got := wordmark(c, sz[0], sz[1]); got != nil {
				t.Errorf("unicode=%v %dx%d: want nil, got %d rows", u, sz[0], sz[1], len(got))
			}
		}
		for _, sz := range [][2]int{{40, 7}, {79, 24}, {80, 24}, {200, 60}} {
			if got := wordmark(c, sz[0], sz[1]); len(got) != 7 {
				t.Errorf("unicode=%v %dx%d: want 7 rows, got %d", u, sz[0], sz[1], len(got))
			}
		}
	}
}

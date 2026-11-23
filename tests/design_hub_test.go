package tests

import (
	"strings"
	"testing"
)

// designSection10 returns section 10 of .agents/DESIGN.md: from its heading up
// to the next level-2 heading.
func designSection10(t *testing.T) string {
	t.Helper()
	doc := read(t, ".agents/DESIGN.md")
	start := strings.Index(doc, "\n## 10.")
	if start < 0 {
		t.Fatal("DESIGN.md has no section 10")
	}
	rest := doc[start+1:]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

// The hub (bare `aval` on a TTY) is a designed surface: the contract must name
// it before any hub code is written, or the design gate acks against nothing.
func TestDesignCoversTheHub(t *testing.T) {
	s := designSection10(t)
	mustHave(t, `^### Hub`, s, "section 10 has a Hub subsection")
	mustHave(t, `[Bb]are `+"`aval`", s, "section 10 says bare `aval` opens the hub")
	mustContain(t, s, "writes nothing", "section 10 says opening the hub writes nothing")
	mustHave(t, `^### Wordmark`, s, "section 10 has a Wordmark subsection")
	mustContain(t, s, "7 rows by 41 columns", "section 10 states the wordmark size")
	mustContain(t, s, "80 columns or more", "section 10 states the wordmark size rule")
	mustContain(t, s, "▜█▙▖  ▄██▌    ▝███▛     ▝██▄  ▗▟██   ██", "section 10 carries the approved wordmark art")
	mustHave(t, `^### Navigation`, s, "section 10 has a Navigation subsection")
	mustHave(t, `^### Operation pattern`, s, "section 10 has the confirmation pattern")
	mustHave(t, `Cancel`, s, "the confirmation pattern opens destructive actions on Cancel")
	mustContain(t, s, "needs a checkout and Go", "section 10 gives the dev sandbox reason")
	mustContain(t, read(t, ".agents/DESIGN.md"), "### 2026-11-23: TUI hub", "the decision log records the hub")
}

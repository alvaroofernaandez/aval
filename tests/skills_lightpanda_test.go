package tests

import "testing"

// The hybrid headless-browser policy. Lightpanda is the default headless
// browser for automation (Playwright/Puppeteer over CDP, E2E, scraping, DOM
// extraction). It has NO rendering engine, so anything that needs pixels
// (screenshots, visual regression, layout/contrast gates, PDF) stays on
// Chromium. These tests pin both halves so neither drifts into "Lightpanda for
// everything".

func TestLightpandaSkill(t *testing.T) {
	s := skill(t, "lightpanda-browser")
	for _, c := range []struct{ what, re string }{
		{"frontmatter name matches dir", `^name: lightpanda-browser$`},
		{"description is one quoted line", `^description: ".+"$`},
		{"documents CDP connection", `connectOverCDP`},
		{"disables telemetry", `LIGHTPANDA_DISABLE_TELEMETRY=true`},
		{"states there is no rendering engine", `(?i)no (graphical )?rendering engine`},
		{"keeps screenshots on Chromium", `(?i)screenshot`},
		{"keeps PDF generation on Chromium", `(?i)pdf`},
		{"never uses Lightpanda png dump as a screenshot", `dump png`},
		{"agent flows use the Lightpanda MCP server", `lightpanda mcp`},
		{"installs playwright-core without browser download", `PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1`},
		{"keeps a single Chromium headless shell", `(?i)one .*headless shell`},
		{"documents the Playwright Test cdpEndpoint fixture", `cdpEndpoint`},
		{"warns Service Workers (MSW) are unsupported", `serviceWorker`},
		{"requires a hydration probe before moving specs", `(?i)hydration probe`},
		{"warns Clerk browser sign-up fails", `(?i)clerk`},
		{"forbids broad process kills", `(?i)never .*pkill`},
	} {
		mustHave(t, c.re, s, "skill: "+c.what)
	}
	mustNotHave(t, `^license:`, s, "skill: own skills carry no license field")
}

// The rule moved to a lesson injected when a headless browser is about to run;
// the shared block keeps only a pointer to the lessons directory.
func TestLightpandaLessonAndSharedBlockPointer(t *testing.T) {
	lesson := read(t, "config/claude/lessons/headless-browser.md")
	mustHave(t, `Lightpanda is the default`, lesson, "headless-browser lesson names Lightpanda as default")
	mustHave(t, `Chromium`, lesson, "headless-browser lesson keeps Chromium for pixels")
	mustHave(t, `lightpanda-browser`, lesson, "headless-browser lesson points to the skill")
	blk := sharedBlock(read(t, "config/claude/CLAUDE.md"))
	isTrue(t, "CLAUDE.md: shared block points to the lessons", has(`headless browser`, blk) && has(`lessons`, blk))
	mustHave(t, `headless browser`, sharedBlock(read(t, "config/opencode/AGENTS.md")), "AGENTS.md mirrors the pointer")
}

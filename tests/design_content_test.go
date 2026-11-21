package tests

import (
	"github.com/alvaroofernaandez/aval/internal/testenv"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The design pipeline as shipped: settings wiring, the skills the gate sends
// the model to, the always-on text budget and the render script.

func TestDesignPipelineBashGateAndDesignAckAreGone(t *testing.T) {
	isTrue(t, "config/claude/hooks/design-pipeline-gate.sh is gone", !exists(p("config/claude/hooks/design-pipeline-gate.sh")))
	isTrue(t, "the design-ack script is gone", !exists(p("skills/design-pipeline/scripts/design-ack")))
}

// The hook is inert unless settings.json calls it.
func TestDesignPipelineSettingsWiring(t *testing.T) {
	s := repoSettings(t)
	var matcher string
	for _, g := range s.Hooks["PreToolUse"] {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "hook design-gate") {
				matcher = g.Matcher
			}
		}
	}
	isTrue(t, "settings.json wires the gate as a PreToolUse hook", matcher != "" || hasHook(s, "PreToolUse", "hook design-gate"))
	isTrue(t, "the gate matches Edit and Write", strings.Contains(matcher, "Edit") && strings.Contains(matcher, "Write"))
	mustNotHave(t, `design-ack`, read(t, "install.manifest"), "install.manifest links a design-ack script")
}

func hasHook(s settingsDoc, event, substr string) bool {
	for _, g := range s.Hooks[event] {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, substr) {
				return true
			}
		}
	}
	return false
}

// bencium-innovative-ux-designer ships a description that is a word-for-word
// subset of frontend-design's: two skills competing for one pipeline step make
// the step ambiguous by construction.
func TestDesignPipelineCollidingPluginIsNotEnabled(t *testing.T) {
	v := repoSettings(t).EnabledPlugins["bencium-innovative-ux-designer@bencium-marketplace"]
	isTrue(t, "the bencium UX plugin is not enabled", v != true)
}

// The gate can demand the line; only the skill makes the line mean something.
func TestDesignPipelineLawsOfUXSkillCarriesAll30Laws(t *testing.T) {
	skill := read(t, "skills/laws-of-ux/SKILL.md")
	mustHave(t, `^name: laws-of-ux$`, skill, "the skill declares frontmatter name")
	mustHave(t, `^description:`, skill, "the skill declares frontmatter description")
	// A skill that quietly drops half of them drops the unglamorous ones (Postel,
	// Tesler, Parkinson): precisely the ones nobody applies from memory.
	laws := []string{"Aesthetic-Usability Effect", "Choice Overload", "Chunking", "Cognitive Bias", "Cognitive Load",
		"Doherty Threshold", "Fitts's Law", "Flow", "Goal-Gradient Effect", "Hick's Law", "Jakob's Law",
		"Law of Common Region", "Law of Proximity", "Law of Prägnanz", "Law of Similarity",
		"Law of Uniform Connectedness", "Mental Model", "Miller's Law", "Occam's Razor",
		"Paradox of the Active User", "Pareto Principle", "Parkinson's Law", "Peak-End Rule", "Postel's Law",
		"Selective Attention", "Serial Position Effect", "Tesler's Law", "Von Restorff Effect", "Working Memory",
		"Zeigarnik Effect"}
	eq(t, "the table lists 30 laws", 30, len(laws))
	var missing []string
	for _, l := range laws {
		if !strings.Contains(skill, l) {
			missing = append(missing, l)
		}
	}
	eq(t, "laws missing from the skill", "", strings.Join(missing, ", "))
}

// Measured: ~2.4K tokens of always-on design text per turn, most of it a
// checklist the marker gate no longer reads. The always-on part stays small;
// detail (the DESIGN.md schema) lives in references/.
func TestDesignPipelineTextDiet(t *testing.T) {
	skill := read(t, "skills/design-pipeline/SKILL.md")
	isTrue(t, "design-pipeline SKILL.md stays under 2500 chars (is "+strconv.Itoa(len(skill))+")", len(skill) < 2500)
	isTrue(t, "the skill names the three steps",
		strings.Contains(skill, "laws-of-ux") && strings.Contains(skill, "direction") && strings.Contains(skill, "gates"))
	mustContain(t, skill, "aval design ack <surface>", "the skill tells the model to run design-ack")
	mustHave(t, `shot\.mjs`, skill, "the skill points at shot.mjs for render evidence")
	isTrue(t, "the skill says .design/ belongs in .gitignore", strings.Contains(skill, ".design/") && strings.Contains(skill, "gitignore"))
	schema := read(t, "skills/design-pipeline/references/design-md-schema.md")
	isTrue(t, "the DESIGN.md schema moved to references/",
		strings.Contains(schema, "Intent & North Star") && strings.Contains(schema, "Decision Log"))
	mustHave(t, `references/design-md-schema\.md`, skill, "the skill links the schema")
}

// descLen is the length of the (possibly folded) frontmatter description.
func descLen(text string) int {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return 0
	}
	var parts []string
	in := false
	for _, l := range lines[1:] {
		if l == "---" {
			break
		}
		if strings.HasPrefix(l, "description:") {
			in = true
			l = regexp.MustCompile(`^description:[ >]*`).ReplaceAllString(l, "")
		} else if in && regexp.MustCompile(`^[a-z-]+:`).MatchString(l) {
			break
		}
		if in {
			parts = append(parts, l)
		}
	}
	return len(strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(strings.Join(parts, " "), " ")))
}

// Skills this repo owns keep their trigger words in <= 220 chars.
func TestDesignPipelineOwnSkillDescriptionsStayShort(t *testing.T) {
	for _, sk := range []string{"laws-of-ux", "design-refactor", "design-pipeline"} {
		n := descLen(read(t, "skills/"+sk+"/SKILL.md"))
		isTrue(t, sk+" description stays under 220 chars (is "+strconv.Itoa(n)+")", n > 0 && n <= 220)
	}
}

func TestDesignPipelineAlwaysOnSection(t *testing.T) {
	claude := read(t, "config/claude/CLAUDE.md")
	var sec []string
	in := false
	for _, l := range strings.Split(claude, "\n") {
		if strings.HasPrefix(l, "## Design Architecture") {
			in = true
		} else if in && strings.HasPrefix(l, "## ") {
			break
		}
		if in {
			sec = append(sec, l)
		}
	}
	section := strings.TrimRight(strings.Join(sec, "\n"), "\n")
	isTrue(t, "the Design Architecture section stays under 850 chars (is "+strconv.Itoa(len(section))+")", len(section) > 0 && len(section) <= 850)
	for _, needle := range []string{"DESIGN.md", "design-pipeline", "aval design ack <surface>", "DESIGN_PIPELINE_OFF", "aval design gate"} {
		mustContain(t, section, needle, "the section still says")
	}

	// The path-scoped half moved to lessons (aval hook lessons), which inject it
	// when a UI file is touched instead of on every turn.
	for _, pair := range [][2]string{
		{"ui-visual-verification", "shot.mjs"}, {"ui-visual-verification", "not run"},
		{"ui-native-elements", "aval design gate native"}, {"ui-native-elements", "components/ui"},
		{"ui-native-elements", "shadcn/ui"},
	} {
		mustContain(t, read(t, "config/claude/lessons/"+pair[0]+".md"), pair[1], "lesson "+pair[0]+" still says")
	}

	// CLAUDE.md must still point at the skill, or the move silently drops the
	// rule for any session that never thinks to look for it.
	mustHave(t, "design-pipeline. skill", claude, "CLAUDE.md still routes UI work to the design-pipeline skill")
	mustHave(t, `DESIGN_PIPELINE_OFF`, claude, "CLAUDE.md still states the gate is blocking")
}

// shot.mjs: verification by rendering. About one user message in five was a
// screenshot saying a "fixed" visual bug was not fixed. shot.mjs renders in
// Chromium at 390/768/1440, writes screenshots into .design/shots/, runs the
// overflow audit and never passes silently when it cannot render.
func TestDesignPipelineShotMjs(t *testing.T) {
	testenv.SkipWindows(t, "drives node and Playwright Chromium; covered on Linux and macOS")
	shot := p("skills/design-pipeline/scripts/shot.mjs")
	isTrue(t, "shot.mjs exists", exists(shot))
	needTool(t, "node", "git")
	env := isoEnv(t.TempDir())

	r := run(repo, env, "", "node", shot)
	isTrue(t, "shot.mjs without a target fails with usage", r.Code != 0 && has(`(?i)usage`, r.Out()))

	r = run(repo, withEnv(env, "SHOT_PLAYWRIGHT=playwright-that-is-not-installed"), "", "node", shot, "/nonexistent.html")
	isTrue(t, "a missing Playwright is a failure, never a silent pass", r.Code != 0 && has(`(?i)playwright`, r.Out()))

	slow(t)
	sp := t.TempDir()
	git(t, sp, "init", "-q")
	write(t, sp+"/clean.html", `<!doctype html><meta name=viewport content="width=device-width"><h1>ok</h1><p>short</p>`)
	write(t, sp+"/wide.html", `<!doctype html><meta name=viewport content="width=device-width"><div style="width:900px;height:20px;background:#ccc">wide</div>`)

	r = run(sp, env, "", "node", shot, "clean.html")
	if has(`(?i)not installed|executable doesn|not found`, r.Out()) {
		t.Skip("Playwright Chromium unavailable here; render assertions skipped")
	}
	eq(t, "a clean page exits 0\n"+r.Out(), 0, r.Code)
	for _, w := range []string{"390", "768", "1440"} {
		m, _ := filepath.Glob(filepath.Join(sp, ".design/shots", "*"+w+".png"))
		eq(t, "it writes a "+w+"px screenshot into .design/shots", 1, len(m))
	}
	mustHave(t, `\.design/shots/.*390\.png`, r.Out(), "it prints the screenshot paths")
	mustHave(t, `scrollWidth=[0-9]+`, r.Out(), "it prints measured numbers (scrollWidth)")

	// Playwright's downloaded Chromium drifts per project version; an installed
	// Chromium browser (Chrome, Brave, Edge, Chromium) must be enough to render.
	if isDir("/Applications/Brave Browser.app") || isDir("/Applications/Google Chrome.app") {
		_ = os.RemoveAll(sp + "/.design")
		r2 := run(sp, withEnv(env, "PLAYWRIGHT_BROWSERS_PATH=/nonexistent"), "", "node", shot, "clean.html")
		eq(t, "it renders with an installed browser when Playwright's Chromium is missing\n"+r2.Out(), 0, r2.Code)
	}

	r = run(sp, env, "", "node", shot, "wide.html")
	eq(t, "a page that scrolls sideways at 390px fails", 1, r.Code)
	isTrue(t, "...and says so with the number", has(`(?i)overflow.*390|390.*overflow`, r.Out()))
}

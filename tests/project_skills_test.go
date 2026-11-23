package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The project skills in .claude/skills/ are the standard for working on aval
// (AGENTS.md: "the project skill wins"). A skill an agent cannot load (broken
// frontmatter) or one that cites a test or ADR that does not exist teaches
// rules no mechanism backs, so both are checked here.

var requiredProjectSkills = []string{"aval-architecture", "aval-testing", "aval-docs", "aval-release"}

// projectSkillFrontmatter returns the name and description of a SKILL.md, and
// whether the file opens with a closed `---` block. A folded (`>`) or literal
// (`|`) description is joined from its indented lines.
func projectSkillFrontmatter(text string) (name, desc string, ok bool) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", "", false
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return "", "", false
	}
	lines := strings.Split(text[4:4+end], "\n")
	for i := 0; i < len(lines); i++ {
		k, v, found := strings.Cut(lines[i], ":")
		if !found || strings.HasPrefix(k, " ") {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "name":
			name = strings.Trim(v, `"'`)
		case "description":
			if v == ">" || v == "|" || v == ">-" || v == "|-" {
				var parts []string
				for i+1 < len(lines) && strings.HasPrefix(lines[i+1], " ") {
					i++
					parts = append(parts, strings.TrimSpace(lines[i]))
				}
				v = strings.Join(parts, " ")
			}
			desc = strings.Trim(v, `"'`)
		}
	}
	return name, desc, true
}

// projectSkillText is SKILL.md plus every file under references/.
func projectSkillText(t *testing.T, dir string) string {
	t.Helper()
	text := read(t, filepath.Join(".claude/skills", dir, "SKILL.md"))
	refs, _ := filepath.Glob(p(".claude/skills", dir, "references", "*.md"))
	sort.Strings(refs)
	for _, r := range refs {
		text += "\n" + readAbs(t, r)
	}
	return text
}

func TestProjectSkillsExist(t *testing.T) {
	for _, s := range requiredProjectSkills {
		isTrue(t, ".claude/skills/"+s+"/SKILL.md exists", exists(p(".claude/skills", s, "SKILL.md")))
	}
	for _, f := range []string{"references/aval.md", "references/field-notes.md"} {
		isTrue(t, "aval-testing has "+f, exists(p(".claude/skills/aval-testing", f)))
	}
}

func TestProjectSkillFrontmatterIsValid(t *testing.T) {
	dirs := projectSkillDirs(t)
	isTrue(t, ".claude/skills holds the project skills", len(dirs) >= len(requiredProjectSkills))
	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			name, desc, ok := projectSkillFrontmatter(read(t, filepath.Join(".claude/skills", dir, "SKILL.md")))
			isTrue(t, dir+": SKILL.md opens with a closed --- frontmatter block", ok)
			eq(t, dir+": frontmatter name equals the directory", dir, name)
			isTrue(t, dir+": description is not empty", strings.TrimSpace(desc) != "")
			mustHave(t, `(?i)\b(use when|use before|trigger)`, desc, dir+": description says when to load it")
		})
	}
}

var (
	skillTestRef = regexp.MustCompile("(?:tests|cmd/aval|internal/[a-z0-9_/]+)/[a-z0-9_]+_test\\.go")
	skillADRRef  = regexp.MustCompile(`\bADR (\d{4})\b`)
)

// Every rule in a project skill is grounded in a mechanism: each skill names at
// least one test file or ADR, and every one it names resolves.
func TestProjectSkillNamesMechanismsThatExist(t *testing.T) {
	for _, dir := range projectSkillDirs(t) {
		t.Run(dir, func(t *testing.T) {
			text := projectSkillText(t, dir)
			tests := skillTestRef.FindAllString(text, -1)
			adrs := skillADRRef.FindAllStringSubmatch(text, -1)
			isTrue(t, dir+" names at least one test file or ADR", len(tests)+len(adrs) > 0)
			for _, f := range tests {
				isTrue(t, dir+" names "+f+", which exists", exists(p(f)))
			}
			for _, m := range adrs {
				found, _ := filepath.Glob(p("docs/adr", m[1]+"-*.md"))
				isTrue(t, dir+" names ADR "+m[1]+", which exists in docs/adr", len(found) == 1)
			}
		})
	}
}

// archsieve is optional and a report, never a gate (ADR 0001). An unpinned
// `npx archsieve` would run whatever the registry serves that day.
func TestProjectSkillArchsieveIsPinnedAndNeverAGate(t *testing.T) {
	text := read(t, ".claude/skills/aval-architecture/SKILL.md")
	pin := regexp.MustCompile(`archive/refs/tags/v\d+\.\d+\.\d+\.tar\.gz|archsieve@\d+\.\d+\.\d+`)
	calls := 0
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, "npx") && strings.Contains(l, "archsieve") {
			calls++
			isTrue(t, "archsieve npx call pins a version: "+strings.TrimSpace(l), pin.MatchString(l))
		}
	}
	isTrue(t, "aval-architecture shows the archsieve npx invocation", calls > 0)
	mustHave(t, `(?i)never a gate`, text, "aval-architecture says archsieve is never a gate")
	mustContain(t, text, "--go auto", "aval-architecture scans Go packages")
}

// The skills carry the specifics the plan asks for; these are the rules an
// agent most often gets wrong, so they are pinned.
func TestProjectSkillCarryTheirChecklists(t *testing.T) {
	arch := projectSkillText(t, "aval-architecture")
	for _, re := range []string{`archRules`, `archMaxExceptions`, `(?i)AS-IS`, `(?i)TO-BE`, `docs/architecture/model\.md`, `-update`, `ADR 0002`} {
		mustHave(t, re, arch, "aval-architecture")
	}
	testing_ := projectSkillText(t, "aval-testing")
	for _, re := range []string{`errors\.Is`, `errors\.As`, `t\.TempDir\(\)`, `-race`, `-count=`, `-p 1`, `git worktree add`,
		`(?i)skipped is not passed`, `TestLessonsHookLatency`, `TestRateGuardHookLatency`, `TestShellStartupBudget`,
		`AVAL_TEST_REPO`, `internal/testenv`, `(?i)mutation`, `(?i)S6`} {
		mustHave(t, re, testing_, "aval-testing")
	}
	docs := projectSkillText(t, "aval-docs")
	for _, re := range []string{`vhs`, `docs/hub-demo\.tape`, `~/\.aval`, `tools/brand`, `-update`, `docs/adr`} {
		mustHave(t, re, docs, "aval-docs")
	}
	rel := projectSkillText(t, "aval-release")
	for _, re := range []string{`docs/releasing\.md`, `CHANGELOG`, `go test -race \./\.\.\.`, `GORELEASER_VERSION`, `aval release`, `(?i)never push`, `(?i)authori[sz]ation`} {
		mustHave(t, re, rel, "aval-release")
	}
}

// Project skills live in this repo only; the distributed ones are in skills/.
func TestProjectSkillsAreNotDistributed(t *testing.T) {
	manifest, err := os.ReadFile(p("install.manifest"))
	if err != nil {
		t.Skip("no install.manifest")
	}
	mustNotHave(t, `\.claude/skills/aval-`, string(manifest), "install.manifest links a project skill")
}

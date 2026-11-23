package tests

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// AGENTS.md is the project's instruction file for coding agents; CLAUDE.md
// only imports it, so every agent reads the same text.

func TestAgentsMDClaudeMdOnlyImportsAgentsMd(t *testing.T) {
	eq(t, "CLAUDE.md", "@AGENTS.md", strings.TrimSpace(read(t, "CLAUDE.md")))
}

func TestAgentsMDHasItsSections(t *testing.T) {
	md := read(t, "AGENTS.md")
	for _, re := range []string{
		`(?i)project skill wins`,
		`^## Load the project skill before the task`,
		`^## Hard rules`,
		`^## Commands`,
		`^## Output contract`,
		`^## Where things live`,
		`go test \./\.\.\.`,
		`go test \./tests/ -run Architecture`,
		`go vet \./\.\.\.`,
		`gofmt -l \.`,
		"-race",
		"-count=1",
		`errors\.Is`,
		`(?i)skipped is not passed`,
		"`aval design ack",
		"docs/architecture/model\\.md",
		"docs/adr",
		"odd/tasks",
		"\\.agents/DESIGN\\.md",
	} {
		mustHave(t, re, md, "AGENTS.md")
	}
}

// pendingProjectSkills are named in the AGENTS.md table but not yet created.
// Task S3 of odd/tasks/engineering-standards.md creates them under
// .claude/skills/; this list MUST be empty once S3 lands.
var pendingProjectSkills = map[string]bool{
	"aval-architecture": true,
	"aval-testing":      true,
	"aval-docs":         true,
	"aval-release":      true,
}

// tableSkills returns the backticked skill names in the task-to-skill table.
func tableSkills(t *testing.T) map[string]bool {
	t.Helper()
	md := read(t, "AGENTS.md")
	start := strings.Index(md, "## Load the project skill before the task")
	if start < 0 {
		t.Fatal("AGENTS.md: no task-to-skill table section")
	}
	section := md[start+3:]
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}
	cell := regexp.MustCompile("(?m)^\\|[^|\\n]*\\|\\s*`([a-z0-9-]+)`")
	out := map[string]bool{}
	for _, m := range cell.FindAllStringSubmatch(section, -1) {
		out[m[1]] = true
	}
	return out
}

func projectSkillDirs(t *testing.T) []string {
	t.Helper()
	if !isDir(p(".claude/skills")) {
		return nil
	}
	return subdirs(t, ".claude/skills")
}

func TestAgentsMDListsEveryProjectSkill(t *testing.T) {
	table := tableSkills(t)
	for _, s := range []string{"aval-architecture", "aval-testing", "aval-docs", "aval-release", "plan-deck", "design-pipeline", "strict-tdd"} {
		isTrue(t, "AGENTS.md table names "+s, table[s])
	}
	for _, dir := range projectSkillDirs(t) {
		isTrue(t, "AGENTS.md table names the project skill .claude/skills/"+dir, table[dir])
	}
}

func TestAgentsMDNamesOnlySkillsThatExist(t *testing.T) {
	for s := range tableSkills(t) {
		if !strings.HasPrefix(s, "aval-") {
			isTrue(t, "distributed skill skills/"+s+" exists", isDir(p("skills", s)))
			continue
		}
		_, err := os.Stat(p(".claude/skills", s))
		isTrue(t, "project skill "+s+" exists under .claude/skills/ or is pending", err == nil || pendingProjectSkills[s])
	}
}

// A pending entry is stale once its skill exists: drop it from the list.
func TestAgentsMDPendingSkillsAreStillPending(t *testing.T) {
	for s := range pendingProjectSkills {
		isTrue(t, s+" now exists: remove it from pendingProjectSkills", !isDir(p(".claude/skills", s)))
	}
}

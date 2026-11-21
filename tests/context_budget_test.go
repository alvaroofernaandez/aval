package tests

import (
	"strings"
	"testing"
)

// The always-on context budget.
//
// Measured 2026-09-19: the global instruction set cost ~24.6K tokens on EVERY
// turn before the user typed anything. An @import is expanded in full on every
// turn whether or not the conversation touches its subject, while a skill
// costs only its description until invoked. These ceilings keep it from
// regressing: a file with no ceiling grows back (CLAUDE.md reached 454 lines
// against a documented ~200 because nothing measured it).

func TestContextBudgetNoOrchestratorImport(t *testing.T) {
	mustNotHave(t, `^@[a-z-]*orchestrator\.md`, read(t, "config/claude/CLAUDE.md"), "CLAUDE.md @imports an orchestrator")
	isTrue(t, "the retired SDD orchestrator skill is gone", !exists(p("skills/sdd-orchestrator")))
}

// Set where the file actually lands, not at an aspirational number, so it fails
// on the next unplanned growth rather than being permanently red.
func TestContextBudgetClaudeMdStaysUnderItsCeiling(t *testing.T) {
	const ceiling = 320
	lines := strings.Count(read(t, "config/claude/CLAUDE.md"), "\n")
	isTrue(t, "CLAUDE.md stays under 320 lines (is "+itoa(lines)+")", lines <= ceiling)
}

// RTK.md is small enough to leave imported; this ceiling catches a new 19KB
// file joining it.
func TestContextBudgetAlwaysOnInstructionsStayUnderTheirCeiling(t *testing.T) {
	const maxBytes = 26000
	n := len(read(t, "config/claude/CLAUDE.md"))
	if exists(p("config/claude/RTK.md")) {
		n += len(read(t, "config/claude/RTK.md"))
	}
	isTrue(t, "always-on instructions stay under 26000 B (are "+itoa(n)+" B)", n <= maxBytes)
}

package budget

import (
	"regexp"
	"strings"
	"time"
)

// guardBudget is the ceiling of `aval hook guard`: it runs before every Bash
// call, so it gets half of the generic budget.
const guardBudget = 10 * time.Millisecond

var guardCommand = regexp.MustCompile(`\bhook guard\b`)

// budgetFor is the p95 ceiling of one hook: the generic one, or the guard's
// when that is stricter.
func budgetFor(command string, args []string, global time.Duration) time.Duration {
	if guardBudget < global && guardCommand.MatchString(command+" "+strings.Join(args, " ")) {
		return guardBudget
	}
	return global
}

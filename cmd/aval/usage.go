package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"time"

	"github.com/alvaroofernaandez/aval/internal/agent/usage"
	"github.com/alvaroofernaandez/aval/internal/core/config"
)

// usageNow and usageLoc are the clock and the time zone of `aval usage`; tests
// replace them.
var (
	usageNow = time.Now
	usageLoc = time.Local
)

const usageHelp = "usage: aval usage [--by project|session|day|consumer] [--window 5h|7d] [--json] [--check] [--rate-limits <file|->]"

// runUsage is `aval usage`: tokens, cost or plan share from the local
// transcripts. It only reads them, never touches the network, and never reads
// ANTHROPIC_API_KEY: a subscription must not be billed to a key by accident.
// With --check it exits 1 when a configured budget is over.
func runUsage(args []string, c cmdIO) error {
	fs := newFlagSet("usage", io.Discard)
	by := fs.String("by", "day", "group by project, session, day or consumer")
	window := fs.String("window", "", "limit the rows to the 5h block or the 7d window")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	check := fs.Bool("check", false, "exit 1 when a configured budget is over")
	rates := fs.String("rate-limits", "", "status line JSON with the runtime's rate limits (- for stdin)")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return newUsage(usageHelp)
	}
	if !slices.Contains([]string{"project", "session", "day", "consumer"}, *by) || !slices.Contains([]string{"", "5h", "7d"}, *window) {
		return newUsage(usageHelp)
	}

	home, err := userHomeDir(c.env)
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	billing, budgets, err := usageSettings(home, c)
	if err != nil {
		return err
	}
	rl, err := usageRateLimits(*rates, c.stdin)
	if err != nil {
		return err
	}

	corpus, err := usage.Read(usage.Roots(c.env, home))
	if err != nil {
		return err
	}
	rep := usage.Build(corpus, usage.Options{
		By: usage.By(*by), Window: *window, Now: usageNow(), Loc: usageLoc,
		Billing: billing, Budgets: budgets, Rates: rl,
	})
	if *asJSON {
		err = usage.RenderJSON(c.stdout, rep)
	} else {
		err = usage.Render(c.stdout, rep)
	}
	if err != nil {
		return err
	}
	if *check && rep.Over() {
		return errChecksFailed
	}
	return nil
}

// usageSettings reads usage.billing and the budgets from aval's config.
func usageSettings(home string, c cmdIO) (string, []usage.Budget, error) {
	f, err := loadConfig(config.Path(c.env, home, runtime.GOOS), c.stderr, c.env)
	if err != nil {
		return "", nil, err
	}
	billing, _ := f.Get("usage.billing")
	budgets, err := usage.ParseBudgets(f.Prefixed("usage.budget."))
	return billing, budgets, err
}

// usageRateLimits reads the runtime's rate limits from a status line JSON
// document: a file, or stdin for "-". No flag means none reported.
func usageRateLimits(path string, stdin io.Reader) (usage.RateLimits, error) {
	switch path {
	case "":
		return usage.RateLimits{}, nil
	case "-":
		return usage.ParseRateLimits(stdin), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return usage.RateLimits{}, err
	}
	defer f.Close()
	return usage.ParseRateLimits(f), nil
}

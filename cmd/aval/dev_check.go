package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alvaroofernaandez/aval/internal/devmode/check"
	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/ui/cli"
)

// devCheckEnv is the machine `aval dev check` runs against. nil in cmdIO is
// the real one; tests pass a fake exec, PATH and clock.
type devCheckEnv struct {
	exec check.Exec
	look func(string) (string, error)
	goos string
	dir  string
	now  func() time.Time
}

func (e *devCheckEnv) withDefaults() devCheckEnv {
	var out devCheckEnv
	if e != nil {
		out = *e
	}
	if out.exec == nil {
		out.exec = check.SystemExec{}
	}
	if out.look == nil {
		out.look = exec.LookPath
	}
	if out.goos == "" {
		out.goos = runtime.GOOS
	}
	if out.now == nil {
		out.now = time.Now
	}
	return out
}

// runDevCheck is `aval dev check [--fast] [--only NAME] [--list] [--keep-going]`:
// the table of steps in internal/devmode/check, which ci.yml also runs, so a
// green here is a green there. It exits with the code of the first failing step.
func runDevCheck(args []string, repoFlag string, flags uiFlags, c cmdIO) error {
	ui, err := devCheckSettings(flags, c)
	if err != nil {
		return err
	}
	tr := i18n.New(ui.Lang)
	usage := tr.T("cli.devcheck.usage", nil)

	fs := newFlagSet("dev check", io.Discard)
	fast := fs.Bool("fast", false, "skip the slow steps (race tests, agents lint, gitleaks)")
	only := fs.String("only", "", "run one step by name")
	list := fs.Bool("list", false, "print the steps, their tools, pinned versions and operating systems")
	keepGoing := fs.Bool("keep-going", false, "run every step even after one fails")
	if err := fs.Parse(args); err != nil {
		return &usageError{msg: usage + ": " + err.Error(), err: err}
	}
	if fs.NArg() > 0 {
		return newUsage(fmt.Sprintf("%s: unexpected argument %q", usage, strings.Join(fs.Args(), " ")))
	}

	env := c.devcheck.withDefaults()
	p := newPrinterFor(c.stdout, ui)
	if *list {
		renderDevCheckList(c.stdout, check.Steps, tr)
		return nil
	}

	entries, err := check.Plan(check.Steps, check.PlanOptions{GOOS: env.goos, Fast: *fast, Only: *only})
	if err != nil {
		return err
	}
	if err := check.Preflight(entries, env.look); err != nil {
		return err
	}
	dir := env.dir
	if dir == "" {
		if dir, err = resolveRepoWith(repoFlag, c.env); err != nil {
			return err
		}
	}
	rep := check.Run(context.Background(), entries, check.Options{Dir: dir, Exec: env.exec, KeepGoing: *keepGoing, Now: env.now})
	renderDevCheckReport(c.stdout, rep, env.goos, p, tr)
	if rep.Code != 0 {
		return exitCode(rep.Code)
	}
	return nil
}

// devCheckSettings resolves the language and theme; without a home it keeps
// the language only.
func devCheckSettings(flags uiFlags, c cmdIO) (uiSettings, error) {
	home, err := userHomeDir(c.env)
	if err != nil {
		return langOnlySettings(flags, c.env)
	}
	return settingsForCommand("dev-check", flags, c.env, home, c.stderr)
}

// renderDevCheckList prints the table the run would follow: tool, pinned
// version, the systems it applies to and the sets it belongs to.
func renderDevCheckList(w io.Writer, steps []check.Step, tr i18n.Translator) {
	rows := [][]string{{
		tr.T("cli.devcheck.col.step", nil), tr.T("cli.devcheck.col.tool", nil), tr.T("cli.devcheck.col.pin", nil),
		tr.T("cli.devcheck.col.os", nil), tr.T("cli.devcheck.col.set", nil), tr.T("cli.devcheck.col.command", nil),
	}}
	for _, s := range steps {
		os := tr.T("cli.devcheck.os.all", nil)
		switch {
		case len(s.OnlyOS) > 0:
			os = tr.T("cli.devcheck.os.only", i18n.Args{"os": strings.Join(s.OnlyOS, ", ")})
		case len(s.SkipOS) > 0:
			os = tr.T("cli.devcheck.os.not", i18n.Args{"os": strings.Join(s.SkipOS, ", ")})
		}
		set := tr.T("cli.devcheck.set.full", nil)
		if s.Fast {
			set = tr.T("cli.devcheck.set.fast", nil)
		}
		rows = append(rows, []string{s.Name, s.Tool, check.Pins[s.Tool].Version, os, set, strings.Join(append([]string{s.Tool}, s.Args...), " ")})
	}
	writeColumns(w, rows, func(cell string) int { return utf8.RuneCountInString(cell) })
}

// renderDevCheckReport prints what failed, then the table of every step with
// its result and duration, then the verdict as the last line.
func renderDevCheckReport(w io.Writer, rep check.Report, goos string, p *cli.Printer, tr i18n.Translator) {
	writeFailures(w, rep, p, tr)

	rows := [][]string{{tr.T("cli.devcheck.col.step", nil), tr.T("cli.devcheck.col.result", nil), tr.T("cli.devcheck.col.time", nil)}}
	var fastSkipped []string
	for _, r := range rep.Results {
		cell, took := resultCells(r, goos, p, tr)
		rows = append(rows, []string{r.Name, cell, took})
		if r.Skip == check.SkippedFast {
			fastSkipped = append(fastSkipped, r.Name)
		}
	}
	writeColumns(w, rows, p.Width)
	fmt.Fprintln(w)

	if len(fastSkipped) > 0 {
		fmt.Fprintln(w, tr.T("cli.devcheck.fastnote", i18n.Args{"steps": strings.Join(fastSkipped, ", ")}))
	}
	fmt.Fprintln(w, verdictLine(rep, p, tr))
}

// writeFailures prints the captured output of every step that failed, before
// the table, so the table and the verdict stay the last thing on screen.
func writeFailures(w io.Writer, rep check.Report, p *cli.Printer, tr i18n.Translator) {
	for _, r := range rep.Results {
		if r.Status != check.Failed {
			continue
		}
		fmt.Fprintln(w, p.Strong(tr.T("cli.devcheck.output", i18n.Args{"step": r.Name})))
		if out := strings.TrimRight(r.Output, "\n"); out != "" {
			fmt.Fprintln(w, out)
		}
		if r.Err != nil {
			fmt.Fprintln(w, r.Err)
		}
		fmt.Fprintln(w)
	}
}

// resultCells is the RESULT and TIME cells of one step.
func resultCells(r check.Result, goos string, p *cli.Printer, tr i18n.Translator) (result, took string) {
	switch r.Status {
	case check.Passed:
		result = p.OK(tr.T("cli.devcheck.result.passed", nil))
	case check.Failed:
		result = p.Fail(tr.T("cli.devcheck.result.failed", nil))
	case check.NotRun:
		result = p.Muted(tr.T("cli.devcheck.result.notrun", nil))
	default:
		result = p.Muted(skipText(r, goos, tr))
	}
	if r.Status == check.Passed || r.Status == check.Failed {
		took = fmt.Sprintf("%.1fs", r.Duration.Seconds())
	}
	return result, took
}

// verdictLine is the last line of a run: green or red, the counts and the
// total time. It is the same words locally and in CI.
func verdictLine(rep check.Report, p *cli.Printer, tr i18n.Translator) string {
	n := map[check.Status]int{}
	for _, r := range rep.Results {
		n[r.Status]++
	}
	verdict := p.OK(tr.T("cli.devcheck.green", nil))
	if rep.Code != 0 {
		verdict = p.Fail(tr.T("cli.devcheck.red", nil))
	}
	counts := []string{tr.N("cli.devcheck.count.passed", n[check.Passed], nil)}
	if k := n[check.Failed]; k > 0 {
		counts = append(counts, tr.N("cli.devcheck.count.failed", k, nil))
	}
	if k := n[check.NotRun]; k > 0 {
		counts = append(counts, tr.N("cli.devcheck.count.notrun", k, nil))
	}
	if k := n[check.Skipped]; k > 0 {
		counts = append(counts, tr.N("cli.devcheck.count.skipped", k, nil))
	}
	return fmt.Sprintf("%s: %s, %.1fs", verdict, strings.Join(counts, ", "), rep.Duration.Seconds())
}

func skipText(r check.Result, goos string, tr i18n.Translator) string {
	switch r.Skip {
	case check.SkippedFast:
		return tr.T("cli.devcheck.skip.fast", nil)
	case check.SkippedOnly:
		return tr.T("cli.devcheck.skip.only", nil)
	}
	return tr.T("cli.devcheck.skip.os", i18n.Args{"os": goos})
}

// writeColumns prints rows as aligned columns, two spaces apart, with no
// trailing padding on the last column.
func writeColumns(w io.Writer, rows [][]string, width func(string) int) {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], width(cell))
		}
	}
	for _, row := range rows {
		var b strings.Builder
		for i, cell := range row {
			b.WriteString(cell)
			if i < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-width(cell)+2))
			}
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}
}

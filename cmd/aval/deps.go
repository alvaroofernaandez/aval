package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/alvaroofernaandez/aval/internal/core/deps"
	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/ui/cli"
)

// runDeps is `aval deps [--json] [--layer harness|agent|env]`: the external
// tool inventory and what this machine has of it. It needs no repository and
// no network, and exits 1 (errChecksFailed) when a blocking tool is missing.
func runDeps(args []string, flags uiFlags, c cmdIO) error {
	ui, err := depsSettings(flags, c)
	if err != nil {
		return err
	}
	tr := i18n.New(ui.Lang)
	usage := tr.T("cli.deps.usage", nil)

	fs := newFlagSet("deps", io.Discard)
	asJSON := fs.Bool("json", false, "print the inventory as a JSON array")
	layerFlag := fs.String("layer", "", "list one layer: harness, agent or env")
	if err := fs.Parse(args); err != nil {
		return &usageError{msg: usage + ": " + err.Error(), err: err}
	}
	if fs.NArg() > 0 {
		return newUsage(fmt.Sprintf("%s: unexpected argument %q", usage, strings.Join(fs.Args(), " ")))
	}

	m, err := deps.Embedded()
	if err != nil {
		return err
	}
	entries := m.Entries
	if *layerFlag != "" {
		l, err := deps.ParseLayer(*layerFlag)
		if err != nil {
			msg := tr.T("cli.deps.unknown_layer", i18n.Args{"layer": strconv.Quote(*layerFlag)})
			return &usageError{msg: msg + "\n" + usage, err: err}
		}
		entries = deps.Filter(entries, l)
	}
	slices.SortStableFunc(entries, func(a, b deps.Entry) int {
		return slices.Index(deps.Layers, a.Layer) - slices.Index(deps.Layers, b.Layer)
	})

	prober := c.prober
	if prober == nil {
		prober = deps.SystemProber{GOOS: runtime.GOOS, Getenv: c.env}
	}
	reports := deps.Check(context.Background(), entries, prober, deps.DefaultTimeout)

	if *asJSON {
		if err := renderDepsJSON(c.stdout, reports); err != nil {
			return err
		}
	} else {
		renderDepsTable(c.stdout, reports, newPrinterFor(c.stdout, ui), tr)
	}
	if len(deps.BlockingMissing(reports)) > 0 {
		return errChecksFailed
	}
	return nil
}

// depsSettings resolves the UI settings for the table; without a home it
// keeps the language only.
func depsSettings(flags uiFlags, c cmdIO) (uiSettings, error) {
	home, err := userHomeDir(c.env)
	if err != nil {
		return langOnlySettings(flags, c.env)
	}
	return settingsForCommand("deps", flags, c.env, home, c.stderr)
}

// depsRow is one line of `aval deps --json`. The field set and order are a
// contract: cmd/aval/testdata/deps.golden.json pins them.
type depsRow struct {
	Name             string   `json:"name"`
	Layer            string   `json:"layer"`
	UsedBy           []string `json:"used_by"`
	IfMissing        string   `json:"if_missing"`
	NativeSubstitute string   `json:"native_substitute"`
	Probe            string   `json:"probe"`
	Status           string   `json:"status"`
	Path             string   `json:"path"`
	Version          string   `json:"version"`
}

func renderDepsJSON(w io.Writer, rs []deps.Report) error {
	rows := make([]depsRow, 0, len(rs))
	for _, r := range rs {
		probe := append([]string{string(r.Probe.Kind), r.Probe.Target}, r.Probe.Args...)
		rows = append(rows, depsRow{
			Name: r.Name, Layer: string(r.Layer), UsedBy: r.UsedBy, IfMissing: string(r.IfMissing),
			NativeSubstitute: r.NativeSubstitute, Probe: strings.Join(probe, " "),
			Status: string(r.Status), Path: r.Path, Version: r.Version,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

// renderDepsTable prints the aligned table and the summary line. The status
// glyph and colour come from p; a pipe gets the plain words.
func renderDepsTable(w io.Writer, rs []deps.Report, p *cli.Printer, tr i18n.Translator) {
	header := []string{
		tr.T("cli.deps.col.status", nil), tr.T("cli.deps.col.tool", nil), tr.T("cli.deps.col.layer", nil),
		tr.T("cli.deps.col.if_missing", nil), tr.T("cli.deps.col.substitute", nil), tr.T("cli.deps.col.used_by", nil),
	}
	rows := [][]string{header}
	var present, missing, skipped int
	for _, r := range rs {
		var cell string
		switch r.Status {
		case deps.Present:
			present++
			cell = p.OK(tr.T("cli.deps.status.present", nil))
		case deps.Skipped:
			skipped++
			cell = p.Muted(tr.T("cli.deps.status.skipped", nil))
		default:
			missing++
			word := tr.T("cli.deps.status.missing", nil)
			if r.IfMissing == deps.Blocks {
				cell = p.Fail(word)
			} else {
				cell = p.Warn(word)
			}
		}
		rows = append(rows, []string{cell, r.Name, string(r.Layer), string(r.IfMissing), r.NativeSubstitute, strings.Join(r.UsedBy, ", ")})
	}
	width := func(col, row int) int {
		if col == 0 && row > 0 {
			return p.Width(rows[row][0])
		}
		return utf8.RuneCountInString(rows[row][col])
	}
	widths := make([]int, len(header))
	for i := range rows {
		for col := range header {
			widths[col] = max(widths[col], width(col, i))
		}
	}
	for i, row := range rows {
		var b strings.Builder
		for col, cell := range row {
			if col == len(row)-1 {
				b.WriteString(cell)
				break
			}
			b.WriteString(cell + strings.Repeat(" ", widths[col]-width(col, i)+2))
		}
		fmt.Fprintln(w, b.String())
	}
	fmt.Fprintf(w, "\n%s, %s, %s\n",
		tr.N("cli.deps.count.present", present, nil),
		tr.N("cli.deps.count.missing", missing, nil),
		tr.N("cli.deps.count.skipped", skipped, nil))
}

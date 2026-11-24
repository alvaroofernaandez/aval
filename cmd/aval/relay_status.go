package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/relay"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/claudemod"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/statefs"
)

// relayStatus is what `aval relay status` reports. The JSON keys are the
// machine contract and stay English.
type relayStatus struct {
	Enabled bool            `json:"enabled"`
	Window  int             `json:"window"`
	Runtime string          `json:"runtime"`
	Tier    string          `json:"tier"`
	Source  string          `json:"source"`
	ErrPct  float64         `json:"err_pct"`
	Repo    string          `json:"repo"`
	Paused  bool            `json:"paused"`
	State   string          `json:"state_dir"`
	Relays  int             `json:"relays"`
	Last    *statefs.Metric `json:"last"`

	tier relay.Tier
}

// readRelayStatus gathers the status from the live Claude Code settings and
// the relay state. Nothing recorded yet is a status, not an error.
func readRelayStatus(c cmdIO) (relayStatus, error) {
	tier := relay.TierOf(claudemod.Capabilities, relay.ModeNativeUI)
	st := relayStatus{
		Runtime: claudemod.Runtime, Tier: tier.String(), tier: tier,
		Source: claudemod.Source, ErrPct: relay.ErrPctOf(claudemod.Source),
	}
	if home, err := userHomeDir(c.env); err == nil && home != "" {
		settings, _, err := readSettings(filepath.Join(home, ".claude", "settings.json"))
		if err != nil {
			return relayStatus{}, err
		}
		env, _ := settings["env"].(map[string]any)
		st.Enabled = env[envRelay] == "1"
		if w, ok := env[envWindow].(string); ok {
			st.Window, _ = strconv.Atoi(w)
		}
	}
	root, err := relayRoot()
	if err != nil {
		return relayStatus{}, err
	}
	st.Repo = root
	store, ok := relayStore(c.env)
	if !ok {
		return st, nil
	}
	st.State, st.Paused = store.Dir, store.Paused(root)
	metrics, err := store.ReadMetrics()
	if err != nil {
		return relayStatus{}, err
	}
	st.Relays = len(metrics)
	if len(metrics) > 0 {
		st.Last = &metrics[len(metrics)-1]
	}
	return st, nil
}

// relayStatusCmd runs `aval relay status [--json]`.
func relayStatusCmd(asJSON bool, flags uiFlags, c cmdIO) error {
	st, err := readRelayStatus(c)
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSON(c.stdout, st)
	}
	ui, err := langOnlySettings(flags, c.env)
	if err != nil {
		return err
	}
	home, _ := userHomeDir(c.env)
	return writeRows(c.stdout, statusRows(st, i18n.New(ui.Lang), home))
}

// statusRows are the five rows of the human status, label then value.
func statusRows(st relayStatus, tr i18n.Translator, home string) [][2]string {
	state := tr.T("cli.relay.status.off", nil)
	if st.Enabled {
		state = tr.T("cli.relay.status.on", i18n.Args{"window": st.Window})
	}
	repo := tr.T("cli.relay.status.active", i18n.Args{"repo": tildePath(st.Repo, home)})
	if st.Paused {
		repo = tr.T("cli.relay.status.paused", i18n.Args{"repo": tildePath(st.Repo, home)})
	}
	relays := tr.T("cli.relay.status.none", i18n.Args{"dir": tildePath(st.State, home)})
	if st.Last != nil {
		relays = tr.N("cli.relay.status.relays", st.Relays, i18n.Args{
			"before": relay.Tokens(st.Last.Before), "after": relay.Tokens(st.Last.After),
			"at": st.Last.At.Local().Format("2006-01-02 15:04"),
		})
	}
	return [][2]string{
		{tr.T("cli.relay.status.label.relay", nil), state},
		{tr.T("cli.relay.status.label.tier", nil), tr.T(tierKeys[st.tier], i18n.Args{"tier": st.Tier})},
		{tr.T("cli.relay.status.label.context", nil), tr.T("cli.relay.status.context", i18n.Args{
			"source": tr.T(sourceKey(st.Source), nil), "bound": strconv.Itoa(int(st.ErrPct*100 + 0.5)),
		})},
		{tr.T("cli.relay.status.label.repo", nil), repo},
		{tr.T("cli.relay.status.label.relays", nil), relays},
	}
}

// relayLogCmd runs `aval relay log [--json]`: every committed relay, oldest
// first, from metrics.jsonl.
func relayLogCmd(asJSON bool, flags uiFlags, c cmdIO) error {
	var metrics []statefs.Metric
	dir := ""
	if store, ok := relayStore(c.env); ok {
		var err error
		if metrics, err = store.ReadMetrics(); err != nil {
			return err
		}
		dir = store.Dir
	}
	if asJSON {
		if metrics == nil {
			metrics = []statefs.Metric{}
		}
		return writeJSON(c.stdout, metrics)
	}
	ui, err := langOnlySettings(flags, c.env)
	if err != nil {
		return err
	}
	tr := i18n.New(ui.Lang)
	if len(metrics) == 0 {
		home, _ := userHomeDir(c.env)
		_, err := fmt.Fprintln(c.stdout, tr.T("cli.relay.log.none", i18n.Args{"dir": tildePath(dir, home)}))
		return err
	}
	for _, m := range metrics {
		line := tr.T("cli.relay.log.row", i18n.Args{
			"at": m.At.Local().Format("2006-01-02 15:04"), "trigger": m.Trigger, "action": m.Action,
			"before": relay.Tokens(m.Before), "after": relay.Tokens(m.After),
		})
		if _, err := fmt.Fprintln(c.stdout, line); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(out, '\n'))
	return err
}

// writeRows writes label and value columns, the labels padded to the widest.
func writeRows(w io.Writer, rows [][2]string) error {
	width := 0
	for _, r := range rows {
		width = max(width, len([]rune(r[0])))
	}
	for _, r := range rows {
		pad := strings.Repeat(" ", width-len([]rune(r[0]))+2)
		if _, err := fmt.Fprintln(w, r[0]+pad+r[1]); err != nil {
			return err
		}
	}
	return nil
}

// relayReport runs status or log; anything else is a usage error.
func relayReport(sub string, asJSON bool, flags uiFlags, c cmdIO) error {
	switch sub {
	case "status":
		return relayStatusCmd(asJSON, flags, c)
	case "log":
		return relayLogCmd(asJSON, flags, c)
	}
	return newUsage(relayUsage)
}

// tierKeys name what each delivery tier does for the person.
var tierKeys = map[relay.Tier]string{
	relay.T0: i18n.Key("cli.relay.tier.T0"),
	relay.T1: i18n.Key("cli.relay.tier.T1"),
	relay.T2: i18n.Key("cli.relay.tier.T2"),
	relay.T3: i18n.Key("cli.relay.tier.T3"),
	relay.T4: i18n.Key("cli.relay.tier.T4"),
}

// sourceKey names a reading source for the person; an unknown one is a
// heuristic, as relay.ErrPctOf treats it.
func sourceKey(source string) string {
	switch source {
	case "exact":
		return i18n.Key("cli.relay.source.exact")
	case "provider":
		return i18n.Key("cli.relay.source.provider")
	case "transcript":
		return i18n.Key("cli.relay.source.transcript")
	}
	return i18n.Key("cli.relay.source.heuristic")
}

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/relay"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/claudemod"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/facts"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/statefs"
)

const relayUsage = "usage: aval relay status|log [--json] | pause|resume|enable|disable | compact|commit|notify"

// Reasons only the command gives (the core's are in package relay).
const (
	reasonDisabled = "disabled" // AVAL_RELAY=0
	reasonLocked   = "locked"   // another relay of the session is running
	reasonNoState  = "no_state" // no state directory: nothing can be recorded
	reasonState    = "state"    // the state directory failed
)

// maxRelayInput bounds what compact reads from the mod.
const maxRelayInput = 64 << 20

// relayLockTTL is how long a session lock counts as held. A compact answers
// in milliseconds and the mod gives it a timeout well under this, so an older
// lock belongs to a process that died.
const relayLockTTL = 30 * time.Second

// runRelay runs `aval relay <sub>`.
func runRelay(sub []string, flags uiFlags, c cmdIO) error {
	if len(sub) == 2 && sub[1] == "--json" {
		return relayReport(sub[0], true, flags, c)
	}
	if len(sub) != 1 {
		return newUsage(relayUsage)
	}
	switch sub[0] {
	case "status", "log":
		return relayReport(sub[0], false, flags, c)
	case "compact":
		return relayCompact(c)
	case "notify":
		return relayNotify(c)
	case "commit":
		return relayCommit(c)
	case "pause", "resume":
		return relaySetPaused(sub[0] == "pause", flags, c)
	case "enable", "disable":
		return relaySetEnabled(sub[0] == "enable", flags, c)
	}
	return newUsage(relayUsage)
}

// relaySetPaused pauses or resumes the relay for the repository that holds
// the working directory (the directory itself outside a repository).
func relaySetPaused(on bool, flags uiFlags, c cmdIO) error {
	ui, err := langOnlySettings(flags, c.env)
	if err != nil {
		return err
	}
	store, ok := relayStore(c.env)
	if !ok {
		return errors.New("relay: no state directory: set HOME or XDG_STATE_HOME")
	}
	root, err := relayRoot()
	if err != nil {
		return err
	}
	if err := store.SetPaused(root, on); err != nil {
		return err
	}
	key := i18n.Key("cli.relay.resumed")
	if on {
		key = i18n.Key("cli.relay.paused")
	}
	_, err = fmt.Fprintln(c.stdout, i18n.New(ui.Lang).T(key, i18n.Args{"repo": root}))
	return err
}

// relayRoot is the repository root of the working directory, or the
// directory itself outside a repository.
func relayRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if root := facts.Root(wd); root != "" {
		return root, nil
	}
	return wd, nil
}

// relayCompact answers the mod's session.compact. It is a hook path: it reads
// no config, starts no process and touches no network, and whatever goes
// wrong it answers passthrough with exit 0, so Claude Code compacts as usual.
func relayCompact(c cmdIO) error {
	out, err := json.Marshal(compactAnswer(c))
	if err != nil {
		out = []byte(`{"version":1,"action":"passthrough"}`)
	}
	_, err = c.stdout.Write(append(out, '\n'))
	return err
}

func passthrough(reason string) relay.Response {
	return relay.Response{Version: relay.ProtocolVersion, Action: relay.AnswerPassthrough, Reason: reason}
}

// readCompactRequest reads and checks the mod's envelope; a non-empty reason
// means answer passthrough.
func readCompactRequest(c cmdIO) (relay.Request, string) {
	if c.env("AVAL_RELAY") == "0" {
		return relay.Request{}, reasonDisabled
	}
	raw, err := io.ReadAll(io.LimitReader(c.stdin, maxRelayInput))
	if err != nil {
		return relay.Request{}, relay.ReasonMalformed
	}
	req, err := claudemod.Decode(raw)
	if err != nil {
		return relay.Request{}, relay.ReasonMalformed
	}
	if req.Version != relay.ProtocolVersion {
		return relay.Request{}, relay.ReasonVersion
	}
	return req, ""
}

func compactAnswer(c cmdIO) relay.Response {
	req, reason := readCompactRequest(c)
	if reason != "" {
		return passthrough(reason)
	}
	store, ok := relayStore(c.env)
	if !ok {
		return passthrough(reasonNoState)
	}
	if root, err := relayRoot(); err == nil && store.Paused(root) {
		return passthrough(reasonPaused)
	}
	release, err := store.Lock(req.SessionID)
	switch {
	case errors.Is(err, statefs.ErrLocked):
		return passthrough(reasonLocked)
	case errors.Is(err, statefs.ErrBadSessionID):
		return passthrough(relay.ReasonMalformed)
	case err != nil:
		return passthrough(reasonState)
	}
	defer release()

	chain, err := store.ReadChain(req.SessionID)
	if err != nil {
		return passthrough(reasonState)
	}
	req.Chain = chain.Links
	if wd, err := os.Getwd(); err == nil {
		req.Capsule.Repo = facts.Repo(wd)
	}
	in, err := json.Marshal(req)
	if err != nil {
		return passthrough(relay.ReasonMalformed)
	}
	var resp relay.Response
	if err := json.Unmarshal(relay.Handle(in, relayPolicy(c.env), relayBrakes(c.env)), &resp); err != nil {
		return passthrough(relay.ReasonMalformed)
	}
	if resp.Action == relay.AnswerReplace {
		recordRelay(store, req, resp.Action, time.Now())
	}
	return resp
}

// relayStore is the relay state under $XDG_STATE_HOME/aval/relay, by default
// ~/.local/state/aval/relay, as the lessons hook keeps its state.
func relayStore(getenv func(string) string) (statefs.Store, bool) {
	state := getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := userHomeDir(getenv)
		if err != nil || home == "" {
			return statefs.Store{}, false
		}
		state = filepath.Join(home, ".local", "state")
	}
	now := time.Now()
	return statefs.Store{
		Dir:   filepath.Join(state, "aval", "relay"),
		PID:   os.Getpid(),
		Start: now.UnixNano(),
		Alive: func(_ int, start int64) bool { return now.Sub(time.Unix(0, start)) < relayLockTTL },
	}, true
}

// relayPolicy is the default policy with the thresholds the environment sets
// (D3 of odd/tasks/relay.md: env only, never the config). A value that is not
// a positive integer keeps the default.
func relayPolicy(getenv func(string) string) relay.Policy {
	p := relay.DefaultPolicy()
	for key, field := range map[string]*int{
		"AVAL_RELAY_SOFT_TOKENS":     &p.SoftTokens,
		"AVAL_RELAY_HARD_TOKENS":     &p.HardTokens,
		"AVAL_RELAY_COLD_MIN_TOKENS": &p.ColdMinTokens,
	} {
		if n, err := strconv.Atoi(getenv(key)); err == nil && n > 0 {
			*field = n
		}
	}
	return p
}

// recordRelay journals the answer, extends the chain and updates the summary
// the status line reads. A failure here does not change the answer.
func recordRelay(store statefs.Store, req relay.Request, action string, now time.Time) {
	_ = store.Prepare(req.SessionID, statefs.Entry{
		Trigger: req.Trigger, Action: action, Source: req.Reading.Source, Floor: req.Reading.Floor,
		TokensBefore: req.Reading.Tokens, At: now,
	})
	_ = store.AppendLink(req.SessionID, relay.Link{SessionID: req.SessionID, Fingerprint: req.Fingerprint, Turn: req.Turn})
	sum, _ := store.ReadSummary(req.SessionID)
	sum.Relays++
	sum.Before, sum.After, sum.Floor, sum.Trigger = req.Reading.Tokens, 0, req.Reading.Floor, req.Trigger
	_ = store.WriteSummary(req.SessionID, sum)
}

const reasonPaused = "paused" // aval relay pause in this repository

// errRelayModMissing: enable found no installed relay mod.
var errRelayModMissing = errors.New("relay: the relay mod is not installed")

// errRelayWindowTooSmall: the hard threshold gives a compaction window under
// Claude Code's 100,000-token minimum.
var errRelayWindowTooSmall = errors.New("relay: the compaction window is under Claude Code's minimum")

// relayBrakes is the default brakes with the limits the environment sets
// (D3: env only). A value that is not a positive integer keeps the default.
func relayBrakes(getenv func(string) string) relay.Brakes {
	b := relay.DefaultBrakes()
	if n, err := strconv.Atoi(getenv("AVAL_RELAY_MAX_CHAIN")); err == nil && n > 0 {
		b.MaxChain = n
	}
	return b
}

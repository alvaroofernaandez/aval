package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/claudemod"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/facts"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/statefs"
)

const relayUsage = "usage: aval relay compact"

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
func runRelay(sub []string, c cmdIO) error {
	if len(sub) != 1 || sub[0] != "compact" {
		return newUsage(relayUsage)
	}
	return relayCompact(c)
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

func compactAnswer(c cmdIO) relay.Response {
	if c.env("AVAL_RELAY") == "0" {
		return passthrough(reasonDisabled)
	}
	raw, err := io.ReadAll(io.LimitReader(c.stdin, maxRelayInput))
	if err != nil {
		return passthrough(relay.ReasonMalformed)
	}
	req, err := claudemod.Decode(raw)
	if err != nil {
		return passthrough(relay.ReasonMalformed)
	}
	if req.Version != relay.ProtocolVersion {
		return passthrough(relay.ReasonVersion)
	}
	store, ok := relayStore(c.env)
	if !ok {
		return passthrough(reasonNoState)
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
	if err := json.Unmarshal(relay.Handle(in, relayPolicy(c.env), relay.DefaultBrakes()), &resp); err != nil {
		return passthrough(relay.ReasonMalformed)
	}
	if resp.Action == relay.AnswerReplace {
		recordRelay(store, req, time.Now())
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
func recordRelay(store statefs.Store, req relay.Request, now time.Time) {
	_ = store.Prepare(req.SessionID, statefs.Entry{Trigger: req.Trigger, TokensBefore: req.Reading.Tokens, At: now})
	_ = store.AppendLink(req.SessionID, relay.Link{SessionID: req.SessionID, Fingerprint: req.Fingerprint, Turn: req.Turn})
	sum, _ := store.ReadSummary(req.SessionID)
	sum.Relays++
	sum.Before, sum.After, sum.Trigger = req.Reading.Tokens, 0, req.Trigger
	_ = store.WriteSummary(req.SessionID, sum)
}

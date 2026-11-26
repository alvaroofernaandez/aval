package main

import (
	"encoding/json"
	"io"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

const reasonRecent = "recent" // a relay just happened: no second clear on its heels

// boundaryRecent is how recent a journal entry must be to hold a boundary
// clear back, so a compaction relay and a boundary clear never double up.
const boundaryRecent = time.Minute

// relayBoundary answers the mod's turn.complete (relay-autoclear T4): clear
// the session now, or not. A hot path like compact: it reads no config and
// starts no process, and anything unexpected answers none with exit 0, so a
// boundary never clears on an error.
func relayBoundary(c cmdIO) error {
	out, err := json.Marshal(boundaryAnswer(c))
	if err != nil {
		out = []byte(`{"version":1,"action":"none"}`)
	}
	_, err = c.stdout.Write(append(out, '\n'))
	return err
}

func boundaryNone(reason string) relay.BoundaryAnswer {
	return relay.BoundaryAnswer{Version: relay.ProtocolVersion, Action: relay.AnswerNone, Reason: reason}
}

func boundaryAnswer(c cmdIO) relay.BoundaryAnswer {
	if c.env("AVAL_RELAY") != "1" {
		return boundaryNone(relay.ReasonDisabled)
	}
	var req relay.BoundaryRequest
	raw, err := io.ReadAll(io.LimitReader(c.stdin, maxRelayInput))
	if err != nil || json.Unmarshal(raw, &req) != nil {
		return boundaryNone(relay.ReasonMalformed)
	}
	a := relayPolicy(c.env).DecideBoundary(true, req)
	if a.Action != relay.AnswerClear {
		return a
	}
	if store, ok := relayStore(c.env); ok {
		if root, err := relayRoot(); err == nil && store.Paused(root) {
			return boundaryNone(reasonPaused)
		}
		if store.RelayedSince(time.Now().Add(-boundaryRecent)) {
			return boundaryNone(reasonRecent)
		}
	}
	return a
}

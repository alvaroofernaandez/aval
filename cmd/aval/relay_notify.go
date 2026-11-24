package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/notify"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/statefs"
)

// notifyPlatform and notifyRun are the machine and the process start of
// `aval relay notify`; tests replace them.
var (
	notifyPlatform = func(getenv func(string) string) notify.Platform {
		return notify.Platform{
			GOOS: runtime.GOOS, WSL: getenv("WSL_DISTRO_NAME") != "", LookPath: exec.LookPath,
			WSLEnv: getenv("WSLENV"), TTY: hasTTY(),
		}
	}
	notifyRun = func(ctx context.Context, argv, env []string) error {
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Env = append(os.Environ(), env...)
		return cmd.Run()
	}
	notifyTTY = func(seq string) error {
		f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(seq)
		return err
	}
)

func hasTTY() bool {
	f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// notifyTimeout bounds the notifier process; a hung one is killed.
const notifyTimeout = 3 * time.Second

// actionTiers maps the answer a relay was given to the tier that delivered it.
var actionTiers = map[string]relay.Tier{
	relay.AnswerReplace:    relay.T0,
	relay.AnswerSteer:      relay.T1,
	relay.AnswerNewSession: relay.T2,
}

// relayNotify shows the notification of the session's committed relay. The
// mod starts it after the answer, never inside compact. Like compact it reads
// no config and fails silently: a notification never fails a relay.
// AVAL_RELAY_NOTIFY=0 turns it off.
func relayNotify(c cmdIO) error {
	var in struct {
		Version   int    `json:"version"`
		SessionID string `json:"session_id"`
	}
	raw, err := io.ReadAll(io.LimitReader(c.stdin, 1<<16))
	if err != nil || json.Unmarshal(raw, &in) != nil || in.Version != relay.ProtocolVersion || c.env("AVAL_RELAY_NOTIFY") == "0" {
		return nil
	}
	store, ok := relayStore(c.env)
	if !ok {
		return nil
	}
	e, err := store.Read(in.SessionID)
	tier, known := actionTiers[e.Action]
	if err != nil || e.State != statefs.Committed || !known {
		return nil
	}
	text := relay.NotifyText(relay.Outcome{Tier: tier, Before: e.TokensBefore, After: e.TokensAfter, Floor: e.Floor})
	d := notify.Plan(notifyPlatform(c.env), text)
	switch {
	case len(d.Argv) > 0:
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		defer cancel()
		_ = notifyRun(ctx, d.Argv, d.Env)
	case d.OSC != "":
		_ = notifyTTY(d.OSC)
	}
	return nil
}

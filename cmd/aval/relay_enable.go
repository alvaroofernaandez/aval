package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/statefs"
)

// compactionBuffer is how far before its window Claude Code fires an auto
// compaction: CLAUDE_CODE_AUTO_COMPACT_WINDOW=100000 compacted at 67,000
// (S3 of spike #63, measured at 100k only).
const compactionBuffer = 33_000

// minCompactionWindow is the smallest window Claude Code accepts: below it,
// it "treats it as the 100k minimum" (its own warning, Claude Code 2.1.331).
const minCompactionWindow = 100_000

// relayEnvKeys are the live settings env keys enable sets and disable removes.
const (
	envRelay      = "AVAL_RELAY"
	envWindow     = "CLAUDE_CODE_AUTO_COMPACT_WINDOW"
	envPluginDirs = "CLAUDE_CODE_PLUGIN_DIRS"
)

// relaySetEnabled turns the relay on or off in the live Claude Code settings:
// AVAL_RELAY, the compaction window (hard tokens plus the buffer, so the
// engine fires its auto compaction where aval's hard threshold is) and the
// mod in the plugin dirs. The repo settings never carry these keys, so aval
// sync keeps them (it merges env and keeps live-only keys).
func relaySetEnabled(on bool, flags uiFlags, c cmdIO) error {
	ui, err := langOnlySettings(flags, c.env)
	if err != nil {
		return err
	}
	home, err := userHomeDir(c.env)
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	mod := filepath.Join(home, ".claude", "aval-relay")
	if on {
		if _, err := os.Stat(mod); err != nil {
			return fmt.Errorf("%w at %s: run aval install first", errRelayModMissing, mod)
		}
	}
	path := filepath.Join(home, ".claude", "settings.json")
	settings, raw, err := readSettings(path)
	if err != nil {
		return err
	}
	env, _ := settings["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	window := relayPolicy(c.env).HardTokens + compactionBuffer
	if on && window < minCompactionWindow {
		return fmt.Errorf("%w: %d tokens is under %d; set AVAL_RELAY_HARD_TOKENS to %d or more",
			errRelayWindowTooSmall, window, minCompactionWindow, minCompactionWindow-compactionBuffer)
	}
	before := maps.Clone(env)
	editRelayEnv(env, on, mod, window)
	if !reflect.DeepEqual(before, env) {
		if len(env) == 0 {
			delete(settings, "env")
		} else {
			settings["env"] = env
		}
		if err := writeSettings(path, raw, settings); err != nil {
			return err
		}
	}
	tr := i18n.New(ui.Lang)
	if on {
		_, err = fmt.Fprintln(c.stdout, tr.T("cli.relay.enabled", i18n.Args{"window": window, "mod": mod}))
	} else {
		_, err = fmt.Fprintln(c.stdout, tr.T("cli.relay.disabled", nil))
	}
	return err
}

// editRelayEnv sets or removes the relay keys in env, leaving every other key
// and every other plugin dir as it was.
func editRelayEnv(env map[string]any, on bool, mod string, window int) {
	dirs := splitDirs(env[envPluginDirs])
	dirs = slicesDelete(dirs, mod)
	if on {
		env[envRelay] = "1"
		env[envWindow] = strconv.Itoa(window)
		dirs = append(dirs, mod)
	} else {
		delete(env, envRelay)
		delete(env, envWindow)
	}
	if len(dirs) == 0 {
		delete(env, envPluginDirs)
	} else {
		env[envPluginDirs] = strings.Join(dirs, string(os.PathListSeparator))
	}
}

func splitDirs(v any) []string {
	s, _ := v.(string)
	var out []string
	for _, d := range strings.Split(s, string(os.PathListSeparator)) {
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

func slicesDelete(dirs []string, drop string) []string {
	out := dirs[:0]
	for _, d := range dirs {
		if filepath.Clean(d) != filepath.Clean(drop) {
			out = append(out, d)
		}
	}
	return out
}

// readSettings returns the live settings and their bytes; a missing file is
// empty settings.
func readSettings(path string) (map[string]any, []byte, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil, fmt.Errorf("relay: %s is not valid JSON: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, raw, nil
}

// writeSettings keeps the previous bytes beside the file, then replaces it
// atomically with mode 0600.
func writeSettings(path string, previous []byte, m map[string]any) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if previous != nil {
		if err := os.WriteFile(path+".aval-relay.bak", previous, 0o600); err != nil {
			return fmt.Errorf("relay: backup of %s: %w", path, err)
		}
	}
	tmp := path + ".aval-relay.tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// relayCommit records the tokens the context holds after a relay, from the
// first session.measure the mod sees. Like compact it is a hook path: no
// config, no process, no network, nothing on stdout, never a failure.
func relayCommit(c cmdIO) error {
	var in struct {
		Version   int    `json:"version"`
		SessionID string `json:"session_id"`
		Tokens    int    `json:"tokens"`
	}
	raw, err := io.ReadAll(io.LimitReader(c.stdin, 1<<20))
	if err != nil || json.Unmarshal(raw, &in) != nil || in.Version != 1 || in.Tokens <= 0 {
		return nil
	}
	store, ok := relayStore(c.env)
	if !ok {
		return nil
	}
	prev, err := store.Read(in.SessionID)
	if err != nil {
		return nil
	}
	if err := store.Commit(in.SessionID, in.Tokens); err != nil {
		return nil
	}
	if prev.State == statefs.Prepared {
		_ = store.AppendMetric(statefs.Metric{
			At: prev.At, Session: in.SessionID, Trigger: prev.Trigger, Action: prev.Action,
			Source: prev.Source, Before: prev.TokensBefore, After: in.Tokens, Floor: prev.Floor, Repo: prev.Repo,
		})
	}
	sum, err := store.ReadSummary(in.SessionID)
	if err == nil {
		sum.After = in.Tokens
		_ = store.WriteSummary(in.SessionID, sum)
	}
	return nil
}

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

// FuzzRelayCompactInput feeds arbitrary bytes to `aval relay compact`. It
// must never fail or panic and must always print exactly one protocol
// response, so the mod can hand the runtime back a passthrough.
func FuzzRelayCompactInput(f *testing.F) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "relay", "compact-auto.json"))
	if err != nil {
		f.Fatal(err)
	}
	for _, s := range []string{
		string(fixture),
		`{"version":1,"event":"compact","session_id":"../x","messages":[]}`,
		`{"version":1,"event":"compact","session_id":"s","messages":[{"handle":"a","toolUses":[{"id":"t"},7,null]}]}`,
		`{"version":1,"event":"measure","session_id":"s","usage":{"tokens":-1,"window":0}}`,
		`{"version":1,"event":"compact","session_id":"s","messages":[{"text":"` + strings.Repeat("x", 1<<16) + `"}]}`,
		``, `null`, `[]`, `{"version":`,
	} {
		f.Add([]byte(s))
	}
	state := f.TempDir()
	env := map[string]string{"XDG_STATE_HOME": state, "HOME": filepath.Join(state, "home")}
	getenv := func(k string) string { return env[k] }

	f.Fuzz(func(t *testing.T, payload []byte) {
		var out, errOut bytes.Buffer
		if err := runRelay([]string{"compact"}, uiFlags{}, cmdIO{stdin: bytes.NewReader(payload), stdout: &out, stderr: &errOut, env: getenv}); err != nil {
			t.Fatalf("relay compact returned %v", err)
		}
		var resp relay.Response
		if err := json.Unmarshal(out.Bytes(), &resp); err != nil || resp.Version != relay.ProtocolVersion || resp.Action == "" {
			t.Fatalf("not one protocol response: %q (%v)", out.Bytes(), err)
		}
		if errOut.Len() > 0 {
			t.Fatalf("stderr on a hook path: %q", errOut.Bytes())
		}
	})
}

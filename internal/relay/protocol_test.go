package relay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func handle(t *testing.T, in string) Response {
	t.Helper()
	var r Response
	if err := json.Unmarshal(Handle([]byte(in), DefaultPolicy(), DefaultBrakes()), &r); err != nil {
		t.Fatalf("Handle returned invalid JSON: %v", err)
	}
	return r
}

func TestProtocol_GoldenIO(t *testing.T) {
	ins, err := filepath.Glob(filepath.Join("testdata", "protocol", "*.in.json"))
	if err != nil || len(ins) == 0 {
		t.Fatalf("no protocol fixtures: %v", err)
	}
	for _, in := range ins {
		t.Run(filepath.Base(in), func(t *testing.T) {
			req, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			got := Handle(req, DefaultPolicy(), DefaultBrakes())
			outPath := strings.TrimSuffix(in, ".in.json") + ".out.json"
			if *updateGolden {
				if err := os.WriteFile(outPath, append(got, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("read golden (run with -update to create it): %v", err)
			}
			if string(got)+"\n" != string(want) {
				t.Fatalf("response differs from %s:\n--- got\n%s\n--- want\n%s", outPath, got, want)
			}
		})
	}
}

func TestProtocol_CompactReplacesWithCapsuleAndTail(t *testing.T) {
	in, err := os.ReadFile(filepath.Join("testdata", "protocol", "compact-auto.in.json"))
	if err != nil {
		t.Fatal(err)
	}
	r := handle(t, string(in))
	if r.Action != AnswerReplace || r.Capsule == "" {
		t.Fatalf("got action %q with capsule %d bytes, want replace with a capsule", r.Action, len(r.Capsule))
	}
	if !slices.Equal(r.Keep, []string{"m2", "m3", "m4"}) {
		t.Fatalf("kept %v, want the pair-safe tail [m2 m3 m4]", r.Keep)
	}
}

func TestProtocol_Passthrough(t *testing.T) {
	cases := map[string]struct{ in, reason string }{
		"VersionMismatch": {`{"version":2,"event":"compact"}`, ReasonVersion},
		"UnknownEvent":    {`{"version":1,"event":"rename"}`, ReasonEvent},
		"MalformedInput":  {`{"version":`, ReasonMalformed},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := handle(t, c.in)
			if r.Action != AnswerPassthrough || r.Reason != c.reason {
				t.Fatalf("got %q/%q, want passthrough/%q", r.Action, r.Reason, c.reason)
			}
		})
	}
}

func TestProtocol_MeasureAsksForARelayAtHardTokens(t *testing.T) {
	r := handle(t, `{"version":1,"event":"measure","turn":9,"fingerprint":"f",
		"reading":{"tokens":200000,"window":1000000,"floor":58000,"source":"exact"}}`)
	if r.Action != AnswerRelay {
		t.Fatalf("got %q, want relay", r.Action)
	}
}

func TestProtocol_BrakeLetsTheNativeCompactionRun(t *testing.T) {
	r := handle(t, `{"version":1,"event":"compact","trigger":"auto","turn":41,"fingerprint":"same",
		"reading":{"tokens":201000,"window":1000000,"floor":58000,"source":"exact"},
		"chain":[{"session_id":"s","fingerprint":"same","turn":10}]}`)
	if r.Action != AnswerPassthrough || r.Reason != "brake:no_progress" {
		t.Fatalf("got %q/%q, want passthrough/brake:no_progress", r.Action, r.Reason)
	}
}

// A tail that keeps every message frees nothing: the capsule only adds to the
// context (found in the T10 sandbox run). The native compaction runs instead,
// steered by the capsule facts (ladder level 1).
func TestProtocol_TailKeepingEverythingSteers(t *testing.T) {
	r := handle(t, `{"version":1,"event":"compact","trigger":"auto","turn":3,"fingerprint":"f",
		"reading":{"tokens":59000,"window":1000000,"source":"exact"},
		"messages":[{"handle":"a","role":"user","tokens":10},{"handle":"b","role":"assistant","tokens":9000}],
		"capsule":{"first_prompt":"Turn 1"}}`)
	if r.Action != AnswerSteer || r.Reason != ReasonNoGain || !strings.Contains(r.Instructions, "Turn 1") {
		t.Fatalf("got %+v, want steer/%s with the first request in the instructions", r, ReasonNoGain)
	}
	if n := len(r.Instructions); n > steerMaxChars {
		t.Fatalf("instructions are %d chars, over %d", n, steerMaxChars)
	}
}

func TestSteerText_StaysUnderTheCap(t *testing.T) {
	got := SteerText(Capsule{FirstPrompt: strings.Repeat("ñandú ", 5_000)})
	if len(got) > steerMaxChars || !utf8.ValidString(got) {
		t.Fatalf("got %d bytes, valid UTF-8 %v; want at most %d, valid", len(got), utf8.ValidString(got), steerMaxChars)
	}
}

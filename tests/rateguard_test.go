package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// rate-guard (internal/agent/rateguard) runs before every Bash call and denies
// batched HTTP to hosts that ban the IP after rapid requests (Helice). These
// tests pin the wiring in settings.json, the binary's behaviour and its cost.

const rateGuardBatch = `{"session_id":"rg","tool_name":"Bash","tool_input":{"command":"for p in a b c; do curl -s https://www.innohelp.es/$p; done"}}`

func TestRateGuardBinaryDeniesABatchAndAllowsAOneOff(t *testing.T) {
	aval := avalBin(t)
	home := t.TempDir()
	r := run(repo, isoEnv(home), rateGuardBatch, aval, "hook", "rate-guard")
	eq(t, "exit code on deny", 0, r.Code)
	var v struct {
		H struct {
			D string `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
	}
	_ = json.Unmarshal([]byte(r.Stdout), &v)
	eq(t, "a curl loop on innohelp.es is denied", "deny", v.H.D)

	one := `{"tool_name":"Bash","tool_input":{"command":"curl -sL https://www.innohelp.es/"}}`
	r = run(repo, isoEnv(home), one, aval, "hook", "rate-guard")
	eq(t, "exit code on allow", 0, r.Code)
	eq(t, "a single curl is allowed (no output)", "", r.Out())
}

// hyperfineMeanMs runs cmd under hyperfine with stdin from the file in and
// returns the mean wall time in milliseconds.
func hyperfineMeanMs(t *testing.T, env []string, in, cmd string) float64 {
	t.Helper()
	out := filepath.Join(t.TempDir(), "hf.json")
	run(repo, env, "", "hyperfine", "-N", "--warmup", "10", "--runs", "50",
		"--input", in, "--export-json", out, cmd)
	var hf struct {
		Results []struct {
			Mean    *float64 `json:"mean"`
			Summary *struct {
				Wall struct {
					Mean float64 `json:"mean"`
				} `json:"time_wall_clock"`
			} `json:"summary"`
		} `json:"results"`
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("hyperfine wrote no report: %v", err)
	}
	_ = json.Unmarshal(b, &hf)
	if len(hf.Results) == 0 {
		t.Fatal("hyperfine report has no results")
	}
	mean := 0.0
	if hf.Results[0].Summary != nil { // newer hyperfine nests it under summary
		mean = hf.Results[0].Summary.Wall.Mean
	} else if hf.Results[0].Mean != nil {
		mean = *hf.Results[0].Mean
	}
	return mean * 1000
}

func TestRateGuardHookLatency(t *testing.T) {
	needTool(t, "hyperfine")
	slow(t)
	aval := avalBin(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	write(t, in, rateGuardBatch)
	ms := hyperfineMeanMs(t, isoEnv(dir), in, aval+" hook rate-guard")
	t.Logf("rate-guard hook mean: %.1f ms", ms)
	isTrue(t, "the rate-guard hook runs in under 10 ms (is "+strconv.FormatFloat(ms, 'f', 1, 64)+" ms)", ms > 0 && ms < 10)
}

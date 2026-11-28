package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/config"
)

const usageFixture = "../../internal/agent/usage/testdata"

// usageEnv is an isolated environment over the committed synthetic corpus.
func usageEnv(t *testing.T, cfg string) map[string]string {
	t.Helper()
	home := t.TempDir()
	env := withHomeVars(map[string]string{
		"HOME":                home,
		"XDG_CONFIG_HOME":     filepath.Join(home, "cfg"),
		"CLAUDE_CONFIG_DIR":   filepath.Join(usageFixture, "claude"),
		"PI_CODING_AGENT_DIR": filepath.Join(home, "no-pi"),
	})
	if cfg != "" {
		p := config.Path(envMap(env), home, runtime.GOOS)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return env
}

func fixedClock(t *testing.T) {
	t.Helper()
	oldNow, oldLoc := usageNow, usageLoc
	t.Cleanup(func() { usageNow, usageLoc = oldNow, oldLoc })
	now := time.Date(2026, 11, 21, 13, 0, 0, 0, time.UTC)
	usageNow, usageLoc = func() time.Time { return now }, time.UTC
}

type usageJSON struct {
	Billing string
	By      string
	Rows    []struct {
		Key      string
		Messages int
		Total    int64
		CostUSD  *float64 `json:"cost_usd"`
	}
	Total struct {
		Total   int64
		CostUSD *float64 `json:"cost_usd"`
	}
	Block5h struct {
		Active           bool
		RemainingSeconds int64    `json:"remaining_seconds"`
		SharePercent     *float64 `json:"share_percent"`
	} `json:"block_5h"`
	Week7d struct {
		SharePercent *float64 `json:"share_percent"`
	} `json:"week_7d"`
	Over bool
}

func runUsageJSON(t *testing.T, env map[string]string, args ...string) (usageJSON, invocation) {
	t.Helper()
	r := runIn(t, env, "", append([]string{"usage", "--json"}, args...)...)
	var u usageJSON
	if err := json.Unmarshal([]byte(r.stdout), &u); err != nil {
		t.Fatalf("stdout is not JSON (%v): %q stderr %q", err, r.stdout, r.stderr)
	}
	return u, r
}

func TestUsageByDayInApiBillingMatchesTheFixture(t *testing.T) {
	fixedClock(t)
	u, r := runUsageJSON(t, usageEnv(t, "usage.billing = api\n"), "--by", "day")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if u.Billing != "api" || len(u.Rows) != 2 || u.Rows[0].Key != "2026-11-20" || u.Rows[0].Total != 35250 || u.Total.Total != 61000 {
		t.Errorf("report = %+v", u)
	}
	if u.Total.CostUSD == nil || *u.Total.CostUSD != 0.138525 {
		t.Errorf("total cost = %v, want 0.138525", u.Total.CostUSD)
	}
	if !u.Block5h.Active || u.Block5h.RemainingSeconds != 7200 {
		t.Errorf("5h block = %+v, want open with 7200 s left", u.Block5h)
	}
}

func TestUsageSubscriptionShowsTheReportedShareAndNoCost(t *testing.T) {
	fixedClock(t)
	env := usageEnv(t, "usage.billing = subscription\n")
	rl := filepath.Join(t.TempDir(), "rl.json")
	if err := os.WriteFile(rl, []byte(`{"rate_limits":{"five_hour":{"used_percentage":42.4}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	u, _ := runUsageJSON(t, env, "--rate-limits", rl)
	if u.Block5h.SharePercent == nil || *u.Block5h.SharePercent != 42.4 || u.Total.CostUSD != nil {
		t.Errorf("subscription report = %+v, want share 42.4 and no cost", u)
	}
	u, _ = runUsageJSON(t, env)
	if u.Block5h.SharePercent != nil {
		t.Errorf("share = %v without runtime data, want n/a", *u.Block5h.SharePercent)
	}
}

func TestUsageCheckExitsNonZeroOnlyOverBudget(t *testing.T) {
	fixedClock(t)
	over := usageEnv(t, "usage.budget.headless.7d = 20000\n")
	if u, r := runUsageJSON(t, over, "--check"); r.code != 1 || !u.Over {
		t.Errorf("over budget: exit %d over=%v, want 1 and true", r.code, u.Over)
	}
	if u, r := runUsageJSON(t, over); r.code != 0 || !u.Over {
		t.Errorf("without --check: exit %d over=%v, want 0 (report only) and true", r.code, u.Over)
	}
	under := usageEnv(t, "usage.budget.headless.7d = 30000\n")
	if _, r := runUsageJSON(t, under, "--check"); r.code != 0 {
		t.Errorf("under budget: exit %d, want 0", r.code)
	}
}

func TestUsageByConsumerSeparatesHeadlessFromInteractive(t *testing.T) {
	fixedClock(t)
	env := usageEnv(t, "")
	piDir := filepath.Join(t.TempDir(), "pi")
	if err := os.MkdirAll(filepath.Join(piDir, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(usageFixture, "pi", "sessions", "--work-gamma--", "2026-11-21T13-00-00_p1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(piDir, "sessions", "p1.jsonl"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	env["PI_CODING_AGENT_DIR"] = piDir
	u, _ := runUsageJSON(t, env, "--by", "consumer")
	got := map[string]int{}
	for _, r := range u.Rows {
		got[r.Key] = r.Messages
	}
	if got["interactive"] != 5 || got["headless"] != 1 || got["daemon"] != 2 {
		t.Errorf("rows = %v, want interactive 5, headless 1, daemon 2 (Pi)", got)
	}
}

func TestUsageRejectsBadFlagsAsUsageErrors(t *testing.T) {
	fixedClock(t)
	for _, args := range [][]string{{"--by", "week"}, {"--window", "1h"}, {"--nope"}} {
		var ue *usageError
		err := runUsage(args, cmdIO{env: envMap(usageEnv(t, "")), stdout: discard{}, stderr: discard{}})
		if !errors.As(err, &ue) {
			t.Errorf("%v: err = %v, want a usage error", args, err)
		}
	}
}

// Reading is all aval does to a transcript, and nothing asks for the API key.
func TestUsageLeavesTranscriptsAloneAndNeverAsksForTheAPIKey(t *testing.T) {
	fixedClock(t)
	env := usageEnv(t, "usage.billing = api\n")
	dir := t.TempDir()
	copyTree(t, filepath.Join(usageFixture, "claude"), dir)
	env["CLAUDE_CONFIG_DIR"] = dir
	before := treeHash(t, dir)
	get := envMap(env)
	var out, errb discardBuf
	err := runUsage([]string{"--json"}, cmdIO{env: func(k string) string {
		if k == "ANTHROPIC_API_KEY" {
			t.Errorf("aval usage read %s", k)
		}
		return get(k)
	}, stdout: &out, stderr: &errb})
	if err != nil {
		t.Fatal(err)
	}
	if treeHash(t, dir) != before {
		t.Error("a transcript changed")
	}
}

func treeHash(t *testing.T, root string) [32]byte {
	t.Helper()
	h := sha256.New()
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			h.Write([]byte(p))
			h.Write(b)
		}
		return nil
	})
	var s [32]byte
	copy(s[:], h.Sum(nil))
	return s
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(to, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

type discardBuf struct{ discard }

func TestUsageBillingDefaultsToSubscription(t *testing.T) {
	fixedClock(t)
	u, r := runUsageJSON(t, usageEnv(t, ""), "--by", "day")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if u.Billing != "subscription" || u.Total.CostUSD != nil {
		t.Errorf("billing = %q, cost = %v, want subscription and no cost", u.Billing, u.Total.CostUSD)
	}
}

// storedLimits writes the status line's rate-limits.json under the env's state
// home, aged by age against the fixed clock.
func storedLimits(t *testing.T, env map[string]string, body string, age time.Duration) {
	t.Helper()
	dir := filepath.Join(env["XDG_STATE_HOME"], "aval")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "rate-limits.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	at := usageNow().Add(-age)
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestUsageReadsTheStatusLineRateLimitsWithoutTheFlag(t *testing.T) {
	fixedClock(t)
	env := usageEnv(t, "")
	env["XDG_STATE_HOME"] = filepath.Join(t.TempDir(), "state")
	storedLimits(t, env, `{"rate_limits":{"five_hour":{"used_percentage":42},"seven_day":{"used_percentage":17}}}`, time.Minute)

	u, _ := runUsageJSON(t, env)
	if u.Block5h.SharePercent == nil || *u.Block5h.SharePercent != 42 || u.Week7d.SharePercent == nil || *u.Week7d.SharePercent != 17 {
		t.Errorf("shares = %v / %v, want 42 and 17 from the stored reading", u.Block5h.SharePercent, u.Week7d.SharePercent)
	}

	flag := filepath.Join(t.TempDir(), "rl.json")
	if err := os.WriteFile(flag, []byte(`{"rate_limits":{"five_hour":{"used_percentage":9}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	u, _ = runUsageJSON(t, env, "--rate-limits", flag)
	if u.Block5h.SharePercent == nil || *u.Block5h.SharePercent != 9 || u.Week7d.SharePercent != nil {
		t.Errorf("with the flag the stored reading must not be used: %v / %v", u.Block5h.SharePercent, u.Week7d.SharePercent)
	}
}

func TestUsageIgnoresAStaleStoredReading(t *testing.T) {
	fixedClock(t)
	env := usageEnv(t, "")
	env["XDG_STATE_HOME"] = filepath.Join(t.TempDir(), "state")
	storedLimits(t, env, `{"rate_limits":{"five_hour":{"used_percentage":42}}}`, 6*time.Hour)
	if u, _ := runUsageJSON(t, env); u.Block5h.SharePercent != nil {
		t.Errorf("share = %v from a reading older than the 5 h window, want n/a", *u.Block5h.SharePercent)
	}
}

// What `aval statusline` writes is what `aval usage` reads.
func TestStatuslineAndUsageAgreeOnTheStateFile(t *testing.T) {
	fixedClock(t)
	env := usageEnv(t, "")
	env["XDG_STATE_HOME"] = filepath.Join(t.TempDir(), "state")
	in := `{"model":{"display_name":"Opus"},"rate_limits":{"five_hour":{"used_percentage":31}}}`
	if r := runIn(t, env, in, "statusline"); r.code != 0 {
		t.Fatalf("statusline exit %d: %q", r.code, r.stderr)
	}
	u, _ := runUsageJSON(t, env)
	if u.Block5h.SharePercent == nil || *u.Block5h.SharePercent != 31 {
		t.Errorf("share = %v, want 31 as the status line reported", u.Block5h.SharePercent)
	}
}

package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// rate-guard (internal/agent/rateguard) runs before every Bash call and denies
// batched HTTP to hosts that ban the IP after rapid requests (Helice). These
// tests pin the wiring in settings.json, the binary's behaviour and its cost.

const rateGuardBatch = `{"session_id":"rg","tool_name":"Bash","tool_input":{"command":"for p in a b c; do curl -s https://www.innohelp.es/$p; done"}}`

// Smoke for `aval hook rate-guard` as a real process. Deny and allow are
// asserted in process in cmd/aval/run_test.go (TestRunHookRateGuardInProcess).
func TestRateGuardBinaryDeniesABatch(t *testing.T) {
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
}

// latency is one hyperfine measurement, in milliseconds.
type latency struct{ Min, Mean, Median float64 }

// String prints every statistic, so a failure shows how loaded the run was.
func (l latency) String() string {
	return fmt.Sprintf("min %.1f ms, median %.1f ms, mean %.1f ms", l.Min, l.Median, l.Mean)
}

// under is the gate of an absolute latency budget. It is the fastest run:
// load from other processes only ever adds time, so a busy machine raises the
// mean and the median but not the floor, while a real regression raises every
// run, the fastest included (#125).
func (l latency) under(ms float64) bool { return l.Min > 0 && l.Min < ms }

// hyperfineMs runs cmd under hyperfine with stdin from the file in.
func hyperfineMs(t *testing.T, env []string, in, cmd string) latency {
	t.Helper()
	out := filepath.Join(t.TempDir(), "hf.json")
	run(repo, env, "", "hyperfine", "-N", "--warmup", "10", "--runs", "50",
		"--input", in, "--export-json", out, cmd)
	type stats struct {
		Min    float64 `json:"min"`
		Mean   float64 `json:"mean"`
		Median float64 `json:"median"`
	}
	var hf struct {
		Results []struct {
			stats
			Summary *struct {
				Wall stats `json:"time_wall_clock"`
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
	s := hf.Results[0].stats
	if hf.Results[0].Summary != nil { // hyperfine 2 nests them under summary
		s = hf.Results[0].Summary.Wall
	}
	return latency{Min: s.Min * 1000, Mean: s.Mean * 1000, Median: s.Median * 1000}
}

func TestRateGuardHookLatency(t *testing.T) {
	needTool(t, "hyperfine")
	slow(t)
	aval := avalBin(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	write(t, in, rateGuardBatch)
	l := hyperfineMs(t, isoEnv(dir), in, aval+" hook rate-guard")
	t.Logf("rate-guard hook: %s", l)
	isTrue(t, "the rate-guard hook runs in under 10 ms ("+l.String()+")", l.under(10))
}

func rateGuardHook(t *testing.T) (hookCmd, bool) {
	t.Helper()
	for _, g := range repoSettings(t).Hooks["PreToolUse"] {
		if g.Matcher != "Bash" {
			continue
		}
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "hook rate-guard") {
				return h, true
			}
		}
	}
	return hookCmd{}, false
}

func TestRateGuardWiring(t *testing.T) {
	h, ok := rateGuardHook(t)
	isTrue(t, "PreToolUse runs the rate-guard hook for Bash", ok)
	isTrue(t, "the rate-guard hook is synchronous (it must decide before the call)", !h.Async)
	eq(t, "the rate-guard hook timeout", 5.0, h.Timeout)
	mustHave(t, `^a="\$HOME/\.local/bin/aval"; \[ -x "\$a" \] \|\| a="\$\(command -v aval\)"; \[ -x "\$a" \] && "\$a" hook rate-guard \|\| true$`,
		h.Command, "the hook uses the same aval wrapper as lessons")
}

// No aval binary: the hook must be invisible, not an error on every Bash call.
func TestRateGuardHookWithNoBinaryIsASilentNoOp(t *testing.T) {
	testenv.SkipWindows(t, "runs the shell-form hook command through sh")
	needTool(t, "sh")
	h, ok := rateGuardHook(t)
	if !ok {
		t.Fatal("rate-guard hook is not wired")
	}
	home := t.TempDir()
	r := run(home, withEnv(isoEnv(home), "PATH=/usr/bin:/bin"), rateGuardBatch, "sh", "-c", h.Command)
	eq(t, "exit code with no aval binary", 0, r.Code)
	eq(t, "output with no aval binary", "", r.Out())
}

// The OpenCode port: a plugin that asks the same binary on every bash call.
func TestRateGuardOpenCodePlugin(t *testing.T) {
	needTool(t, "bun")
	plugin := p("config", "opencode", "plugin", "rate-guard.ts")
	isTrue(t, "the OpenCode plugin exists", exists(plugin))
	if !exists(plugin) {
		return
	}
	aval := avalBin(t)
	home := t.TempDir()
	src := read(t, "tests/testdata/opencode-rate-guard-probe.ts")
	src = regexp.MustCompile(`from "[^"]*rate-guard\.ts"`).ReplaceAllString(src, `from "`+filepath.ToSlash(plugin)+`"`)
	probe := filepath.Join(home, "probe", "opencode-rate-guard-probe.ts")
	write(t, probe, src)
	env := withEnv(isoEnv(home), "PATH="+filepath.Dir(aval)+string(os.PathListSeparator)+os.Getenv("PATH"))
	fire := func(tool, cmd string) string {
		return strings.TrimSpace(run(home, env, "", "bun", "run", probe, tool, cmd).Stdout)
	}
	eq(t, "a curl loop on innohelp.es is denied with the Helice reason",
		"deny Helice (innohelp.es / helice.app) bans the IP after rapid requests. Do not crawl it: use the Helice panel via Claude in Chrome, or make 1-2 spaced requests.",
		fire("bash", `for p in a b c; do curl -s https://www.innohelp.es/$p; done`))
	eq(t, "a single curl is allowed", "allow", fire("bash", `curl -sL https://www.innohelp.es/`))
	eq(t, "a non-bash tool is ignored", "allow", fire("edit", `for p in a b c; do curl -s https://www.innohelp.es/$p; done`))
	// A PATH without aval. On Windows the Unix stand-in would also hide the
	// system directories bun needs; the runner's own PATH has no aval either, as
	// the binary under test lives in a private directory.
	noAvalPath := "/usr/bin:/bin"
	if runtime.GOOS == "windows" {
		noAvalPath = os.Getenv("PATH")
	}
	noBin := withEnv(isoEnv(home), "PATH="+noAvalPath)
	eq(t, "with no aval binary the plugin fails open", "allow",
		strings.TrimSpace(run(home, noBin, "", "bun", "run", probe, "bash", `for p in a b c; do curl https://innohelp.es/$p; done`).Stdout))
}

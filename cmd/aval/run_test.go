package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// The in-process seam: every command reachable through run(args, io, env)
// with no child process. One real-binary smoke test per entry point lives in
// tests/; everything that only checks output or an exit code lives here.

// invocation is one in-process `aval ...`: what it printed and how it exited.
type invocation struct {
	code           int
	stdout, stderr string
}

// envMap is an env func over a fixed map: nothing leaks in from the shell
// running the tests.
func envMap(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

// withHomeVars adds what os.UserHomeDir, UserCacheDir and UserConfigDir read on
// Windows (USERPROFILE, LocalAppData, AppData) next to a HOME the test set, so
// the same map isolates the home on every OS. Keys the test set stay as they are.
func withHomeVars(env map[string]string) map[string]string {
	home, ok := env["HOME"]
	if !ok {
		return env
	}
	out := make(map[string]string, len(env)+3)
	for _, kv := range testenv.HomeEnv(home) {
		k, v, _ := strings.Cut(kv, "=")
		out[k] = v
	}
	for k, v := range env {
		out[k] = v
	}
	return out
}

func runIn(t *testing.T, env map[string]string, stdin string, args ...string) invocation {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb, envMap(withHomeVars(env)))
	return invocation{code, out.String(), errb.String()}
}

func TestRunVersionInProcess(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		r := runIn(t, nil, "", args...)
		if r.code != 0 {
			t.Fatalf("%v: exit %d, stderr %q", args, r.code, r.stderr)
		}
		want := currentBuild().text(runtime.GOOS, runtime.GOARCH)
		if r.stdout != want {
			t.Errorf("%v: stdout %q, want %q", args, r.stdout, want)
		}
	}
}

const rateGuardBatch = `{"session_id":"rg","tool_name":"Bash","tool_input":{"command":"for p in a b c; do curl -s https://www.innohelp.es/$p; done"}}`

// Moved from tests/rateguard_test.go: the deny JSON for a batch, silence for
// a one-off, exit 0 for both.
func TestRunHookRateGuardInProcess(t *testing.T) {
	env := map[string]string{"HOME": t.TempDir()}
	r := runIn(t, env, rateGuardBatch, "hook", "rate-guard")
	if r.code != 0 {
		t.Fatalf("exit code on deny: %d", r.code)
	}
	var v struct {
		H struct {
			D string `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
	}
	_ = json.Unmarshal([]byte(r.stdout), &v)
	if v.H.D != "deny" {
		t.Errorf("a curl loop on innohelp.es: decision %q, want deny (stdout %q)", v.H.D, r.stdout)
	}

	one := `{"tool_name":"Bash","tool_input":{"command":"curl -sL https://www.innohelp.es/"}}`
	r = runIn(t, env, one, "hook", "rate-guard")
	if r.code != 0 || r.stdout+r.stderr != "" {
		t.Errorf("a single curl: exit %d, output %q, want 0 and nothing", r.code, r.stdout+r.stderr)
	}
}

// procIO is the process's own environment with test streams, for the
// dispatch-level tests that set their env with t.Setenv.
func procIO(in io.Reader, out io.Writer) cmdIO {
	return cmdIO{stdin: in, stdout: out, stderr: io.Discard, env: os.Getenv}
}

// The exit codes main had before the seam: flag.ExitOnError's 0 for -h and 2
// for a bad flag (usage on stderr), 1 for an error ("error: ..." on stderr),
// and 1 with no extra line when the checks already reported themselves.
func TestRunExitCodes(t *testing.T) {
	home := t.TempDir()
	env := map[string]string{"HOME": home, "XDG_CONFIG_HOME": home}
	cases := []struct {
		name       string
		args       []string
		code       int
		stdout     string // substring; "" means empty
		stderr     string // substring; "" means empty
		stderrNone string // must not appear on stderr
	}{
		{"help flag", []string{"-h"}, 0, "", "Usage:", ""},
		{"bad global flag", []string{"--nope"}, 2, "", "flag provided but not defined: -nope", ""},
		{"help command", []string{"help"}, 0, "Usage:", "", ""},
		{"unknown command", []string{"instal"}, 1, "", `error: unknown command "instal"`, ""},
		{"failing gate reports once", []string{"design", "gate", "contrast", "#777777", "#888888"}, 1, "FAIL", "", "error:"},
		{"unknown hook", []string{"hook", "nope"}, 1, "", "error: usage: aval hook", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := runIn(t, env, "", c.args...)
			if r.code != c.code {
				t.Errorf("exit %d, want %d (stderr %q)", r.code, c.code, r.stderr)
			}
			check := func(stream, got, want string) {
				if want == "" && got != "" {
					t.Errorf("%s: %q, want nothing", stream, got)
				}
				if want != "" && !strings.Contains(got, want) {
					t.Errorf("%s: %q, want it to contain %q", stream, got, want)
				}
			}
			check("stdout", r.stdout, c.stdout)
			check("stderr", r.stderr, c.stderr)
			if c.stderrNone != "" && strings.Contains(r.stderr, c.stderrNone) {
				t.Errorf("stderr %q must not contain %q", r.stderr, c.stderrNone)
			}
		})
	}
}

// Two calls in one process share nothing: the flag set is per call, so a
// flag from the first never leaks into the second.
func TestRunCallsAreIndependent(t *testing.T) {
	first := runIn(t, nil, "", "--theme", "nord", "--version")
	second := runIn(t, nil, "", "version")
	if first.code != 0 || second.code != 0 || first.stdout != second.stdout {
		t.Errorf("first %+v, second %+v", first, second)
	}
}

// isoHome is an environment confined to a fresh HOME, as tests/isoEnv gives a
// child process: nothing reaches the real home, config or cache.
func isoHome(t *testing.T) (map[string]string, string) {
	t.Helper()
	home := t.TempDir()
	return map[string]string{
		"HOME": home, "USERPROFILE": home,
		"XDG_CONFIG_HOME":      filepath.Join(home, ".config"),
		"XDG_CACHE_HOME":       filepath.Join(home, ".cache"),
		"XDG_STATE_HOME":       filepath.Join(home, ".state"),
		"AVAL_NO_UPDATE_CHECK": "1",
	}, home
}

// A bare `aval` is a question, not an order: it prints the usage and leaves
// HOME untouched. Moved from tests/install_test.go. (Unknown commands, also
// moved from there, are a case of TestRunExitCodes.)
func TestBareAvalPrintsUsageAndChangesNothing(t *testing.T) {
	env, home := isoHome(t)
	r := runIn(t, env, "")
	if r.code != 0 {
		t.Fatalf("exit %d: %s%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "Usage:") || !strings.Contains(r.stdout, "aval install") {
		t.Errorf("no usage on stdout:\n%s%s", r.stdout, r.stderr)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("bare aval wrote to HOME: %v", entries)
	}
}

// The pre-v0.2 form keeps working for provisioning scripts, with a notice.
// Moved from tests/install_test.go.
func TestLegacyFlagsStillInstallWithNotice(t *testing.T) {
	env, _ := isoHome(t)
	r := runIn(t, env, "", "--yes", "--dry-run", "--repo", repoRoot(t))
	if r.code != 0 {
		t.Fatalf("exit %d: %s%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stderr, "aval install") || !strings.Contains(r.stderr, "deprecated") {
		t.Errorf("no deprecation notice on stderr:\n%s", r.stderr)
	}
	if !strings.Contains(r.stdout, "~/") {
		t.Errorf("legacy dry run did not show the plan:\n%s", r.stdout)
	}
}

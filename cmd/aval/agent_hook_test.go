package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// q is a JSON string literal: Windows paths hold backslashes, so a path cannot
// be pasted between quotes.
func q(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestRunHookLessonsUsesEnvDirsAndXDGState(t *testing.T) {
	dir, state, home := t.TempDir(), t.TempDir(), t.TempDir()
	testenv.SetHome(t, home)
	t.Setenv("AVAL_LESSONS_DIR", dir)
	t.Setenv("XDG_STATE_HOME", state)
	writeFile(t, filepath.Join(dir, "commit.md"), "---\ncommands:\n  - 'git commit'\n---\nNo trailers.\n")

	var out bytes.Buffer
	in := `{"session_id":"abc","tool_name":"Bash","tool_input":{"command":"git commit -m x"}}`
	if err := runHook([]string{"lessons"}, procIO(strings.NewReader(in), &out)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No trailers.") {
		t.Errorf("output = %q, want the lesson", out.String())
	}
	if !fileExists(filepath.Join(state, "aval", "lessons", "abc.seen")) {
		t.Error("dedupe state must live under $XDG_STATE_HOME/aval/lessons")
	}
}

func TestRunHookLessonsDefaultsToHomeClaudeLessons(t *testing.T) {
	home := t.TempDir()
	testenv.SetHome(t, home)
	t.Setenv("AVAL_LESSONS_DIR", "")
	t.Setenv("XDG_STATE_HOME", "")
	writeFile(t, filepath.Join(home, ".claude", "lessons", "x.md"), "---\ntriggers:\n  - '**/*.tsx'\n---\nHi.\n")
	var out bytes.Buffer
	in := `{"session_id":"s","tool_name":"Edit","tool_input":{"file_path":"/p/a.tsx"}}`
	if err := runHook([]string{"lessons"}, procIO(strings.NewReader(in), &out)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Hi.") {
		t.Errorf("output = %q", out.String())
	}
	if !fileExists(filepath.Join(home, ".local", "state", "aval", "lessons", "s.seen")) {
		t.Error("default state dir should be ~/.local/state/aval/lessons")
	}
}

func TestRunHookUnknownHookIsAnError(t *testing.T) {
	if err := runHook([]string{"nope"}, procIO(strings.NewReader(""), &bytes.Buffer{})); err == nil {
		t.Error("an unknown hook name must be reported")
	}
}

func TestRunHookDesignGateReadsEnvAndDenies(t *testing.T) {
	cache, proj := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("DESIGN_PIPELINE_OFF", "")
	t.Setenv("DESIGN_ACK_TTL", "")
	in := `{"session_id":"s","cwd":` + q(proj) + `,"tool_name":"Edit","tool_input":{"file_path":` + q(filepath.Join(proj, "Button.tsx")) + `}}`

	var out bytes.Buffer
	if err := runHook([]string{"design-gate"}, procIO(strings.NewReader(in), &out)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"permissionDecision":"deny"`) || !strings.Contains(out.String(), "`aval design ack <surface>`") {
		t.Errorf("want a deny naming `aval design ack <surface>`, got %q", out.String())
	}

	t.Setenv("DESIGN_PIPELINE_OFF", "1")
	out.Reset()
	if err := runHook([]string{"design-gate"}, procIO(strings.NewReader(in), &out)); err != nil || out.Len() != 0 {
		t.Errorf("DESIGN_PIPELINE_OFF must silence the gate, got %q err=%v", out.String(), err)
	}
}

func TestRunHookDesignGateAllowsAfterAck(t *testing.T) {
	cache, proj := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("DESIGN_PIPELINE_OFF", "")
	t.Setenv("DESIGN_ACK_TTL", "")
	if err := runDesign([]string{"ack", "pricing"}, proj, &bytes.Buffer{}, os.Getenv); err != nil {
		t.Fatal(err)
	}
	in := `{"session_id":"s","cwd":` + q(proj) + `,"tool_name":"Write","tool_input":{"file_path":` + q(filepath.Join(proj, "app.css")) + `}}`
	var out bytes.Buffer
	if err := runHook([]string{"design-gate"}, procIO(strings.NewReader(in), &out)); err != nil || out.Len() != 0 {
		t.Errorf("an acked project must be allowed, got %q err=%v", out.String(), err)
	}
	if !fileExists(filepath.Join(cache, "design-gate", "ui-edited.s")) {
		t.Error("an allowed UI edit must stamp the session")
	}
}

func TestRunHookDesignShotStopUsesCache(t *testing.T) {
	cache, proj := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("DESIGN_PIPELINE_OFF", "")
	writeFile(t, filepath.Join(cache, "design-gate", "ui-edited.s9"), proj)
	var out bytes.Buffer
	if err := runHook([]string{"design-shot-stop"}, procIO(strings.NewReader(`{"session_id":"s9"}`), &out)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"decision":"block"`) {
		t.Errorf("want a block, got %q", out.String())
	}
}

func TestRunHookRtkRewriteWithoutRtkIsASilentNoOp(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var out bytes.Buffer
	if err := runHook([]string{"rtk-rewrite"}, procIO(strings.NewReader(`{"tool_input":{"command":"git status"}}`), &out)); err != nil || out.Len() != 0 {
		t.Errorf("no rtk on PATH must pass through, got %q err=%v", out.String(), err)
	}
}

func rateGuardEnv(t *testing.T) string {
	t.Helper()
	cfg := t.TempDir()
	testenv.SetHome(t, t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("AVAL_RATE_GUARD_HOSTS", "")
	return cfg
}

func rateGuard(t *testing.T, in string) string {
	t.Helper()
	var out bytes.Buffer
	if err := runHook([]string{"rate-guard"}, procIO(strings.NewReader(in), &out)); err != nil {
		t.Fatalf("rate-guard must exit 0, got %v", err)
	}
	return out.String()
}

func bashPayload(cmd string) string {
	return `{"session_id":"s","tool_name":"Bash","tool_input":{"command":` + q(cmd) + `}}`
}

func TestRunHookRateGuardDeniesABatchWithTheExactJSON(t *testing.T) {
	rateGuardEnv(t)
	got := rateGuard(t, bashPayload(`for p in a b c; do curl -s "https://www.innohelp.es/$p"; done`))
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Helice (innohelp.es / helice.app) bans the IP after rapid requests. Do not crawl it: use the Helice panel via Claude in Chrome, or make 1-2 spaced requests."}}` + "\n"
	if got != want {
		t.Errorf("output:\n  got  %q\n  want %q", got, want)
	}
}

func TestRunHookRateGuardAllowsWithNoOutput(t *testing.T) {
	rateGuardEnv(t)
	for _, cmd := range []string{`curl -sL https://www.innohelp.es/`, `for p in a b; do curl https://example.com/$p; done`, `echo innohelp.es`} {
		if got := rateGuard(t, bashPayload(cmd)); got != "" {
			t.Errorf("%s: output = %q, want none", cmd, got)
		}
	}
}

func TestRunHookRateGuardReadsTheHostsFileAndEnv(t *testing.T) {
	cfg := rateGuardEnv(t)
	writeFile(t, filepath.Join(cfg, "aval", "rate-guard-hosts"), "# mine\nshop.test\n")
	if got := rateGuard(t, bashPayload(`for i in 1 2; do curl https://api.shop.test/$i; done`)); !strings.Contains(got, `"shop.test bans the IP`) {
		t.Errorf("a host from the file must be guarded and named, got %q", got)
	}
	t.Setenv("AVAL_RATE_GUARD_HOSTS", "env.test")
	if got := rateGuard(t, bashPayload(`for i in 1 2; do wget https://env.test/$i; done`)); !strings.Contains(got, `"deny"`) {
		t.Errorf("a host from AVAL_RATE_GUARD_HOSTS must be guarded, got %q", got)
	}
}

func TestRunHookRateGuardFailsOpen(t *testing.T) {
	cfg := rateGuardEnv(t)
	batch := `for p in a b c; do curl -s "https://www.innohelp.es/$p"; done`
	for name, in := range map[string]string{
		"bad JSON":      `{"tool_name":"Bash","tool_input":`,
		"non-Bash tool": `{"tool_name":"Write","tool_input":{"command":` + q(batch) + `}}`,
	} {
		if got := rateGuard(t, in); got != "" {
			t.Errorf("%s: output = %q, want none", name, got)
		}
	}
	// A directory where the file should be: readable as nothing.
	if err := os.MkdirAll(filepath.Join(cfg, "aval", "rate-guard-hosts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := rateGuard(t, bashPayload(batch)); got != "" {
		t.Errorf("unreadable hosts file: output = %q, want none", got)
	}
}

// Behaviour against the real lessons in config/claude/lessons, in process.
// Moved from tests/lessons_test.go, which keeps one real-binary smoke call.
func TestLessonsInjectionAgainstTheRealLessons(t *testing.T) {
	env := map[string]string{
		"HOME":             t.TempDir(),
		"AVAL_LESSONS_DIR": filepath.Join(repoRoot(t), "config", "claude", "lessons"),
	}
	fire := func(payload string) string {
		r := runIn(t, env, payload, "hook", "lessons")
		var v struct {
			H struct {
				C string `json:"additionalContext"`
			} `json:"hookSpecificOutput"`
		}
		_ = json.Unmarshal([]byte(r.stdout), &v)
		return v.H.C
	}
	cases := []struct{ what, payload, want string }{
		{"editing a .tsx file injects the screenshot lesson", `{"session_id":"u1","tool_name":"Edit","tool_input":{"file_path":"/p/web/src/Card.tsx"}}`, `screenshot|shot.mjs`},
		{"editing a .tsx file injects the native-elements lesson", `{"session_id":"u2","tool_name":"Write","tool_input":{"file_path":"/p/web/src/Card.tsx"}}`, `native`},
		{"editing a Dockerfile injects the Kiri lesson", `{"session_id":"d1","tool_name":"Edit","tool_input":{"file_path":"/p/api/Dockerfile"}}`, `Kiri`},
		{"running docker injects the Kiri lesson", `{"session_id":"d2","tool_name":"Bash","tool_input":{"command":"docker compose up -d"}}`, `Kiri`},
		{"editing Go code injects the strict-TDD lesson", `{"session_id":"g1","tool_name":"Edit","tool_input":{"file_path":"/p/main.go"}}`, `tdd-gate`},
		{"committing injects the attribution lesson", `{"session_id":"c1","tool_name":"Bash","tool_input":{"command":"git commit -m \"feat: x\""}}`, `Co-Authored-By`},
	}
	for _, c := range cases {
		if got := fire(c.payload); !regexp.MustCompile(c.want).MatchString(got) {
			t.Errorf("%s: /%s/ not found in %q", c.what, c.want, got)
		}
	}
	if got := fire(`{"session_id":"n1","tool_name":"Edit","tool_input":{"file_path":"/p/notes.md"}}`); got != "" {
		t.Errorf("editing prose injects nothing: got %q", got)
	}
	if got := fire(`{"session_id":"d1","tool_name":"Edit","tool_input":{"file_path":"/p/api/Dockerfile"}}`); got != "" {
		t.Errorf("a lesson shows once per session: got %q", got)
	}
}

func TestRunHookFmtFormatsTheEditedFile(t *testing.T) {
	if !fileExists(gofmtPath(t)) {
		t.Skip("gofmt not installed")
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	file := filepath.Join(root, "x.go")
	writeFile(t, file, "package x\nfunc  F( ) {\n}\n")
	in := `{"hook_event_name":"PostToolUse","tool_name":"Edit","cwd":` + q(root) + `,"tool_input":{"file_path":` + q(file) + `}}`
	var out bytes.Buffer
	if err := runHook([]string{"fmt"}, procIO(strings.NewReader(in), &out)); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(file)
	if want := "package x\n\nfunc F() {\n}\n"; string(got) != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	if out.Len() != 0 {
		t.Errorf("the fmt hook prints nothing on stdout, got %q", out.String())
	}
}

func gofmtPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("gofmt")
	if err != nil {
		return ""
	}
	return p
}

func guardHook(t *testing.T, in string) string {
	t.Helper()
	var out bytes.Buffer
	if err := runHook([]string{"guard"}, procIO(strings.NewReader(in), &out)); err != nil {
		t.Fatalf("guard must exit 0, got %v", err)
	}
	return out.String()
}

func guardPayload(cwd, cmd string) string {
	return `{"session_id":"s","cwd":` + q(cwd) + `,"tool_name":"Bash","tool_input":{"command":` + q(cmd) + `}}`
}

func TestRunHookGuardDeniesWithTheExactJSON(t *testing.T) {
	testenv.SetHome(t, t.TempDir())
	t.Setenv("AVAL_PRIVATE_DIR", t.TempDir())
	got := guardHook(t, guardPayload(t.TempDir(), "rm -rf /"))
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"aval guard blocked this command (rm-outside-roots: rm -rf outside the project and home, or on /, ~ or the home folder). Command: rm -rf /. If you really mean it, add the full command as one line to ~/.config/aval/private/guard.allow (a private file, never committed)."}}` + "\n"
	if got != want {
		t.Errorf("output:\n  got  %q\n  want %q", got, want)
	}
}

func TestRunHookGuardAllowsCommonWorkWithNoOutput(t *testing.T) {
	testenv.SetHome(t, t.TempDir())
	t.Setenv("AVAL_PRIVATE_DIR", t.TempDir())
	cwd := t.TempDir()
	for _, cmd := range []string{"go test ./...", "git push origin feature", "rm -rf ./dist", "git status"} {
		if got := guardHook(t, guardPayload(cwd, cmd)); got != "" {
			t.Errorf("%s: output = %q, want none", cmd, got)
		}
	}
}

func TestRunHookGuardReadsTheAllowListFromThePrivateOverlay(t *testing.T) {
	testenv.SetHome(t, t.TempDir())
	priv := t.TempDir()
	t.Setenv("AVAL_PRIVATE_DIR", priv)
	cwd := t.TempDir()
	writeFile(t, filepath.Join(priv, "guard.allow"), "# mine\ndocker system prune -af\n")
	if got := guardHook(t, guardPayload(cwd, "docker system prune -af")); got != "" {
		t.Errorf("an allow-listed command was denied: %q", got)
	}
	if got := guardHook(t, guardPayload(cwd, "docker system prune")); !strings.Contains(got, `"deny"`) {
		t.Errorf("a similar command must stay denied, got %q", got)
	}
}

func TestRunHookGuardFailsOpen(t *testing.T) {
	testenv.SetHome(t, t.TempDir())
	t.Setenv("AVAL_PRIVATE_DIR", t.TempDir())
	for name, in := range map[string]string{
		"bad JSON":      `{"tool_name":"Bash","tool_input":`,
		"non-Bash tool": `{"tool_name":"Write","tool_input":{"command":"rm -rf /"}}`,
	} {
		if got := guardHook(t, in); got != "" {
			t.Errorf("%s: output = %q, want none", name, got)
		}
	}
}

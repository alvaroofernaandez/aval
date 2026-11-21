// Package tests holds the repository-level checks: the ones that read the
// versioned configs, skills and docs as a whole, and the end-to-end cases that
// drive the real aval binary. Unit tests live next to the code they cover in
// internal/ and cmd/.
//
// The binary is built once per `go test` run (lazily, so a run that only
// reads files never pays for it). Tests that need an external tool skip when
// it is absent; tests that touch $HOME use a temporary one.
//
// AVAL_TEST_REPO points the suite at another checkout (or a mutated copy of
// this one), which is how the guards are shown to fail when what they protect
// is broken.
package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// exeSuffix is the executable extension of the platform the suite runs on.
var exeSuffix = func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()

var (
	repo string

	buildOnce sync.Once
	binDir    string
	avalPath  string
	buildErr  error
)

func TestMain(m *testing.M) {
	var err error
	repo, err = findRepo()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	code := m.Run()
	for _, d := range []string{binDir, brandDir} {
		if d != "" {
			_ = os.RemoveAll(d)
		}
	}
	os.Exit(code)
}

func findRepo() (string, error) {
	if r := os.Getenv("AVAL_TEST_REPO"); r != "" {
		return filepath.Abs(r)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// go test runs with the package directory (<repo>/tests) as cwd.
	return filepath.Dir(wd), nil
}

// avalBin returns the path of the aval binary built from the repo under test.
// It skips when there is no Go toolchain and fails when the build is broken.
func avalBin(t testing.TB) string {
	t.Helper()
	buildOnce.Do(func() {
		if _, err := exec.LookPath("go"); err != nil {
			buildErr = errors.New("no go toolchain")
			return
		}
		binDir, buildErr = os.MkdirTemp("", "aval-tests-bin-")
		if buildErr != nil {
			return
		}
		avalPath = filepath.Join(binDir, "aval"+exeSuffix)
		cmd := exec.Command("go", "build", "-o", avalPath, "./cmd/aval")
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("go build ./cmd/aval: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		if buildErr.Error() == "no go toolchain" {
			t.Skip("go toolchain not installed")
		}
		t.Fatal(buildErr)
	}
	return avalPath
}

// needTool skips the test when any of the tools is missing from PATH.
func needTool(t testing.TB, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
}

// slow skips under `go test -short`.
func slow(t testing.TB) {
	t.Helper()
	if testing.Short() {
		t.Skip("slow end-to-end case; skipped with -short")
	}
}

// needGit skips when the repo under test is not a git checkout (a tarball,
// a source archive) or git itself is missing.
func needGit(t testing.TB) {
	t.Helper()
	needTool(t, "git")
	if r := run(repo, nil, "", "git", "rev-parse", "--git-dir"); r.Code != 0 {
		t.Skip("not a git checkout")
	}
}

// --- running commands ---------------------------------------------------------

type result struct {
	Stdout, Stderr string
	Code           int
}

func (r result) Out() string { return r.Stdout + r.Stderr }

// run executes a command. env == nil inherits the test process environment.
func run(dir string, env []string, stdin, name string, args ...string) result {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = 127
			se.WriteString(err.Error())
		}
	}
	return result{so.String(), se.String(), code}
}

// withEnv returns base with the KEY=VALUE pairs overriding, and a bare KEY
// removing, any earlier value.
func withEnv(base []string, kv ...string) []string {
	drop := map[string]bool{}
	for _, p := range kv {
		drop[strings.SplitN(p, "=", 2)[0]] = true
	}
	var out []string
	for _, e := range base {
		if !drop[strings.SplitN(e, "=", 2)[0]] {
			out = append(out, e)
		}
	}
	for _, p := range kv {
		if strings.Contains(p, "=") {
			out = append(out, p)
		}
	}
	return out
}

// isoEnv is a process environment confined to home: nothing the suite does can
// reach the real $HOME or the real caches, and no AVAL_*/DESIGN_* switch from
// the developer's shell leaks in.
func isoEnv(home string, kv ...string) []string {
	var clean []string
	for _, e := range os.Environ() {
		k := strings.SplitN(e, "=", 2)[0]
		if strings.HasPrefix(k, "AVAL_") || strings.HasPrefix(k, "DESIGN_") ||
			strings.HasPrefix(k, "XDG_") || k == "TMUX" || k == "HOME" || k == "ZDOTDIR" ||
			strings.EqualFold(k, "USERPROFILE") || strings.EqualFold(k, "LocalAppData") || strings.EqualFold(k, "AppData") {
			continue
		}
		clean = append(clean, e)
	}
	base := append(clean,
		"HOME="+home,
		"USERPROFILE="+home, // os.UserHomeDir on Windows
		"LocalAppData="+filepath.Join(home, "AppData", "Local"),
		"AppData="+filepath.Join(home, "AppData", "Roaming"),
		"XDG_CACHE_HOME="+filepath.Join(home, ".cache"),
		"XDG_STATE_HOME="+filepath.Join(home, ".state"),
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	)
	return withEnv(base, kv...)
}

// git runs git in dir with a neutral identity and no global config.
func git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	env := withEnv(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	r := run(dir, env, "", "git", args...)
	if r.Code != 0 {
		t.Fatalf("git %v in %s: exit %d\n%s", args, dir, r.Code, r.Out())
	}
	return strings.TrimSpace(r.Stdout)
}

// --- files --------------------------------------------------------------------

func p(rel ...string) string { return filepath.Join(append([]string{repo}, rel...)...) }

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// read returns a repo file's text and fails the test when it is missing.
func read(t testing.TB, rel string) string {
	t.Helper()
	b, err := os.ReadFile(p(rel))
	if err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	return string(b)
}

func write(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readAbs(t testing.TB, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// subdirs lists the immediate subdirectory names of a repo directory.
func subdirs(t testing.TB, rel string) []string {
	t.Helper()
	ents, err := os.ReadDir(p(rel))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// --- matching -----------------------------------------------------------------

// has reports whether re matches anywhere in s, line-anchored like ripgrep.
func has(re, s string) bool { return regexp.MustCompile("(?m)" + re).MatchString(s) }

func count(re, s string) int { return len(regexp.MustCompile("(?m)"+re).FindAllString(s, -1)) }

func mustHave(t testing.TB, re, s, what string) {
	t.Helper()
	if !has(re, s) {
		t.Errorf("%s: /%s/ not found", what, re)
	}
}

func mustNotHave(t testing.TB, re, s, what string) {
	t.Helper()
	if m := regexp.MustCompile("(?m)" + re).FindString(s); m != "" {
		t.Errorf("%s: /%s/ matched %q", what, re, m)
	}
}

func mustContain(t testing.TB, s, sub, what string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("%s: %q not found", what, sub)
	}
}

func eq(t testing.TB, what string, want, got any) {
	t.Helper()
	if want != got {
		t.Errorf("%s:\n  want: %v\n  got:  %v", what, want, got)
	}
}

func isTrue(t testing.TB, what string, cond bool) {
	t.Helper()
	if !cond {
		t.Errorf("%s", what)
	}
}

// block returns the lines between two exact marker lines (exclusive).
func block(s, begin, end string) string {
	var out []string
	in := false
	for _, l := range strings.Split(s, "\n") {
		switch {
		case l == begin:
			in = true
		case l == end:
			in = false
		case in:
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

const (
	rulesBegin = "<!-- shared:agent-rules -->"
	rulesEnd   = "<!-- /shared:agent-rules -->"
)

func sharedBlock(s string) string { return block(s, rulesBegin, rulesEnd) }

// --- settings.json ------------------------------------------------------------

type hookCmd struct {
	Command string  `json:"command"`
	Async   bool    `json:"async"`
	Timeout float64 `json:"timeout"`
}

type hookGroup struct {
	Matcher string    `json:"matcher"`
	Hooks   []hookCmd `json:"hooks"`
}

type settingsDoc struct {
	Hooks          map[string][]hookGroup `json:"hooks"`
	EnabledPlugins map[string]any         `json:"enabledPlugins"`
	Attribution    struct {
		Commit *string `json:"commit"`
		PR     *string `json:"pr"`
	} `json:"attribution"`
}

// allHooks flattens every hook command with its event.
func (s settingsDoc) allHooks() (events, commands []string) {
	var evs []string
	for ev := range s.Hooks {
		evs = append(evs, ev)
	}
	sort.Strings(evs)
	for _, ev := range evs {
		for _, g := range s.Hooks[ev] {
			for _, h := range g.Hooks {
				events = append(events, ev)
				commands = append(commands, h.Command)
			}
		}
	}
	return
}

func loadSettings(t testing.TB, path string) settingsDoc {
	t.Helper()
	var s settingsDoc
	if err := json.Unmarshal([]byte(readAbs(t, path)), &s); err != nil {
		t.Fatalf("%s is not valid JSON: %v", path, err)
	}
	return s
}

func repoSettings(t testing.TB) settingsDoc {
	return loadSettings(t, p("config", "claude", "settings.json"))
}

// regexpFind returns the first capture group (or whole match) of re in s.
func regexpFind(re, s string) string {
	m := regexp.MustCompile(re).FindStringSubmatch(s)
	switch len(m) {
	case 0:
		return ""
	case 1:
		return m[0]
	}
	return m[1]
}

func itoa(n int) string { return strconv.Itoa(n) }

// withHookTools is a PATH= entry that adds stand-ins for the programs the
// repo's own hooks call (gentle-ai, code-review-graph). Every sync ends with
// the doctor's hooks-resolve check, which rolls back on a machine, like a CI
// runner, that lacks them. Copies of the aval binary resolve exactly like the
// real tools and are never run by these tests.
func withHookTools(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	bin, err := os.ReadFile(avalBin(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"gentle-ai", "code-review-graph"} {
		if err := os.WriteFile(filepath.Join(dir, name+exeSuffix), bin, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return "PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")
}

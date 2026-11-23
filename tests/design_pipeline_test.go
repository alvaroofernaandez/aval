package tests

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The design-pipeline gate (`aval hook design-gate`), its marker
// (`aval design ack`) and the Stop hook (`aval hook design-shot-stop`),
// exercised through the real binary.
//
// The decision tables are tested in process in cmd/aval/design_pipeline_test.go
// (through run) and unit-tested in internal/design/hook and internal/design/ack.
// What only a real process proves stays here, one smoke test per entry point:
// argv, exit codes, a binary that runs with an empty PATH, and the git
// toplevel found from a real working directory.

type dgEnv struct {
	t     *testing.T
	root  string // temp root; every project lives under it
	cache string
	aval  string
}

func newDG(t *testing.T) *dgEnv {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &dgEnv{t: t, root: root, cache: filepath.Join(root, "cache"), aval: avalBin(t)}
}

func (d *dgEnv) env(kv ...string) []string {
	return isoEnv(d.root, append([]string{"XDG_CACHE_HOME=" + d.cache}, kv...)...)
}

func (d *dgEnv) project(name string, designMD string) string {
	d.t.Helper()
	dir := filepath.Join(d.root, name)
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		d.t.Fatal(err)
	}
	if designMD != "" {
		write(d.t, filepath.Join(dir, ".agents", "DESIGN.md"), designMD)
	}
	return dir
}

func sha1Of(s string) string { h := sha1.Sum([]byte(s)); return hex.EncodeToString(h[:]) }

// marker is computed independently of the code under test.
func (d *dgEnv) marker(project string) string {
	real, err := filepath.EvalSymlinks(project)
	if err != nil {
		d.t.Fatal(err)
	}
	return filepath.Join(d.cache, "design-gate", sha1Of(real))
}

func (d *dgEnv) ack(project, surface string, kv ...string) result {
	args := []string{"design", "ack"}
	if surface != "" {
		args = append(args, surface)
	}
	return run(project, d.env(kv...), "", d.aval, args...)
}

func (d *dgEnv) rewriteEpoch(marker string, epoch int64) {
	b := readAbs(d.t, marker)
	b = regexp.MustCompile(`(?m)^epoch=.*$`).ReplaceAllString(b, "epoch="+strconv.FormatInt(epoch, 10))
	write(d.t, marker, b)
}

// stopDecision reads the Stop hook's decision ("none" when silent); the
// installed-hooks e2e in hooks_e2e_test.go uses it.
func stopDecision(out string) string {
	if strings.TrimSpace(out) == "" {
		return "none"
	}
	var v struct {
		Decision string `json:"decision"`
	}
	if json.Unmarshal([]byte(out), &v) != nil || v.Decision == "" {
		return "none"
	}
	return v.Decision
}

// Smoke for `aval design ack`. The project root is the git toplevel of cwd,
// not cwd itself.
func TestDesignAckFromASubdirectoryMarksTheGitToplevel(t *testing.T) {
	needTool(t, "git")
	d := newDG(t)
	p2 := d.project("p2", "")
	deep := filepath.Join(p2, "src", "deep")
	_ = os.MkdirAll(deep, 0o755)
	git(t, p2, "init", "-q")
	r := d.ack(deep, "card")
	eq(t, "design-ack exit code", 0, r.Code)
	top := git(t, p2, "rev-parse", "--show-toplevel")
	isTrue(t, "design-ack from a subdirectory marks the git toplevel", exists(d.marker(top)))
}

// Smoke for `aval hook design-gate`. A non-UI edit needs nothing but the
// binary: no jq, no git, no hash tool.
func TestDesignGateWhatIsNotADesignSurface(t *testing.T) {
	d := newDG(t)
	p5 := d.project("p5", "")
	body := `{"tool_name":"Edit","tool_input":{"file_path":"` + p5 + `/main.go"},"cwd":"/x"}`
	r := run(p5, withEnv(d.env(), "PATH=/nonexistent"), body, d.aval, "hook", "design-gate")
	eq(t, "a non-UI edit runs with an empty PATH (exit)", 0, r.Code)
	eq(t, "...and prints nothing", "", r.Out())
}

// Smoke for `aval hook design-shot-stop`. The common case (no UI edit this
// session) needs nothing but the binary.
func TestDesignShotStopMalformedInputIsSilent(t *testing.T) {
	d := newDG(t)
	r := run(d.root, withEnv(d.env(), "PATH=/nonexistent"), `{"session_id":"s-none","stop_hook_active":false}`, d.aval, "hook", "design-shot-stop")
	eq(t, "the no-edit path runs with an empty PATH (exit)", 0, r.Code)
	eq(t, "...and prints nothing", "", strings.TrimSpace(r.Out()))
}

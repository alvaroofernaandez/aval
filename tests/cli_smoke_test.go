package tests

import (
	"strings"
	"testing"
)

// One real-binary smoke test per entry point whose behaviour is tested in
// process in cmd/aval (through run) and has no other real-process test here.
// What only the built binary proves: argv reaches run, the exit code reaches
// the shell, and the hot paths start with a broken config on disk. Output is
// asserted in process; here only the code and one marker of the right path.
//
// Entry points smoke-tested elsewhere in this package: hook design-gate,
// hook design-shot-stop and design ack (design_pipeline_test.go), hook
// rate-guard (rateguard_test.go), hook lessons (lessons_test.go), hook
// pre-push (pre_push_test.go), rules sync (rules_sync_test.go), install
// (install_test.go, manifest_test.go), dev (devmode_test.go) and tmux launch
// (launch_test.go).
func TestCLIBinarySmoke(t *testing.T) {
	aval := avalBin(t)
	cases := []struct {
		name   string
		args   []string
		stdin  string
		code   int
		stdout string // substring of stdout; "" means not checked
		stderr string // substring of stderr; "" means not checked
	}{
		{"bare aval on a pipe prints the usage", nil, "", 0, "Usage:", ""},
		{"unknown command", []string{"instal"}, "", 1, "", `unknown command "instal"`},
		{"statusline with a broken config", []string{"statusline"}, "{}", 0, "", ""},
		{"hook plan-gate with a broken config", []string{"hook", "plan-gate"}, "{}", 0, "", ""},
		{"hook fmt with an empty payload", []string{"hook", "fmt"}, "{}", 0, "", ""},
		{"doctor rejects a bad --theme", []string{"--theme", "tokio-night", "doctor"}, "", 1, "", "tokyo-night"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			write(t, home+"/.config/aval/config", "theme = tokio-night\nmotion = sometimes\n")
			r := run(t.TempDir(), isoEnv(home, "AVAL_NO_UPDATE_CHECK=1"), c.stdin, aval, c.args...)
			eq(t, "exit code\n"+r.Out(), c.code, r.Code)
			if c.stdout != "" && !strings.Contains(r.Stdout, c.stdout) {
				t.Errorf("stdout %q does not contain %q", r.Stdout, c.stdout)
			}
			if c.stderr != "" && !strings.Contains(r.Stderr, c.stderr) {
				t.Errorf("stderr %q does not contain %q", r.Stderr, c.stderr)
			}
		})
	}
}

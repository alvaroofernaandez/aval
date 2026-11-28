package guard

import (
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func payload(tool, cmd string) string {
	return `{"session_id":"s","cwd":"/work/proj","tool_name":"` + tool + `","tool_input":{"command":` + quote(cmd) + `}}`
}

func quote(s string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"` }

func runHook(in string, o RunOptions) string {
	var out strings.Builder
	Run(strings.NewReader(in), &out, o)
	return out.String()
}

func opts(files map[string]string) RunOptions {
	return RunOptions{
		Home:   "/home/u",
		Getenv: func(string) string { return "" },
		ReadFile: func(p string) ([]byte, error) {
			if s, ok := files[filepath.ToSlash(p)]; ok {
				return []byte(s), nil
			}
			return nil, fs.ErrNotExist
		},
	}
}

func TestRunDenyIsTheExactHookJSON(t *testing.T) {
	got := runHook(payload("Bash", "rm -rf /"), opts(nil))
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":` +
		`"aval guard blocked this command (rm-outside-roots: rm -rf outside the project and home, or on /, ~ or the home folder). ` +
		`Command: rm -rf /. If you really mean it, add the full command as one line to ~/.config/aval/private/guard.allow (a private file, never committed)."}}` + "\n"
	if got != want {
		t.Errorf("output =\n%s\nwant\n%s", got, want)
	}
}

func TestRunUsesTheProjectDirectoryFromThePayload(t *testing.T) {
	if got := runHook(payload("Bash", "rm -rf ./build"), opts(nil)); got != "" {
		t.Errorf("inside the project must pass, got %q", got)
	}
	if got := runHook(payload("Bash", "rm -rf ../other"), opts(nil)); got == "" {
		t.Error("outside the project must be denied")
	}
}

func TestRunAllowListEntryAllowsAndOnlyThatCommand(t *testing.T) {
	files := map[string]string{"/home/u/.config/aval/private/guard.allow": "# mine\nrm -rf /opt/cache\n"}
	if got := runHook(payload("Bash", "rm -rf /opt/cache"), opts(files)); got != "" {
		t.Errorf("allow-listed command denied: %q", got)
	}
	if got := runHook(payload("Bash", "rm -rf /opt/cache2"), opts(files)); got == "" {
		t.Error("a similar command must stay denied")
	}
}

func TestRunReadsTheAllowListFromTheOverlayEnv(t *testing.T) {
	o := opts(map[string]string{"/priv/guard.allow": "rm -rf /opt/cache"})
	o.Getenv = func(k string) string {
		if k == "AVAL_PRIVATE_DIR" {
			return "/priv"
		}
		return ""
	}
	if got := runHook(payload("Bash", "rm -rf /opt/cache"), o); got != "" {
		t.Errorf("AVAL_PRIVATE_DIR overlay ignored: %q", got)
	}
}

func TestRunNeverTouchesTheDiskWhenNothingMatches(t *testing.T) {
	o := opts(nil)
	o.ReadFile = func(p string) ([]byte, error) { t.Fatalf("read %s on the no-op path", p); return nil, nil }
	for _, c := range []string{"ls", "go test ./...", "git push origin feature", "rm -rf ./dist", "git status && git diff"} {
		if got := runHook(payload("Bash", c), o); got != "" {
			t.Errorf("%q produced %q", c, got)
		}
	}
}

func TestRunFailsOpen(t *testing.T) {
	boom := opts(nil)
	boom.ReadFile = func(string) ([]byte, error) { panic("boom") }
	broken := opts(nil)
	broken.ReadFile = func(string) ([]byte, error) { return nil, errors.New("permission denied") }
	cases := []struct {
		name, in string
		o        RunOptions
		deny     bool
	}{
		{"bad JSON", `{"tool_name":`, opts(nil), false},
		{"empty input", ``, opts(nil), false},
		{"another tool", payload("Edit", "rm -rf /"), opts(nil), false},
		{"empty command", payload("Bash", ""), opts(nil), false},
		{"a panic while reading the allow-list", payload("Bash", "rm -rf /"), boom, false},
		{"an unreadable allow-list keeps the deny", payload("Bash", "rm -rf /"), broken, true},
	}
	for _, c := range cases {
		if got := runHook(c.in, c.o); (got != "") != c.deny {
			t.Errorf("%s: output %q, deny want %v", c.name, got, c.deny)
		}
	}
}

// The hook is a hot path: no exec, no network, no aval packages.
func TestGuardImportsOnlySafeStdlib(t *testing.T) {
	fset := token.NewFileSet()
	matches, _ := filepath.Glob("*.go")
	for _, f := range matches {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(strings.SplitN(p, "/", 2)[0], ".") || p == "os/exec" || strings.HasPrefix(p, "net") {
				t.Errorf("%s imports %s", f, p)
			}
		}
	}
}

func TestRunAllowsRmInsideTempDirs(t *testing.T) {
	o := opts(nil)
	o.Getenv = func(k string) string {
		if k == "TMPDIR" {
			return "/srv/scratch/"
		}
		return ""
	}
	for _, c := range []string{"rm -rf /tmp/x", "rm -rf /srv/scratch/job1"} {
		if got := runHook(payload("Bash", c), o); got != "" {
			t.Errorf("%q denied: %q", c, got)
		}
	}
	for _, c := range []string{"rm -rf /srv/scratch", "rm -rf /srv/scratch/../etc", "rm -rf /srv/other"} {
		if got := runHook(payload("Bash", c), o); got == "" {
			t.Errorf("%q must stay denied", c)
		}
	}
}

func TestRunDeniesRmThroughASymlinkOutOfTemp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	root := t.TempDir()
	if err := os.Symlink("/etc", filepath.Join(root, "link")); err != nil {
		t.Skip(err)
	}
	o := opts(nil)
	o.Getenv = func(k string) string {
		if k == "TMPDIR" {
			return root
		}
		return ""
	}
	target := filepath.ToSlash(filepath.Join(root, "link", "x"))
	if got := runHook(payload("Bash", "rm -rf "+target), o); got == "" {
		t.Errorf("rm through a symlink that leaves the temp dir passed")
	}
}

//go:build !windows

package e2e

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// These tests drive the real `aval` binary against real tmux servers.
//
// Why a package of its own rather than *_integration_test.go beside the unit
// tests in internal/tmux: TestMain here builds aval once, and in internal/tmux
// that build (and the tmux/yazi probing) would be paid by every `go test` of the
// unit suite, which is deliberately hermetic and fast. A separate package also
// keeps these tests on the public surface: they only see the binary and the
// config files, exactly as tmux, Ghostty and yazi do. (tests/ is not used: it
// belongs to the repo-level suites.)
//
// Isolation, in every test:
//   - one tmux server per test on its own `-L` socket, killed in t.Cleanup;
//   - a temp HOME laid out like an installed machine (see newSrv);
//   - a scrubbed environment, so the developer's own TMUX/TMUX_PANE never leak in;
//   - stand-ins for yazi, ya, nvim and an editor that are this very test binary
//     under another name (see fakeMain), so no shell script is needed.
//
// Overrides, mainly to prove the suite fails when the thing it guards breaks:
//
//	AVAL_BIN                an already built aval to test instead of building one
//	AVAL_E2E_CONFIG_ROOT    a directory laid out like the repo (home/tmux.conf,
//	                        config/yazi, config/yazi-sidebar) to test instead of it

var (
	repoRoot   string // the repository, where the aval source lives
	configRoot string // where home/ and config/ are read from (default repoRoot)
	avalBin    string // the aval binary under test
	fakeBin    string // directory of stand-in executables
	realTmux   string // absolute path of the tmux on this machine
)

// fakeNames are the executables fakeBin provides, all symlinks to this binary.
var fakeNames = []string{"yazi", "ya", "fakeedit", "nvim", "tmux-counted", "tmux-stub"}

func TestMain(m *testing.M) {
	if fakeMain() {
		return
	}
	flag.Parse()
	os.Exit(setupAndRun(m))
}

func setupAndRun(m *testing.M) int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	repoRoot = root
	configRoot = root
	if v := os.Getenv("AVAL_E2E_CONFIG_ROOT"); v != "" {
		configRoot = v
	}
	realTmux, _ = exec.LookPath("tmux")

	// -short skips every test that drives a process, so nothing is built; only
	// the pure-Go tests (the wait helpers, the static config checks) run.
	if testing.Short() {
		return m.Run()
	}

	tmp, err := os.MkdirTemp("", "aval-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	defer os.RemoveAll(tmp)

	{
		if v := os.Getenv("AVAL_BIN"); v != "" {
			avalBin = v
		} else {
			avalBin = filepath.Join(tmp, "aval")
			cmd := exec.Command("go", "build", "-o", avalBin, "./cmd/aval")
			cmd.Dir = repoRoot
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "e2e: go build failed: %v\n%s", err, out)
				return 1
			}
		}
		self, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "e2e:", err)
			return 1
		}
		fakeBin = filepath.Join(tmp, "fake")
		if err := os.MkdirAll(fakeBin, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "e2e:", err)
			return 1
		}
		for _, n := range fakeNames {
			// tmux reports a process by the name of the executable it resolved,
			// so a symlink would show up as this test binary. Only nvim is
			// matched by name (close-file treats vim-family editors specially),
			// so only nvim has to be a real file.
			link := os.Symlink
			if n == "nvim" {
				link = copyExecutable
			}
			if err := link(self, filepath.Join(fakeBin, n)); err != nil {
				fmt.Fprintln(os.Stderr, "e2e:", err)
				return 1
			}
		}
	}
	return m.Run()
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above the working directory")
		}
		dir = parent
	}
}

// fakeMain makes this test binary behave as a stand-in tool when it is run
// through one of the fakeNames symlinks. It reports whether it did.
//
// Every stand-in is the same compiled code, so the suites need no shell script,
// no C compiler (the old nvim double was built with cc) and nothing written to
// disk per test but a symlink. The tool writes its observations into
// $E2E_FAKE_DIR, which each test points at a directory of its own.
func fakeMain() bool {
	name := filepath.Base(os.Args[0])
	dir := os.Getenv("E2E_FAKE_DIR")
	args := os.Args[1:]
	switch name {
	case "yazi":
		// Named yazi so the toggle takes its real code path (client id, watcher).
		write(dir, "yazi-env.txt", orUnset(os.Getenv("YAZI_CONFIG_HOME")))
		write(dir, "yazi-args.txt", strings.Join(args, "\n")+"\n")
		sleepForever()
	case "ya":
		appendTo(dir, "ya-calls.txt", strings.Join(args, " ")+"\n")
	case "fakeedit":
		write(dir, "args.txt", strings.Join(args, "\n")+"\n")
		sleepForever()
	case "nvim":
		// tmux reports the process name, so being called nvim is the point.
		sleepForever()
	case "tmux-counted":
		appendTo(dir, "tmux-calls.txt", "x\n")
		bin := os.Getenv("E2E_REAL_TMUX")
		if err := syscall.Exec(bin, append([]string{bin}, args...), os.Environ()); err != nil {
			fmt.Fprintln(os.Stderr, "tmux-counted:", err)
			os.Exit(127)
		}
	case "tmux-stub":
		appendTo(dir, "tmux-stub.txt", strings.Join(args, " ")+"\n")
	default:
		return false
	}
	return true
}

func orUnset(s string) string {
	if s == "" {
		return "unset"
	}
	return s
}

func write(dir, name, s string) {
	if dir != "" {
		_ = os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644)
	}
}

func appendTo(dir, name, s string) {
	if dir == "" {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(s)
}

func sleepForever() {
	for {
		time.Sleep(time.Hour)
	}
}

func copyExecutable(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o755)
}

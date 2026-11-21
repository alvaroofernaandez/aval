// Package tmux holds the helpers tmux, Ghostty, yazi and the zsh prompt call:
// sidebar toggle/follow/watch, file open/close, the cmd+v router, the status
// bar and the Ghostty launcher.
//
// Everything here runs with launchd's PATH (/usr/bin:/bin:/usr/sbin:/sbin) and
// no rc files, so every external binary is resolved by absolute path and a
// failure never leaves a pane or a window dead without a reason.
package tmux

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Runner runs one tmux command against a server and returns its stdout.
type Runner interface {
	Run(args ...string) (string, error)   // tmux's own stderr is passed through
	Query(args ...string) (string, error) // a probe: stderr is discarded
}

// Env is every side effect the helpers have, injectable so tests need neither
// a live tmux server, nor a real /proc, nor a clipboard.
type Env struct {
	Getenv     func(string) string
	LookupEnv  func(string) (string, bool)
	IsExec     func(path string) bool
	IsDir      func(path string) bool
	LookPath   func(name string) (string, error)
	NewRunner  func(bin, socket string) Runner
	Output     func(bin string, args ...string) (string, error) // stdout of an external tool
	Exec       func(path string, argv []string) error           // replaces the process; returns only on failure
	Spawn      func(path string, args, env []string) error      // detached background child
	Executable func() (string, error)
	Sleep      func(time.Duration)
	Now        func() time.Time
	Rand       func(n int) int
	GOOS       string
	UID        int
	Stdout     io.Writer
	Stderr     io.Writer
}

const usage = `usage: aval tmux <name> [args]
  sidebar-toggle            open or close the file-tree sidebar
  sidebar-follow [dir]      point the sidebar at a directory (zsh chpwd hook)
  sidebar-watch <pane> <id> keep the sidebar on the work pane's directory
  open-file <file>...       open files in a new tmux window
  close-file                close a window opened by open-file
  paste-router              route cmd+v by what the clipboard holds
  statusbar                 render status-right
  launch                    Ghostty's startup command`

// Run dispatches `aval tmux <name>` and returns the process exit code.
func Run(args []string, env Env) int {
	if len(args) == 0 {
		fmt.Fprintln(env.Stderr, usage)
		return 2
	}
	rest := args[1:]
	switch args[0] {
	case "sidebar-toggle":
		return sidebarToggle(env)
	case "sidebar-follow":
		return sidebarFollow(rest, env)
	case "sidebar-watch":
		return sidebarWatch(rest, env)
	case "open-file":
		return openFile(rest, env)
	case "close-file":
		return closeFile(env)
	case "paste-router":
		return pasteRouter(env)
	case "statusbar":
		return statusbar(env)
	case "launch":
		return launch(env)
	}
	fmt.Fprintln(env.Stderr, usage)
	return 2
}

// OSEnv is the production Env.
func OSEnv() Env {
	env := Env{
		Getenv:    os.Getenv,
		LookupEnv: os.LookupEnv,
		IsExec: func(p string) bool {
			st, err := os.Stat(p)
			return err == nil && !st.IsDir() && st.Mode().Perm()&0o111 != 0
		},
		IsDir: func(p string) bool {
			st, err := os.Stat(p)
			return err == nil && st.IsDir()
		},
		LookPath:   exec.LookPath,
		Executable: os.Executable,
		Sleep:      time.Sleep,
		Now:        time.Now,
		GOOS:       goos(),
		UID:        os.Getuid(),
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Exec:       execReplace,
		Spawn:      spawnDetached,
	}
	env.Rand = randInt
	env.NewRunner = func(bin, socket string) Runner {
		return execRunner{bin: bin, socket: socket, stderr: env.Stderr}
	}
	env.Output = func(bin string, args ...string) (string, error) {
		cmd := exec.Command(bin, args...)
		var out strings.Builder
		cmd.Stdout = &out
		err := cmd.Run()
		return out.String(), err
	}
	return env
}

// execRunner runs the real tmux. Its stderr is passed through so a failing
// command still explains itself.
type execRunner struct {
	bin, socket string
	stderr      io.Writer
}

func (r execRunner) Run(args ...string) (string, error) { return r.run(r.stderr, args) }

func (r execRunner) Query(args ...string) (string, error) { return r.run(io.Discard, args) }

func (r execRunner) run(stderr io.Writer, args []string) (string, error) {
	if r.socket != "" {
		args = append([]string{"-L", r.socket}, args...)
	}
	cmd := exec.Command(r.bin, args...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = stderr
	err := cmd.Run()
	return out.String(), err
}

// trim mirrors shell command substitution: trailing newlines go.
func trim(s string) string { return strings.TrimRight(s, "\n") }

// search directories, in order. Nix first for ordinary tools (a user who
// installed them there wants them); Homebrew first for tmux, where the system
// build is the fallback of last resort.
func toolDirs(home string) []string {
	return []string{home + "/.nix-profile/bin", "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"}
}

func tmuxDirs(home string) []string {
	return []string{"/opt/homebrew/bin", "/usr/local/bin", home + "/.nix-profile/bin", "/usr/bin", "/bin"}
}

// resolve turns a bare command name into an absolute path. A name that is
// already a path (or carries arguments, when keepSpaced) is left alone: the
// caller chose it. Unresolvable names are returned as given, so the failure
// surfaces where the command runs.
func resolve(env Env, name string, dirs []string, keepSpaced bool) string {
	if strings.Contains(name, "/") || (keepSpaced && strings.Contains(name, " ")) {
		return name
	}
	for _, d := range dirs {
		if p := d + "/" + name; env.IsExec(p) {
			return p
		}
	}
	if p, err := env.LookPath(name); err == nil && p != "" {
		return p
	}
	return name
}

// findTmux searches for tmux by absolute path, then PATH. Empty when absent.
func findTmux(env Env) string {
	for _, d := range tmuxDirs(env.Getenv("HOME")) {
		if p := d + "/tmux"; env.IsExec(p) {
			return p
		}
	}
	if p, err := env.LookPath("tmux"); err == nil && p != "" && env.IsExec(p) {
		return p
	}
	return ""
}

// tmuxBin is $TMUX_BIN (resolved) or the search result.
func tmuxBin(env Env) string {
	if v := env.Getenv("TMUX_BIN"); v != "" {
		return resolve(env, v, tmuxDirs(env.Getenv("HOME")), false)
	}
	return findTmux(env)
}

func newRunner(env Env, bin string) Runner {
	return env.NewRunner(bin, env.Getenv("TMUX_SOCKET"))
}

// shellQuote quotes for POSIX sh the way printf %q does for plain words:
// safe words stay bare, everything else is single-quoted.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("_/.:=@%+,-", r):
		default:
			safe = false
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

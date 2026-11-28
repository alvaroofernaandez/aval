package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// On Windows the repo's hook and status-line commands cannot be used as they
// are: they are POSIX shell one-liners (`a="$HOME/.local/bin/aval"; [ -x "$a" ]
// ...`). Claude Code runs a command hook through Git Bash when it is installed
// and through PowerShell when it is not, so a hook that must work on both can
// depend on neither. Claude Code's exec form (`command` plus `args`) skips the
// shell entirely, so sync rewrites every aval hook into it. The status line
// has no exec form: it stays a command string, written with forward slashes
// and no shell syntax, which both Git Bash and PowerShell run.
// https://code.claude.com/docs/en/hooks (shell form vs exec form, Windows)
// https://code.claude.com/docs/en/statusline (Windows configuration)

// avalInvocation matches the shell wrapper the repo uses for aval, capturing
// the subcommand: `[ -x "$a" ] && "$a" hook lessons || true` -> `hook lessons`.
var avalInvocation = regexp.MustCompile(`\[ -x "\$a" \] && "\$a" ([a-z][a-z -]*?)(?: \|\| [^;]+)?$`)

// ForOS adapts the repo-owned settings keys (hooks, statusLine) to the target
// OS. For anything but "windows" the input is returned unchanged. avalBin is
// the aval executable the hooks must run; empty means ResolveAvalBin. The
// input is not mutated.
func ForOS(repo map[string]any, goos, avalBin, home string) map[string]any {
	if goos != "windows" {
		return repo
	}
	if avalBin == "" {
		avalBin = ResolveAvalBin(goos, home)
	}
	out := cloneMap(repo)
	if hooks, ok := out["hooks"].(map[string]any); ok {
		for _, groups := range hooks {
			gl, _ := groups.([]any)
			for _, g := range gl {
				gm, _ := g.(map[string]any)
				hl, _ := gm["hooks"].([]any)
				for _, h := range hl {
					if hm, ok := h.(map[string]any); ok {
						adaptHook(hm, avalBin)
					}
				}
			}
		}
	}
	if sl, ok := out["statusLine"].(map[string]any); ok {
		if cmd, _ := sl["command"].(string); cmd != "" {
			if argv, ok := avalArgs(cmd); ok {
				sl["command"] = statusCommand(avalBin, argv)
			}
		}
	}
	return out
}

// avalArgs extracts the aval subcommand (as argv) from a repo hook command.
func avalArgs(cmd string) ([]string, bool) {
	m := avalInvocation.FindStringSubmatch(strings.TrimSpace(cmd))
	if m == nil {
		return nil, false
	}
	return strings.Fields(m[1]), true
}

func adaptHook(h map[string]any, avalBin string) {
	if t, _ := h["type"].(string); t != "" && t != "command" {
		return
	}
	cmd, _ := h["command"].(string)
	argv, ok := avalArgs(cmd)
	if !ok {
		return
	}
	h["command"] = forwardSlashes(avalBin)
	args := make([]any, len(argv))
	for i, a := range argv {
		args[i] = a
	}
	h["args"] = args
}

// statusCommand is the status-line command string: the aval path with forward
// slashes (Git Bash eats backslashes), quoted only when it contains a space.
func statusCommand(avalBin string, argv []string) string {
	p := forwardSlashes(avalBin)
	if strings.ContainsAny(p, " \t") {
		p = `"` + p + `"`
	}
	return p + " " + strings.Join(argv, " ")
}

func forwardSlashes(p string) string { return strings.ReplaceAll(p, `\`, "/") }

// ResolveAvalBin finds the aval executable the Windows hooks should run:
// ~/.local/bin/aval.exe (where the installer docs put it), else aval on PATH,
// else the running executable, else the conventional path even if absent (the
// doctor then reports it as unresolved, which is the truth).
func ResolveAvalBin(goos, home string) string {
	name := "aval"
	if goos == "windows" {
		name = "aval.exe"
	}
	conventional := filepath.Join(home, ".local", "bin", name)
	if fi, err := os.Stat(conventional); err == nil && !fi.IsDir() {
		return conventional
	}
	if p, err := exec.LookPath("aval"); err == nil {
		return p
	}
	if runtime.GOOS == goos {
		if exe, err := os.Executable(); err == nil && strings.EqualFold(filepath.Base(exe), name) {
			return exe
		}
	}
	return conventional
}

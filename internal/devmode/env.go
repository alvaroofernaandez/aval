package devmode

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// isolatedPaths maps every variable that points at a directory or file to its
// place inside the sandbox. Why each one is here:
//
//	HOME / USERPROFILE   os.UserHomeDir, which every aval command resolves from
//	XDG_*                OpenCode, gh, atuin, yazi and aval's own state/cache
//	CLAUDE_CONFIG_DIR    Claude Code's config dir (documented override)
//	CODEX_HOME           Codex CLI's home (`codex --help` documents it)
//	GEMINI_CLI_HOME      Gemini CLI replaces os.homedir() with it (gemini-cli-core utils/paths)
//	AVAL_PRIVATE_DIR     the private overlay (sync.PrivateDir)
//	GIT_CONFIG_GLOBAL    replaces ~/.gitconfig and the XDG git config
//	ENGRAM_DATA_DIR      Engram's data dir (`engram --help`; default ~/.engram)
//	TMPDIR, TMUX_TMPDIR  scratch files and the tmux server socket
//	ZDOTDIR              where zsh looks for its rc files
func isolatedPaths(s Sandbox, goos string) map[string]string {
	h := s.Home()
	m := map[string]string{
		"HOME":              h,
		"XDG_CONFIG_HOME":   filepath.Join(h, ".config"),
		"XDG_DATA_HOME":     filepath.Join(h, ".local", "share"),
		"XDG_STATE_HOME":    filepath.Join(h, ".local", "state"),
		"XDG_CACHE_HOME":    filepath.Join(h, ".cache"),
		"CLAUDE_CONFIG_DIR": filepath.Join(h, ".claude"),
		"CODEX_HOME":        filepath.Join(h, ".codex"),
		"GEMINI_CLI_HOME":   h,
		"AVAL_PRIVATE_DIR":  filepath.Join(h, ".config", "aval", "private"),
		"GIT_CONFIG_GLOBAL": s.GitConfig(),
		"ENGRAM_DATA_DIR":   filepath.Join(h, ".engram"),
		"TMPDIR":            s.Tmp(),
		"TMUX_TMPDIR":       filepath.Join(s.Tmp(), "tmux"),
		"ZDOTDIR":           s.RC(),
	}
	if goos == "windows" {
		m["USERPROFILE"] = h
		m["APPDATA"] = filepath.Join(h, "AppData", "Roaming")
		m["LOCALAPPDATA"] = filepath.Join(h, "AppData", "Local")
		m["TEMP"] = s.Tmp()
		m["TMP"] = s.Tmp()
	}
	return m
}

// dropped are removed from the inherited environment because they would point
// a command back at the real machine: another aval/engram setup, a live tmux
// server, the real repository, the runtime dir with the user's sockets.
var droppedExact = map[string]bool{
	"TMUX": true, "TMUX_PANE": true, "XDG_RUNTIME_DIR": true,
	"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true, "GIT_COMMON_DIR": true,
	"GIT_OBJECT_DIRECTORY": true, "GIT_NAMESPACE": true, "GIT_CONFIG": true,
}

// A prefix is dropped wholesale: AVAL_* (AVAL_BIN, AVAL_LESSONS_DIR ... all
// name real paths) and ENGRAM_* (ENGRAM_CLOUD_* would sync the sandbox's
// memory to the real cloud).
var droppedPrefix = []string{"AVAL_", "ENGRAM_"}

func sameKey(goos, a, b string) bool {
	if goos == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Env returns the environment for a command run inside s: base with the
// isolated variables replaced, the leaky ones removed, and the dev markers
// added. Go's caches stay shared on purpose (they are content addressed and
// rebuilding the world inside every sandbox would make dev mode unusable).
func Env(s Sandbox, base []string, goos, realHome string) []string {
	paths := isolatedPaths(s, goos)

	pathKey := "PATH"
	var kept []string
	var oldPath string
	for _, e := range base {
		k, v, _ := strings.Cut(e, "=")
		if k == "" {
			continue
		}
		if sameKey(goos, k, "PATH") {
			pathKey, oldPath = k, v
			continue
		}
		skip := droppedExact[k]
		for _, p := range droppedPrefix {
			if strings.HasPrefix(k, p) {
				skip = true
			}
		}
		for iso := range paths {
			if sameKey(goos, k, iso) {
				skip = true
			}
		}
		for _, fixed := range []string{"GIT_CONFIG_NOSYSTEM"} {
			if sameKey(goos, k, fixed) {
				skip = true
			}
		}
		if !skip {
			kept = append(kept, e)
		}
	}

	out := kept
	for k, v := range paths {
		out = append(out, k+"="+v)
	}
	newPath := s.Bin()
	if oldPath != "" {
		newPath += string(os.PathListSeparator) + oldPath
	}
	out = append(out,
		pathKey+"="+newPath,
		"GIT_CONFIG_NOSYSTEM=1",
		"ENGRAM_PORT="+strconv.Itoa(engramPort(s.Name)),
		"AVAL_DEV=1",
		"AVAL_DEV_NAME="+s.Name,
		"AVAL_DEV_ROOT="+s.Root,
		"AVAL_DEV_REAL_HOME="+realHome,
		"AVAL_NO_UPDATE_CHECK=1",
	)
	return out
}

// engramPort is a stable port per sandbox, away from the real daemon's 7437.
func engramPort(name string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return 17500 + int(h.Sum32()%1000)
}

// VerifyEnv is the runtime self-check run before anything is launched: every
// isolated variable must be set and resolve under root. It fails closed, so a
// bug in Env can never turn into a write to the real machine.
func VerifyEnv(env []string, root, goos string) error {
	got := map[string]string{}
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		if goos == "windows" {
			k = strings.ToUpper(k)
		}
		got[k] = v
	}
	r, err := resolve(root)
	if err != nil {
		return err
	}
	probe := Sandbox{Name: "x", Root: root}
	for k := range isolatedPaths(probe, goos) {
		key := k
		if goos == "windows" {
			key = strings.ToUpper(k)
		}
		v, ok := got[key]
		if !ok || v == "" {
			return &envError{Var: k}
		}
		rv, err := resolve(v)
		if err != nil {
			return err
		}
		if !inside(r, rv) {
			return &envError{Var: k, Value: v, Root: root}
		}
	}
	return nil
}

// envError is a sandbox variable that is unset (Value "") or points outside
// the sandbox root. Var names it.
type envError struct{ Var, Value, Root string }

func (e *envError) Error() string {
	if e.Value == "" {
		return fmt.Sprintf("isolated variable %s is not set", e.Var)
	}
	return fmt.Sprintf("%s=%s is outside the sandbox root %s", e.Var, e.Value, e.Root)
}

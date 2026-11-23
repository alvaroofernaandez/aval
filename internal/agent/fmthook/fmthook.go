// Package fmthook is `aval hook fmt`: after an agent edits a file, format that
// one file with the formatter its project already uses.
//
// The hook runs after every Edit and Write, so its no-op path (no known
// extension, no formatter, outside a repository) is pure stat calls: it reads
// no config and starts no process. A process starts only once a formatter is
// resolved, bounded by a timeout (ADR 0007). It never fails the edit: a
// formatter that fails or hangs leaves one warning line and the file as it was.
//
// It is a leaf (ADR 0002), so it does not reuse repoprofile's stack detection:
// the few files it checks are its own.
package fmthook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Runner starts a formatter in dir and waits for it, bounded by ctx.
type Runner func(ctx context.Context, dir, name string, args ...string) error

// Options carries every effect the hook has, so a test can see each one.
type Options struct {
	Run      Runner                       // starts the formatter
	LookPath func(string) (string, error) // PATH lookup, a stat walk
	Ignored  func(root, file string) bool // git check-ignore
	Timeout  time.Duration                // per formatter run
	GOOS     string                       // picks the .cmd shims on Windows
}

// DefaultTimeout bounds one formatter run.
const DefaultTimeout = 3 * time.Second

// DefaultOptions are the real effects.
func DefaultOptions() Options {
	return Options{Run: execRun, LookPath: exec.LookPath, Ignored: gitIgnored, Timeout: DefaultTimeout, GOOS: runtime.GOOS}
}

type hookInput struct {
	Cwd       string `json:"cwd"`
	ToolInput struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

// Run reads a PostToolUse payload on in, formats the edited file and writes
// at most one warning line to warn. It never fails the edit.
func Run(in io.Reader, warn io.Writer, o Options) {
	file, ok := editedFile(in)
	if !ok {
		return
	}
	root, ok := repoRoot(filepath.Dir(file))
	if !ok {
		return
	}
	argv := resolve(root, file, o)
	if argv == nil || looksBinary(file) || o.Ignored(root, file) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), o.Timeout)
	defer cancel()
	if err := o.Run(ctx, root, argv[0], argv[1:]...); err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("no answer within %s", o.Timeout)
		}
		first, _, _ := strings.Cut(err.Error(), "\n")
		fmt.Fprintf(warn, "aval hook fmt: %s left %s unformatted: %s\n", filepath.Base(argv[0]), filepath.Base(file), first)
	}
}

func editedFile(in io.Reader) (string, bool) {
	b, err := io.ReadAll(in)
	if err != nil {
		return "", false
	}
	var p hookInput
	if json.Unmarshal(b, &p) != nil || p.ToolInput.FilePath == "" {
		return "", false
	}
	file := p.ToolInput.FilePath
	if !filepath.IsAbs(file) {
		if p.Cwd == "" {
			return "", false
		}
		file = filepath.Join(p.Cwd, file)
	}
	return file, true
}

// resolve picks the formatter for file, or nil. Only stat calls.
func resolve(root, file string, o Options) []string {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".go":
		if bin, err := o.LookPath("gofmt"); err == nil {
			return []string{bin, "-w", file}
		}
	case ".ts", ".tsx", ".js", ".jsx", ".json", ".css":
		return nodeFormatter(root, file, o.GOOS)
	case ".py":
		bin, err := o.LookPath("ruff")
		if err == nil && hasAny(root, filepath.Dir(file), "pyproject.toml", "ruff.toml", ".ruff.toml") {
			return []string{bin, "format", file}
		}
	}
	return nil
}

// nodeFormatter is biome when the project configures it and has it installed,
// else prettier on the same terms.
func nodeFormatter(root, file, goos string) []string {
	shim := func(name string) string {
		if goos == "windows" {
			name += ".cmd"
		}
		return filepath.Join(root, "node_modules", ".bin", name)
	}
	if exists(filepath.Join(root, "biome.json")) || exists(filepath.Join(root, "biome.jsonc")) {
		if bin := shim("biome"); exists(bin) {
			return []string{bin, "format", "--write", file}
		}
		return nil
	}
	if bin := shim("prettier"); exists(bin) && hasPrettierConfig(root) {
		return []string{bin, "--write", file}
	}
	return nil
}

func hasPrettierConfig(root string) bool {
	m, _ := filepath.Glob(filepath.Join(root, ".prettierrc*"))
	if len(m) > 0 {
		return true
	}
	m, _ = filepath.Glob(filepath.Join(root, "prettier.config.*"))
	return len(m) > 0
}

// hasAny looks for any of names from dir up to root.
func hasAny(root, dir string, names ...string) bool {
	for d := dir; ; d = filepath.Dir(d) {
		for _, n := range names {
			if exists(filepath.Join(d, n)) {
				return true
			}
		}
		if d == root || filepath.Dir(d) == d {
			return false
		}
	}
}

// repoRoot is the nearest ancestor holding .git.
func repoRoot(dir string) (string, bool) {
	for d := dir; ; {
		if exists(filepath.Join(d, ".git")) {
			return d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// looksBinary is git's test: a NUL byte in the first 8000 bytes.
func looksBinary(file string) bool {
	f, err := os.Open(file)
	if err != nil {
		return true
	}
	defer f.Close()
	buf := make([]byte, 8000)
	n, _ := io.ReadFull(f, buf)
	return bytes.IndexByte(buf[:n], 0) >= 0
}

// execRun runs the formatter. WaitDelay ends the wait when a child of the
// formatter keeps its pipes open after it exits, as deck.ExecRunner does.
func execRun(ctx context.Context, dir, name string, args ...string) error {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if errors.Is(err, exec.ErrWaitDelay) {
		return nil
	}
	if err != nil && stderr.Len() > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return err
}

// gitIgnored asks git whether the repository ignores file. Only on the
// formatting path; an error counts as not ignored.
func gitIgnored(root, file string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "check-ignore", "-q", file)
	return cmd.Run() == nil
}

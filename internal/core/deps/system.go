package deps

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// HomebrewPrefixes are the default Homebrew prefixes: Apple silicon, Intel
// macOS and Linux.
var HomebrewPrefixes = []string{"/opt/homebrew", "/usr/local", "/home/linuxbrew/.linuxbrew"}

// SystemProber probes the real machine. It reads PATH through Getenv, never
// from the process, so a caller decides what the probe sees. Nothing it does
// touches the network: a lookup is a stat, a version is the tool's own
// --version (or the args the entry gives) under the probe's context.
type SystemProber struct {
	GOOS   string
	Getenv func(string) string
	// SharePrefixes are searched for share probes; nil means HomebrewPrefixes.
	SharePrefixes []string
	// NoVersion makes bin probes a lookup only: no process is started. The
	// doctor and the hub probe this way.
	NoVersion bool
}

// Probe looks for one tool.
func (s SystemProber) Probe(ctx context.Context, p Probe) (Found, error) {
	switch p.Kind {
	case ProbeBin:
		path, err := s.lookPath(p.Target)
		if err != nil {
			return Found{}, err
		}
		if s.NoVersion || len(p.Args) == 0 {
			return Found{Path: path}, ctx.Err()
		}
		return s.version(ctx, path, p.Args)
	case ProbeApp:
		if s.GOOS != "darwin" {
			return Found{}, ErrSkipped
		}
		return exists(p.Target)
	case ProbeShare:
		prefixes := s.SharePrefixes
		if prefixes == nil {
			prefixes = HomebrewPrefixes
		}
		for _, pre := range prefixes {
			if f, err := exists(filepath.Join(pre, "share", filepath.FromSlash(p.Target))); err == nil {
				return f, nil
			}
		}
		return Found{}, fmt.Errorf("%w: share/%s", ErrNotFound, p.Target)
	}
	return Found{}, fmt.Errorf("%w: %q", ErrBadProbe, p.Kind)
}

func exists(path string) (Found, error) {
	if _, err := os.Stat(path); err != nil {
		return Found{}, fmt.Errorf("%w: %s", ErrNotFound, path)
	}
	return Found{Path: path}, nil
}

// lookPath is exec.LookPath over the PATH Getenv returns.
func (s SystemProber) lookPath(name string) (string, error) {
	exts := []string{""}
	if s.GOOS == "windows" {
		exts = []string{".exe", ".cmd", ".bat", ".com"}
		if pe := s.Getenv("PATHEXT"); pe != "" {
			exts = strings.Split(strings.ToLower(pe), ";")
		}
	}
	for _, dir := range filepath.SplitList(s.Getenv("PATH")) {
		if dir == "" {
			continue // an empty entry would mean the current directory
		}
		for _, ext := range exts {
			p := filepath.Join(dir, name+ext)
			if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && (s.GOOS == "windows" || fi.Mode()&0o111 != 0) {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("%w: %s on PATH", ErrNotFound, name)
}

// version runs the tool with args and keeps the first line of its output.
// A tool that is found but cannot print its version is still present.
func (s SystemProber) version(ctx context.Context, path string, args []string) (Found, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = nil
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return Found{}, ctx.Err()
	}
	f := Found{Path: path}
	if err == nil {
		sc := bufio.NewScanner(bytes.NewReader(out))
		if sc.Scan() {
			f.Version = strings.TrimSpace(sc.Text())
		}
	}
	return f, nil
}

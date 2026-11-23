package deps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// executable writes an executable file name into dir and returns its path.
func executable(t *testing.T, dir, name, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func envWith(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestSystemProberFindsABinOnTheGivenPath(t *testing.T) {
	dir := t.TempDir()
	want := executable(t, dir, "tool", "#!/bin/sh\n")
	s := SystemProber{GOOS: runtime.GOOS, Getenv: envWith(map[string]string{"PATH": dir}), NoVersion: true}
	f, err := s.Probe(context.Background(), Probe{Kind: ProbeBin, Target: "tool"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Path != want {
		t.Errorf("path = %q, want %q", f.Path, want)
	}
	if _, err := s.Probe(context.Background(), Probe{Kind: ProbeBin, Target: "other"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestSystemProberIgnoresTheProcessPath(t *testing.T) {
	s := SystemProber{GOOS: runtime.GOOS, Getenv: envWith(nil), NoVersion: true}
	if _, err := s.Probe(context.Background(), Probe{Kind: ProbeBin, Target: "go"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("an empty PATH must find nothing, got %v", err)
	}
}

func TestSystemProberSkipsNonExecutableFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no executable bit")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tool"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s := SystemProber{GOOS: runtime.GOOS, Getenv: envWith(map[string]string{"PATH": dir}), NoVersion: true}
	if _, err := s.Probe(context.Background(), Probe{Kind: ProbeBin, Target: "tool"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestSystemProberReadsTheVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture is a shell script")
	}
	dir := t.TempDir()
	executable(t, dir, "tool", "#!/bin/sh\necho \"tool 1.2.3\"\necho second line\n")
	s := SystemProber{GOOS: runtime.GOOS, Getenv: envWith(map[string]string{"PATH": dir})}
	f, err := s.Probe(context.Background(), Probe{Kind: ProbeBin, Target: "tool", Args: []string{"--version"}})
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != "tool 1.2.3" {
		t.Errorf("version = %q", f.Version)
	}
}

func TestSystemProberChecksAppsOnMacOSOnly(t *testing.T) {
	app := filepath.Join(t.TempDir(), "Tool.app")
	if err := os.Mkdir(app, 0o755); err != nil {
		t.Fatal(err)
	}
	mac := SystemProber{GOOS: "darwin", Getenv: envWith(nil)}
	if f, err := mac.Probe(context.Background(), Probe{Kind: ProbeApp, Target: app}); err != nil || f.Path != app {
		t.Errorf("darwin: %+v, %v", f, err)
	}
	if _, err := mac.Probe(context.Background(), Probe{Kind: ProbeApp, Target: app + "-gone"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("darwin, absent: want ErrNotFound, got %v", err)
	}
	linux := SystemProber{GOOS: "linux", Getenv: envWith(nil)}
	if _, err := linux.Probe(context.Background(), Probe{Kind: ProbeApp, Target: app}); !errors.Is(err, ErrSkipped) {
		t.Errorf("linux: want ErrSkipped, got %v", err)
	}
}

func TestSystemProberChecksShareUnderEachPrefix(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	want := filepath.Join(b, "share", "plugin", "plugin.zsh")
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s := SystemProber{GOOS: runtime.GOOS, Getenv: envWith(nil), SharePrefixes: []string{a, b}}
	if f, err := s.Probe(context.Background(), Probe{Kind: ProbeShare, Target: "plugin/plugin.zsh"}); err != nil || f.Path != want {
		t.Errorf("share: %+v, %v", f, err)
	}
	if _, err := s.Probe(context.Background(), Probe{Kind: ProbeShare, Target: "gone.zsh"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("share, absent: want ErrNotFound, got %v", err)
	}
}

// A tool the lookup found is present even when the probe's time ran out: a
// slow PATH walk (Windows, many probes at once) must not turn a present tool
// into a missing one, and neither must a version command that hangs.
func TestSystemProberFoundToolStaysPresentAfterTheDeadline(t *testing.T) {
	dir := t.TempDir()
	want := executable(t, dir, "tool", "#!/bin/sh\nsleep 5\n")
	expired, cancel := context.WithCancel(context.Background())
	cancel()
	for _, noVersion := range []bool{true, false} {
		s := SystemProber{GOOS: runtime.GOOS, Getenv: envWith(map[string]string{"PATH": dir}), NoVersion: noVersion}
		f, err := s.Probe(expired, Probe{Kind: ProbeBin, Target: "tool", Args: []string{"--version"}})
		if err != nil || f.Path != want {
			t.Errorf("NoVersion=%v: got %+v, %v; want present at %s", noVersion, f, err, want)
		}
	}
}

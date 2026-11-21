package tmux

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeRunner stands in for a tmux client: it records every call and answers
// from reply, so no live tmux server is needed.
type fakeRunner struct {
	calls []string
	reply func(key string) (string, error)
}

func (f *fakeRunner) Run(args ...string) (string, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if f.reply == nil {
		return "", nil
	}
	return f.reply(key)
}

func (f *fakeRunner) Query(args ...string) (string, error) { return f.Run(args...) }

func (f *fakeRunner) called(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

type spawnCall struct {
	path string
	args []string
	env  []string
}

// harness builds an Env whose every side effect is observable and injectable.
type harness struct {
	vars       map[string]string
	exec       map[string]bool // executable files
	dirs       map[string]bool
	look       map[string]string // command -v
	outputs    map[string]string // "bin arg arg" -> stdout
	outErr     map[string]bool
	out, errb  bytes.Buffer
	runner     *fakeRunner
	runnerBin  string
	runnerSock string
	outCalls   []string
	execCalls  [][]string
	execErr    error
	spawns     []spawnCall
	sleeps     []time.Duration
	now        time.Time
	goos       string
	randVal    int
	self       string
}

func newHarness() *harness {
	return &harness{
		vars:    map[string]string{"HOME": "/home/u"},
		exec:    map[string]bool{},
		dirs:    map[string]bool{},
		look:    map[string]string{},
		outputs: map[string]string{},
		outErr:  map[string]bool{},
		runner:  &fakeRunner{},
		now:     time.Date(2026, 11, 5, 14, 7, 0, 0, time.UTC),
		goos:    "darwin",
		self:    "/home/u/.local/bin/aval",
	}
}

func (h *harness) env() Env {
	return Env{
		Getenv:    func(k string) string { return h.vars[k] },
		LookupEnv: func(k string) (string, bool) { v, ok := h.vars[k]; return v, ok },
		IsExec:    func(p string) bool { return h.exec[p] },
		IsDir:     func(p string) bool { return h.dirs[p] },
		LookPath: func(n string) (string, error) {
			if p, ok := h.look[n]; ok {
				return p, nil
			}
			return "", errors.New("not found")
		},
		NewRunner: func(bin, sock string) Runner {
			h.runnerBin, h.runnerSock = bin, sock
			return h.runner
		},
		Output: func(bin string, args ...string) (string, error) {
			key := strings.Join(append([]string{bin}, args...), " ")
			h.outCalls = append(h.outCalls, key)
			if h.outErr[key] {
				return "", errors.New("failed")
			}
			return h.outputs[key], nil
		},
		Exec: func(path string, argv []string) error {
			h.execCalls = append(h.execCalls, append([]string{path}, argv...))
			return h.execErr
		},
		Spawn: func(path string, args, env []string) error {
			h.spawns = append(h.spawns, spawnCall{path, args, env})
			return nil
		},
		Executable: func() (string, error) { return h.self, nil },
		Sleep:      func(d time.Duration) { h.sleeps = append(h.sleeps, d) },
		Now:        func() time.Time { return h.now },
		Rand:       func(n int) int { return h.randVal % n },
		GOOS:       h.goos,
		UID:        501,
		Stdout:     &h.out,
		Stderr:     &h.errb,
	}
}

func (h *harness) run(args ...string) int { return Run(args, h.env()) }

func assertEq(t *testing.T, name string, want, got any) {
	t.Helper()
	if want != got {
		t.Errorf("%s:\n  want %#v\n  got  %#v", name, want, got)
	}
}

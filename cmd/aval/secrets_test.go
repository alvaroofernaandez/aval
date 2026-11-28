package main

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/secrets"
)

type secretsRig struct {
	store      *secrets.Memory
	out, errb  bytes.Buffer
	execPath   string
	execArgv   []string
	execEnv    []string
	execCalled bool
}

func (r *secretsRig) run(stdin string, args ...string) error {
	d := secretsDeps{
		store: r.store,
		exec: func(path string, argv, env []string) error {
			r.execCalled, r.execPath, r.execArgv, r.execEnv = true, path, argv, env
			return nil
		},
	}
	c := cmdIO{stdin: strings.NewReader(stdin), stdout: &r.out, stderr: &r.errb, env: func(string) string { return "" }}
	return runSecrets(args, c, d)
}

func newRig() *secretsRig { return &secretsRig{store: secrets.NewMemory()} }

const sentinel = "tok-SENTINEL-123"

func TestSecretsSetReadsStdinAndNeverEchoes(t *testing.T) {
	r := newRig()
	if err := r.run(sentinel+"\n", "set", "GH_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if v, _ := r.store.Get("GH_TOKEN"); v != sentinel {
		t.Errorf("stored %q, want the value without its newline", v)
	}
	if strings.Contains(r.out.String()+r.errb.String(), sentinel) {
		t.Error("set echoed the value")
	}
}

func TestSecretsGetWithoutShowPrintsNothing(t *testing.T) {
	r := newRig()
	_ = r.store.Set("K", sentinel)
	if err := r.run("", "get", "K"); err != nil {
		t.Fatal(err)
	}
	if r.out.Len()+r.errb.Len() != 0 {
		t.Errorf("get without --show wrote %q %q", r.out.String(), r.errb.String())
	}
	if err := r.run("", "get", "MISSING"); !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("missing: err = %v, want ErrNotFound", err)
	}
	if err := r.run("", "get", "K", "--show"); err != nil || r.out.String() != sentinel+"\n" {
		t.Errorf("--show: %q, %v", r.out.String(), err)
	}
}

func TestSecretsListAndRm(t *testing.T) {
	r := newRig()
	_ = r.store.Set("B", "2")
	_ = r.store.Set("A", "1")
	if err := r.run("", "list"); err != nil || r.out.String() != "A\nB\n" {
		t.Errorf("list = %q, %v", r.out.String(), err)
	}
	if err := r.run("", "rm", "A"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.store.Get("A"); !errors.Is(err, secrets.ErrNotFound) {
		t.Error("rm did not delete")
	}
}

func TestSecretsExecPutsTheValueOnlyInTheChildEnv(t *testing.T) {
	r := newRig()
	_ = r.store.Set("A", "1")
	_ = r.store.Set("B", "2")
	if err := r.run("", "exec", "A,B", "--", "echo", "hi"); err != nil {
		t.Fatal(err)
	}
	if !r.execCalled || !slices.Equal(r.execArgv, []string{"echo", "hi"}) {
		t.Errorf("exec argv = %v", r.execArgv)
	}
	if !slices.Contains(r.execEnv, "A=1") || !slices.Contains(r.execEnv, "B=2") {
		t.Errorf("env lacks the secrets: %v", r.execEnv)
	}
	r = newRig()
	if err := r.run("", "exec", "NOPE", "--", "echo"); !errors.Is(err, secrets.ErrNotFound) || r.execCalled {
		t.Errorf("missing secret: err = %v, exec called = %v; want ErrNotFound and no exec", err, r.execCalled)
	}
}

func TestSecretsUsageErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"bogus"}, {"set"}, {"exec", "A"}, {"exec", "A", "--"}, {"set", "bad name"}} {
		if err := newRig().run("v", args...); err == nil {
			t.Errorf("%v: want an error", args)
		}
	}
}

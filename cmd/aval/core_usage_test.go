package main

import (
	"bytes"
	"strings"
	"testing"
)

// A bare `aval` prints this instead of installing, so it must point at the
// install command, the repo override and every subcommand.
func TestUsage(t *testing.T) {
	var out bytes.Buffer
	printUsage(&out)
	text := out.String()
	for _, want := range []string{"Usage:", "aval install", "--dry-run", "--yes", "--copy", "--repo", "AVAL_REPO"} {
		if !strings.Contains(text, want) {
			t.Errorf("usage does not mention %q:\n%s", want, text)
		}
	}
	for _, c := range commands {
		if !strings.Contains(text, "aval "+c.name) {
			t.Errorf("usage does not list %q", c.name)
		}
	}
}

func TestKnownCommand(t *testing.T) {
	for _, name := range []string{"install", "update", "doctor", "sync", "uninstall", "version", "release", "hook", "dev", "help"} {
		if !knownCommand(name) {
			t.Errorf("%q should be known", name)
		}
	}
	for _, name := range []string{"instal", "nope", "--yes"} {
		if knownCommand(name) {
			t.Errorf("%q should be unknown", name)
		}
	}
}

func TestParseInstallArgs(t *testing.T) {
	cases := []struct {
		name    string
		base    installOpts
		args    []string
		want    installOpts
		wantErr bool
	}{
		{"none", installOpts{}, nil, installOpts{}, false},
		{"all after the subcommand", installOpts{}, []string{"--yes", "--dry-run", "--copy"}, installOpts{dryRun: true, yes: true, copy: true}, false},
		{"single dash", installOpts{}, []string{"-yes"}, installOpts{yes: true}, false},
		{"global flags carry over", installOpts{dryRun: true}, []string{"--yes"}, installOpts{dryRun: true, yes: true}, false},
		{"unknown flag", installOpts{}, []string{"--force"}, installOpts{}, true},
		{"stray argument", installOpts{}, []string{"extra"}, installOpts{}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseInstallArgs(c.base, c.args)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if err != nil {
				if !strings.Contains(err.Error(), "usage: aval install") {
					t.Errorf("error %q does not show the usage", err)
				}
				return
			}
			if got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSyncCheckReportsDriftAndWritesNothing(t *testing.T) {
	repo, home := fakeMachine(t)
	var out bytes.Buffer
	err := runSync(repo, home, []string{"--check"}, false, &out)
	if !errors.Is(err, errChecksFailed) {
		t.Fatalf("err = %v, want errChecksFailed on drift", err)
	}
	if !strings.Contains(out.String(), "spinnerTipsEnabled") {
		t.Errorf("--check must name the drifted key:\n%s", out.String())
	}
	if _, serr := os.Stat(filepath.Join(home, ".claude/settings.json")); serr == nil {
		t.Error("--check wrote the live settings")
	}
}

func TestRunSyncCheckPassesWhenInSync(t *testing.T) {
	repo, home := fakeMachine(t)
	if err := runSync(repo, home, nil, false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runSync(repo, home, []string{"--check"}, false, &out); err != nil {
		t.Fatalf("in-sync machine must pass --check: %v\n%s", err, out.String())
	}
}

func TestRunSyncCheckFlagsRulesDrift(t *testing.T) {
	repo, home := fakeMachine(t)
	if err := runSync(repo, home, nil, false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".codex/AGENTS.md"), "stale\n")
	var out bytes.Buffer
	err := runSync(repo, home, []string{"--check"}, false, &out)
	if !errors.Is(err, errChecksFailed) || !strings.Contains(out.String(), "AGENTS.md") {
		t.Errorf("rules drift not reported: err=%v\n%s", err, out.String())
	}
}

func TestRunDoctorSecurityFlagAddsChecksToJSON(t *testing.T) {
	repo, home := fakeMachine(t)
	var out bytes.Buffer
	_ = runDoctor(repo, home, []string{"--security", "--json"}, &out, uiSettings{})
	if !strings.Contains(out.String(), "sec-permissions-broad") {
		t.Errorf("--security --json should include the sec- checks:\n%s", out.String())
	}
	out.Reset()
	_ = runDoctor(repo, home, []string{"--json"}, &out, uiSettings{})
	if strings.Contains(out.String(), "sec-") {
		t.Errorf("sec- checks must be off by default:\n%s", out.String())
	}
}

func TestRunDoctorSecurityAllowFlag(t *testing.T) {
	repo, home := fakeMachine(t)
	writeFile(t, filepath.Join(home, ".claude/settings.json"), `{"permissions":{"allow":["Bash(*)"]}}`)
	var out bytes.Buffer
	_ = runDoctor(repo, home, []string{"--security", "--json"}, &out, uiSettings{})
	if !strings.Contains(out.String(), "wildcard-broad") {
		t.Fatalf("precondition: broad entry should be flagged:\n%s", out.String())
	}
	out.Reset()
	_ = runDoctor(repo, home, []string{"--security", "--allow", "sec-permissions-broad", "--json"}, &out, uiSettings{})
	if strings.Contains(out.String(), "wildcard-broad") {
		t.Errorf("--allow did not suppress:\n%s", out.String())
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

func budgetHome(t *testing.T, script string) string {
	t.Helper()
	home := t.TempDir()
	hook := filepath.Join(home, "h.sh")
	writeFile(t, hook, script)
	chmodX(t, hook)
	writeFile(t, filepath.Join(home, ".claude", "CLAUDE.md"), "rules\n")
	writeFile(t, filepath.Join(home, ".claude", "settings.json"),
		fmt.Sprintf(`{"hooks":{"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":%q}]}]}}`, hook))
	return home
}

func TestRunBudgetOverBudgetExitsNonZero(t *testing.T) {
	testenv.SkipWindows(t, "the fixture hook is a POSIX shell script")
	home := budgetHome(t, "#!/bin/sh\nsleep 0.2\n")
	var out bytes.Buffer
	err := runBudget(home, []string{"--runs", "2"}, &out)
	if !errors.Is(err, errChecksFailed) {
		t.Errorf("err = %v, want errChecksFailed for a 200 ms PreToolUse hook", err)
	}
	if !strings.Contains(out.String(), "OVER") || !strings.Contains(out.String(), "claude") {
		t.Errorf("table = %s", out.String())
	}
}

func TestRunBudgetUnderBudgetAndJSON(t *testing.T) {
	testenv.SkipWindows(t, "the fixture hook is a POSIX shell script")
	home := budgetHome(t, "#!/bin/sh\nexit 0\n")
	var out bytes.Buffer
	// A generous ceiling keeps this independent of machine load; the budget
	// logic itself is covered in internal/agent/budget.
	if err := runBudget(home, []string{"--json", "--runs", "3", "--hook-budget", "1s"}, &out); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	var r struct {
		Agents []struct {
			Name   string `json:"name"`
			Chars  int    `json:"chars"`
			Tokens int    `json:"approx_tokens"`
		} `json:"agents"`
		Hooks []struct {
			Over bool `json:"over"`
		} `json:"hooks"`
		Over bool `json:"over_budget"`
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if len(r.Agents) != 2 || r.Agents[0].Name != "claude" || r.Agents[0].Chars != 6 || r.Agents[0].Tokens != 2 {
		t.Errorf("agents = %+v", r.Agents)
	}
	if len(r.Hooks) != 1 || r.Over {
		t.Errorf("hooks = %+v over = %v", r.Hooks, r.Over)
	}
}

func TestRunBudgetSkipHooks(t *testing.T) {
	home := budgetHome(t, "#!/bin/sh\nsleep 0.2\n")
	var out bytes.Buffer
	if err := runBudget(home, []string{"--skip-hooks"}, &out); err != nil {
		t.Errorf("--skip-hooks must not run (or fail on) hooks: %v", err)
	}
	if strings.Contains(out.String(), "HOOK") {
		t.Errorf("hook table printed with --skip-hooks:\n%s", out.String())
	}
}

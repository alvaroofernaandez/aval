package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// DESIGN.md section 10: no em dashes in UI copy, and dry runs speak the same
// sentence as the TUI.
func TestDryRunOutputHasNoEmDashes(t *testing.T) {
	t.Run("restore", func(t *testing.T) {
		repo, home := fakeMachine(t)
		seedBackup(t, home)
		var out bytes.Buffer
		if err := runRestore(repo, home, []string{"latest"}, true, &out, englishTr); err != nil {
			t.Fatal(err)
		}
		assertDryRunCopy(t, out.String(), "Dry run: nothing was written.")
	})
	t.Run("uninstall", func(t *testing.T) {
		repo, home, _ := installedMachine(t)
		var out bytes.Buffer
		if err := runUninstall(repo, home, []string{"--dry-run"}, false, false, strings.NewReader(""), &out, englishTr); err != nil {
			t.Fatal(err)
		}
		assertDryRunCopy(t, out.String(), "Dry run: nothing was written.")
	})
	t.Run("sync", func(t *testing.T) {
		repo, home := fakeMachine(t)
		var out bytes.Buffer
		if err := runSync(repo, home, []string{"--dry-run"}, false, &out, englishTr); err != nil {
			t.Fatal(err)
		}
		assertDryRunCopy(t, out.String(), "Dry run: nothing was written.")
	})
	t.Run("update", func(t *testing.T) {
		repo, home := upstreamPair(t)
		var out bytes.Buffer
		if err := runUpdate(repo, home, true, false, &out, io.Discard); err != nil {
			t.Fatal(err)
		}
		assertDryRunCopy(t, out.String(), "Dry run: nothing was changed.")
	})
}

func assertDryRunCopy(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, "—") {
		t.Errorf("output contains an em dash:\n%s", got)
	}
	if !strings.Contains(got, want) {
		t.Errorf("output misses %q:\n%s", want, got)
	}
}

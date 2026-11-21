package main

import "testing"

func TestRunTmuxUnknownSubcommand(t *testing.T) {
	if got := runTmux([]string{"nope"}); got != 2 {
		t.Fatalf("exit code = %d, want 2", got)
	}
}

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/agent/prepush"
)

// `aval hook pre-push patterns` is how the shell suites read the pattern list.
func TestPrePushPatternsSubcommand(t *testing.T) {
	var out bytes.Buffer
	if err := runHook([]string{"pre-push", "patterns"}, procIO(strings.NewReader(""), &out)); err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	want := prepush.Patterns()
	if len(got) != len(want) {
		t.Fatalf("got %d patterns, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pattern %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPrePushDriftBinary(t *testing.T) {
	t.Setenv("AVAL_BIN", "/does/not/exist")
	if drift := prePushDrift("/repo"); drift != nil {
		t.Fatal("a missing AVAL_BIN means nothing to check")
	}
	t.Setenv("AVAL_BIN", "")
	if drift := prePushDrift("/repo"); drift == nil {
		t.Fatal("without AVAL_BIN the running binary does the check")
	}
}

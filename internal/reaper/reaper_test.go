package reaper

import (
	"strconv"
	"strings"
	"testing"
)

func TestParseEtime(t *testing.T) {
	for in, want := range map[string]int{
		"00:05": 5, "10:05": 605, "1:02:03": 3723, "2-00:00:01": 172801, "01-10:00:00": 122400, "09:08": 548,
	} {
		got, err := ParseEtime(in)
		if err != nil || got != want {
			t.Errorf("ParseEtime(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := ParseEtime("junk"); err == nil {
		t.Error("junk must not parse")
	}
}

const psOut = `  100     1       15:00 /bin/sh /tmp/x/fake-sidecar.sh
  101     1       15:00 /tmp/x/my-server.sh
  102   555       15:00 /bin/sh /tmp/x/fake-sidecar.sh
  103     1       00:30 node /p/node_modules/vitest/dist/cli.js
  104     1     1-00:00 /w/target/debug/deps/app-abc123
  105     1       20:00 /Users/me/Library/Caches/ms-playwright/chromium/chrome-headless-shell
  106     1       20:00 node /srv/app/server.js
`

func TestSelect(t *testing.T) {
	procs := ParsePS(psOut)
	if len(procs) != 7 {
		t.Fatalf("parsed %d procs", len(procs))
	}
	var got []int
	for _, p := range Select(procs, 600, 999) {
		got = append(got, p.PID)
	}
	want := []int{100, 104, 105}
	if len(got) != len(want) {
		t.Fatalf("selected %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("selected %v, want %v", got, want)
		}
	}
	for _, p := range Select(procs, 0, 100) {
		if p.PID == 100 {
			t.Fatal("must never select itself")
		}
	}
}

func TestRunDryAndKill(t *testing.T) {
	var calls []string
	opt := Options{
		MinAge: 600, Self: 999,
		PS:           func() (string, error) { return psOut, nil },
		KillChildren: func(pid int) { calls = append(calls, "children:"+strconv.Itoa(pid)) },
		Kill:         func(pid int) bool { calls = append(calls, "kill:"+strconv.Itoa(pid)); return true },
	}
	var out strings.Builder
	opt.Dry = true
	if err := Run(opt, &out); err != nil || len(calls) != 0 {
		t.Fatalf("dry run acted: %v (%v)", calls, err)
	}
	if !strings.Contains(out.String(), "would end 100 (15:00)") {
		t.Fatalf("out = %q", out.String())
	}

	out.Reset()
	opt.Dry = false
	if err := Run(opt, &out); err != nil {
		t.Fatal(err)
	}
	want := "children:100 kill:100 children:104 kill:104 children:105 kill:105"
	if strings.Join(calls, " ") != want { // its own children first, then itself
		t.Fatalf("calls = %v", calls)
	}
	if !strings.Contains(out.String(), "ended 3 orphaned test process(es)") {
		t.Fatalf("out = %q", out.String())
	}
}

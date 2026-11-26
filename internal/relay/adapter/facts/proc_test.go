package facts

import "testing"

type proc struct {
	ppid int
	name string
}

func fakeTable(table map[int]proc) func(int) (int, string, bool) {
	return func(pid int) (int, string, bool) {
		p, ok := table[pid]
		return p.ppid, p.name, ok
	}
}

func TestClaudePID_WalksUpThroughShells(t *testing.T) {
	table := map[int]proc{
		500: {400, "/bin/zsh"},
		400: {300, "sh"},
		300: {100, "/Users/me/.local/share/claude/versions/2.1.331"},
		100: {1, "tmux"},
	}
	if got := ClaudePID(500, fakeTable(table)); got != 300 {
		t.Fatalf("got %d, want 300 (the nearest claude ancestor)", got)
	}
}

func TestClaudePID_NoClaudeAncestorIsZero(t *testing.T) {
	table := map[int]proc{500: {400, "zsh"}, 400: {1, "login"}}
	if got := ClaudePID(500, fakeTable(table)); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
	if got := ClaudePID(999, fakeTable(table)); got != 0 {
		t.Fatalf("unknown pid: got %d, want 0", got)
	}
}

func TestClaudePID_CyclesAndDepthTerminate(t *testing.T) {
	table := map[int]proc{5: {6, "a"}, 6: {5, "b"}}
	if got := ClaudePID(5, fakeTable(table)); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

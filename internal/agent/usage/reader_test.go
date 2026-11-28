package usage

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func fixtureRoots() []Root {
	return []Root{{Dir: filepath.Join("testdata", "claude", "projects"), Source: "claude"}}
}

// The fixture holds six distinct responses: m1..m6, m2 written in two files.
func TestReadCountsEachResponseOnceAcrossFiles(t *testing.T) {
	c, err := Read(fixtureRoots())
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entries) != 6 {
		t.Fatalf("entries = %d, want 6 (m2 appears in two files and counts once)", len(c.Entries))
	}
	if c.Files != 3 {
		t.Errorf("files = %d, want 3", c.Files)
	}
	if c.Corrupt != 1 {
		t.Errorf("corrupt = %d, want 1 (the line that is not JSON)", c.Corrupt)
	}
}

func TestReadTagsTheConsumerFromTheEntrypoint(t *testing.T) {
	c, err := Read(fixtureRoots())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Consumer{}
	for _, e := range c.Entries {
		got[e.Session+"/"+e.Project] = e.Consumer
	}
	// a1 is entrypoint cli, a2 is sdk-cli (claude -p).
	if got["a1//work/alpha"] != Interactive {
		t.Errorf("a1 = %q, want interactive", got["a1//work/alpha"])
	}
	if got["a2//work/alpha"] != Headless {
		t.Errorf("a2 = %q, want headless", got["a2//work/alpha"])
	}
}

func TestReadPiSessionsAreDaemonRows(t *testing.T) {
	c, err := Read([]Root{{Dir: filepath.Join("testdata", "pi", "sessions"), Source: "pi"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entries) != 2 {
		t.Fatalf("entries = %d, want 2 assistant responses", len(c.Entries))
	}
	e := c.Entries[0]
	want := Tokens{Input: 200, Output: 100, CacheWrite: 50, CacheRead: 1000}
	if e.Tokens != want || e.Consumer != Daemon || e.Source != "pi" || e.Project != "/work/gamma" || e.Session != "p1" {
		t.Errorf("entry = %+v", e)
	}
	if !e.HasReported || e.Reported != 258750 {
		t.Errorf("reported = %v/%d, want the 0.0025875 USD Pi wrote", e.HasReported, e.Reported)
	}
}

func TestRootsHonourTheConfigDirVariables(t *testing.T) {
	env := map[string]string{"CLAUDE_CONFIG_DIR": "/a, /b", "PI_CODING_AGENT_DIR": "/pi"}
	got := Roots(func(k string) string { return env[k] }, "/home/u")
	want := []Root{{"/a/projects", "claude"}, {"/b/projects", "claude"}, {"/pi/sessions", "pi"}}
	if !slices.Equal(got, want) {
		t.Errorf("roots = %v, want %v", got, want)
	}
	got = Roots(func(string) string { return "" }, "/home/u")
	want = []Root{
		{filepath.Join("/home/u", ".config", "claude", "projects"), "claude"},
		{filepath.Join("/home/u", ".claude", "projects"), "claude"},
		{filepath.Join("/home/u", ".pi", "agent", "sessions"), "pi"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("default roots = %v, want %v", got, want)
	}
}

func TestReadSkipsAndCountsBrokenLinesAndMissingRoots(t *testing.T) {
	dir := t.TempDir()
	body := "{\"type\":\"assistant\",\"timestamp\":\"2026-11-20T09:00:00Z\",\"message\":{\"model\":\"m\",\"usage\":{\"input_tokens\":1}}}\n" +
		"{\"type\":\"assistant\",\"message\":{\"usage\":{\"input_tokens\":1\n" + // truncated, mentions usage
		"{\"type\":\"user\",\"message\":{\"content\":\"cut off\n" + // truncated, no usage
		"{\"type\":\"user\"}\n"
	if err := os.WriteFile(filepath.Join(dir, "s.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Read([]Root{{Dir: dir, Source: "claude"}, {Dir: filepath.Join(dir, "missing"), Source: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entries) != 1 || c.Corrupt != 2 {
		t.Errorf("entries = %d, corrupt = %d; want 1 and 2", len(c.Entries), c.Corrupt)
	}
}

func TestReadPiStandaloneUsageCountsOnlyWithoutMessages(t *testing.T) {
	dir := t.TempDir()
	hdr := `{"type":"session","id":"s","cwd":"/w"}` + "\n"
	use := `{"type":"usage","timestamp":"2026-11-21T10:00:00Z","model":"m","usage":{"input":5,"output":1}}` + "\n"
	msg := `{"type":"message","timestamp":"2026-11-21T10:00:00Z","message":{"role":"assistant","model":"m","usage":{"input":5,"output":1}}}` + "\n"
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("only.jsonl", strings.Replace(hdr, `"s"`, `"s1"`, 1)+use)
	write("both.jsonl", hdr+use+msg)
	c, err := Read([]Root{{Dir: dir, Source: "pi"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entries) != 2 {
		t.Errorf("entries = %d, want 2: one from the usage-only file, one (not two) from the file with both", len(c.Entries))
	}
}

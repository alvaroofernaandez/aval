package relay

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite the goldens under testdata")

func sample() Capsule {
	return Capsule{
		FirstPrompt: "Add a retry to the upload client",
		LastPrompt:  "Now cover the timeout path with a test",
		Prompts:     []string{"Show me the upload client", "Use exponential backoff"},
		Files:       []string{"internal/upload/client.go", "internal/upload/client_test.go"},
		Repo:        &RepoFacts{Branch: "feat/upload-retry", Changed: 2},
		Work:        []string{"go test ./internal/upload/ (background, running)"},
		Note:        "Next: assert the third attempt waits 400 ms.",
	}
}

func TestCapsuleRender_Golden(t *testing.T) {
	got := Render(sample(), 4_000)
	path := filepath.Join("testdata", "capsule.golden")
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create it): %v", err)
	}
	if got != string(want) {
		t.Fatalf("capsule differs from %s:\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}

func TestCapsuleRender_ShrinksInOrder_NoteTailObjective(t *testing.T) {
	full := Render(sample(), 4_000)
	steps := []struct {
		budget  int
		gone    []string
		present []string
	}{
		{tokensOf(full) - 1, []string{"## Note"}, []string{"## Recent requests", "## Objective"}},
		{tokensOf(full) - 40, []string{"## Note", "## Recent requests"}, []string{"## Objective"}},
	}
	for _, s := range steps {
		got := Render(sample(), s.budget)
		for _, h := range s.gone {
			if strings.Contains(got, h) {
				t.Errorf("budget %d: %q still present", s.budget, h)
			}
		}
		for _, h := range s.present {
			if !strings.Contains(got, h) {
				t.Errorf("budget %d: %q dropped too early", s.budget, h)
			}
		}
	}
}

func TestCapsuleRender_NoRepoOmitsGit(t *testing.T) {
	c := sample()
	c.Repo = nil
	if got := Render(c, 4_000); strings.Contains(got, "## Repository") {
		t.Fatalf("no repository, but the capsule has a repository section:\n%s", got)
	}
}

func TestCapsuleRender_NeverAboveBudget(t *testing.T) {
	c := sample()
	c.FirstPrompt = strings.Repeat("long request ", 400)
	for _, budget := range []int{50, 120, 400} {
		got := Render(c, budget)
		if got == "" || tokensOf(got) > budget {
			t.Errorf("budget %d: rendered %d tokens (%d bytes)", budget, tokensOf(got), len(got))
		}
	}
}

func TestCapsuleRender_Redacts(t *testing.T) {
	c := sample()
	c.LastPrompt = "use the key sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123"
	if got := Render(c, 4_000); strings.Contains(got, "sk-ant-api03-abcdef") {
		t.Fatalf("a credential reached the capsule:\n%s", got)
	}
}

func TestCapsuleRender_KeepsNewestFilesAndPrompts(t *testing.T) {
	c := sample()
	c.Files, c.Prompts = nil, nil
	for i := range 30 {
		c.Files = append(c.Files, fmt.Sprintf("file%02d.go", i))     // newest first
		c.Prompts = append(c.Prompts, fmt.Sprintf("prompt %02d", i)) // oldest first
	}
	got := Render(c, 40_000)
	for _, want := range []string{"file00.go", "file24.go", "prompt 15", "prompt 29"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q missing", want)
		}
	}
	for _, gone := range []string{"file25.go", "prompt 14"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q kept past the limit", gone)
		}
	}
}

func TestCapsuleRender_UnknownChangeCountIsOmitted(t *testing.T) {
	c := sample()
	c.Repo = &RepoFacts{Branch: "main"}
	got := Render(c, 4_000)
	if !strings.Contains(got, "Branch: main\n") || strings.Contains(got, "changed files") {
		t.Fatalf("repository line:\n%s", got)
	}
}

func TestIsRequest(t *testing.T) {
	for text, want := range map[string]bool{
		"Add retries.":                         true,
		"":                                     false,
		CapsuleHeader + "\nFirst request: x\n": false,
		"This session is being continued from a previous conversation that ran out of context.": false,
		"<command-name>/clear</command-name>":                                                   false,
		"<local-command-stdout>Compacted</local-command-stdout>":                                false,
		"[Image #11]":                          false,
		"  [Image #1]\n[Image #2]  ":           false,
		"[Pasted text #3 +40 lines]":           false,
		"[Image #1] [Pasted text #2 +5 lines]": false,
		"[Image #1] what is wrong here?":       true,
		"see [Image #1]":                       true,
	} {
		if got := IsRequest(text); got != want {
			t.Errorf("IsRequest(%.40q) = %v, want %v", text, got, want)
		}
	}
}

// Found in the T11 sandbox run: Claude Code wraps a plugin-submitted prompt
// in one line naming the plugin, so a level 3 capsule no longer starts with
// the header.
func TestCapsuleFirst_UnderARuntimeLine(t *testing.T) {
	text := "The aval-relay plugin sent a message:\n" + CapsuleHeader + "\n\n## Objective\n" + CapsuleFirstRequest + "ALPHA\n"
	if first, ok := CapsuleFirst(text); !ok || first != "ALPHA" {
		t.Fatalf("got %q, %v; want ALPHA, true", first, ok)
	}
	if IsRequest(text) {
		t.Fatal("a wrapped capsule counted as a request")
	}
	if _, ok := CapsuleFirst("Line one\nLine two\n" + CapsuleHeader); ok {
		t.Fatal("a header deeper than the second line counted as a capsule")
	}
}

func TestCapsuleRender_CapsEachPrompt(t *testing.T) {
	c := sample()
	c.LastPrompt = strings.Repeat("x", 1500)
	c.Prompts = []string{strings.Repeat("é", 900)}
	got := Render(c, 40_000)
	if strings.Contains(got, strings.Repeat("x", maxPromptRunes+1)) {
		t.Fatal("a long prompt was kept whole")
	}
	for _, want := range []string{"… [truncated 900 chars]", "… [truncated 300 chars]"} {
		if !strings.Contains(got, want) {
			t.Errorf("marker %q missing:\n%s", want, got)
		}
	}
	if short := Render(sample(), 4_000); strings.Contains(short, "truncated") {
		t.Error("a short prompt was marked as truncated")
	}
}

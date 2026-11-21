package statusline

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string { return ansi.ReplaceAllString(s, "") }

func repo(t *testing.T, branch string) string {
	t.Helper()
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, ".git"), 0o755)
	os.WriteFile(filepath.Join(d, ".git", "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0o644)
	return d
}

func TestRendersWhatTheMetersDoNotShow(t *testing.T) {
	d := repo(t, "feat/x")
	os.MkdirAll(filepath.Join(d, "odd", "tasks"), 0o755)
	os.WriteFile(filepath.Join(d, "odd", "tasks", "plan.md"), []byte("- [x] a\n- [x] b\n- [ ] c\n"), 0o644)
	in := `{"model":{"display_name":"Opus 5.5"},"effort":{"level":"high"},
	  "workspace":{"current_dir":"` + filepath.ToSlash(d) + `","project_dir":"` + filepath.ToSlash(d) + `"},
	  "rate_limits":{"five_hour":{"used_percentage":42.4,"resets_at":0}},
	  "prompt_cache":{"warm":false,"caching_observed":true}}`
	got := plain(Render(strings.NewReader(in)))
	for _, want := range []string{"Opus 5.5", "high", filepath.Base(d), "feat/x", "5h 42%", "cache cold", "odd 2/3"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "\n") {
		t.Errorf("must be one line: %q", got)
	}
}

func TestOptionalPartsAreOmitted(t *testing.T) {
	d := repo(t, "main")
	in := `{"model":{"display_name":"Sonnet"},"workspace":{"current_dir":"` + filepath.ToSlash(d) + `","project_dir":"` + filepath.ToSlash(d) + `"},
	  "prompt_cache":{"warm":true,"caching_observed":true}}`
	got := plain(Render(strings.NewReader(in)))
	for _, bad := range []string{"5h", "cache", "odd"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q should be omitted: %q", bad, got)
		}
	}
}

func TestWorktreeAndGitFile(t *testing.T) {
	main := t.TempDir()
	gitdir := filepath.Join(main, ".git", "worktrees", "wt1")
	os.MkdirAll(gitdir, 0o755)
	os.WriteFile(filepath.Join(gitdir, "HEAD"), []byte("ref: refs/heads/wt-branch\n"), 0o644)
	wt := t.TempDir()
	os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o644)
	in := `{"model":{"display_name":"M"},"workspace":{"current_dir":"` + filepath.ToSlash(wt) + `","git_worktree":"wt1"}}`
	got := plain(Render(strings.NewReader(in)))
	if !strings.Contains(got, "wt-branch") || !strings.Contains(got, "wt1") {
		t.Errorf("worktree branch not shown: %q", got)
	}
}

func TestGarbageInputStillPrintsSomething(t *testing.T) {
	if Render(strings.NewReader("not json")) == "" {
		t.Error("a status line must never be empty")
	}
}

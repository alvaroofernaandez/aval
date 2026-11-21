// Package statusline renders Claude Code's status line (`aval statusline`).
//
// It shows what the context and usage meters already on screen do not: model
// and effort, directory and git branch (and worktree), the 5-hour limit, a
// cold prompt cache, and progress of the repo's odd/tasks plan. It runs on
// every refresh, so it reads files directly: no git process, no network.
package statusline

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type input struct {
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Effort *struct {
		Level string `json:"level"`
	} `json:"effort"`
	Workspace struct {
		CurrentDir  string `json:"current_dir"`
		ProjectDir  string `json:"project_dir"`
		GitWorktree string `json:"git_worktree"`
	} `json:"workspace"`
	RateLimits *struct {
		FiveHour *struct {
			UsedPercentage float64 `json:"used_percentage"`
		} `json:"five_hour"`
	} `json:"rate_limits"`
	PromptCache *struct {
		Warm            bool `json:"warm"`
		CachingObserved bool `json:"caching_observed"`
	} `json:"prompt_cache"`
}

const (
	dim    = "\x1b[2m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	reset  = "\x1b[0m"
)

// Render returns one line. Bad input still yields a line: a status line that
// disappears looks like a crash.
func Render(r io.Reader) string {
	var in input
	_ = json.NewDecoder(r).Decode(&in)

	var parts []string
	model := in.Model.DisplayName
	if model == "" {
		model = "claude"
	}
	if in.Effort != nil && in.Effort.Level != "" {
		model += " " + dim + in.Effort.Level + reset
	}
	parts = append(parts, model)

	dir := in.Workspace.CurrentDir
	if dir != "" {
		loc := filepath.Base(dir)
		if b := branch(dir); b != "" {
			loc += " " + dim + "on" + reset + " " + b
		}
		if in.Workspace.GitWorktree != "" {
			loc += " " + dim + "⎇" + reset + " " + in.Workspace.GitWorktree
		}
		parts = append(parts, loc)
	}

	if in.RateLimits != nil && in.RateLimits.FiveHour != nil {
		p := in.RateLimits.FiveHour.UsedPercentage
		c := ""
		switch {
		case p >= 90:
			c = red
		case p >= 70:
			c = yellow
		}
		parts = append(parts, fmt.Sprintf("%s5h %d%%%s", c, int(p), reset))
	}

	if pc := in.PromptCache; pc != nil && pc.CachingObserved && !pc.Warm {
		parts = append(parts, yellow+"cache cold"+reset)
	}

	proj := in.Workspace.ProjectDir
	if proj == "" {
		proj = dir
	}
	if done, total := oddProgress(proj); total > 0 {
		parts = append(parts, fmt.Sprintf("odd %d/%d", done, total))
	}

	return strings.Join(parts, " "+dim+"·"+reset+" ")
}

// branch reads HEAD for dir's repository, following a worktree's .git file.
func branch(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		g := filepath.Join(d, ".git")
		fi, err := os.Stat(g)
		if err == nil {
			head := filepath.Join(g, "HEAD")
			if !fi.IsDir() {
				b, err := os.ReadFile(g)
				if err != nil {
					return ""
				}
				gd := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(b)), "gitdir:"))
				if !filepath.IsAbs(gd) {
					gd = filepath.Join(d, gd)
				}
				head = filepath.Join(gd, "HEAD")
			}
			b, err := os.ReadFile(head)
			if err != nil {
				return ""
			}
			s := strings.TrimSpace(string(b))
			if ref, ok := strings.CutPrefix(s, "ref: refs/heads/"); ok {
				return ref
			}
			if len(s) >= 7 {
				return s[:7]
			}
			return s
		}
		if p := filepath.Dir(d); p == d {
			return ""
		}
	}
}

// oddProgress counts checkboxes in the most recently modified odd/tasks plan.
func oddProgress(proj string) (done, total int) {
	if proj == "" {
		return 0, 0
	}
	files, _ := filepath.Glob(filepath.Join(proj, "odd", "tasks", "*.md"))
	if len(files) == 0 {
		return 0, 0
	}
	sort.Slice(files, func(i, j int) bool {
		a, _ := os.Stat(files[i])
		b, _ := os.Stat(files[j])
		return a != nil && b != nil && a.ModTime().After(b.ModTime())
	})
	b, err := os.ReadFile(files[0])
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		l := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(l, "- [x]"), strings.HasPrefix(l, "- [X]"):
			done++
			total++
		case strings.HasPrefix(l, "- [ ]"):
			total++
		}
	}
	return done, total
}

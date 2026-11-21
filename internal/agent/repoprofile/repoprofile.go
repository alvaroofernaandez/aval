// Package repoprofile is the SessionStart hook `aval hook repo-profile`.
//
// It detects the project's stack from a few files and tells the agent, in at
// most six lines, how this repo is run and tested, so the right commands are
// used without asking. Unknown directories print nothing (0 tokens). It never
// fails the session: errors are swallowed.
package repoprofile

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type payload struct {
	Cwd string `json:"cwd"`
}

// Run reads the SessionStart payload and prints hook JSON, or nothing.
func Run(in io.Reader, out io.Writer) {
	var p payload
	_ = json.NewDecoder(in).Decode(&p)
	dir := p.Cwd
	if dir == "" {
		dir, _ = os.Getwd()
	}
	lines := Profile(dir)
	if len(lines) == 0 {
		return
	}
	ctx := "Repo profile (detected by aval):\n- " + strings.Join(lines, "\n- ")
	_ = json.NewEncoder(out).Encode(map[string]any{
		"hookSpecificOutput": map[string]string{
			"hookEventName":     "SessionStart",
			"additionalContext": ctx,
		},
	})
}

// Profile returns up to five facts about the project in dir.
func Profile(dir string) []string {
	var lines []string
	exists := func(name string) bool { _, err := os.Stat(filepath.Join(dir, name)); return err == nil }

	if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
			Deps    map[string]string `json:"dependencies"`
			DevDeps map[string]string `json:"devDependencies"`
		}
		_ = json.Unmarshal(b, &pkg)
		pm := "npm"
		switch {
		case exists("pnpm-lock.yaml"):
			pm = "pnpm"
		case exists("bun.lock"), exists("bun.lockb"):
			pm = "bun"
		case exists("yarn.lock"):
			pm = "yarn"
		}
		line := "Node project, package manager " + pm
		if fw := frameworks(pkg.Deps, pkg.DevDeps); fw != "" {
			line += " (" + fw + ")"
		}
		lines = append(lines, line)
		var cmds []string
		for _, s := range []string{"test", "lint", "typecheck", "dev"} {
			if _, ok := pkg.Scripts[s]; ok {
				cmds = append(cmds, pm+" "+run(pm, s))
			}
		}
		if len(cmds) > 0 {
			lines = append(lines, "Scripts: "+strings.Join(cmds, ", "))
		}
	}
	if exists("go.mod") {
		lines = append(lines, "Go module: test with `go test ./...`, vet with `go vet ./...`")
	}
	if exists("pyproject.toml") || exists("requirements.txt") {
		l := "Python project"
		if exists("uv.lock") {
			l += " (uv: `uv run pytest`)"
		} else {
			l += " (tests: `pytest`)"
		}
		lines = append(lines, l)
	}
	for _, c := range []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"} {
		if exists(c) {
			lines = append(lines, "Docker Compose stack: start/stop with `up` / `down` (Kiri runtime)")
			break
		}
	}
	if open := openPlans(dir); open != "" {
		lines = append(lines, "Open plan: "+open)
	}
	if len(lines) > 5 {
		lines = lines[:5]
	}
	return lines
}

func run(pm, script string) string {
	if pm == "npm" && script != "test" {
		return "run " + script
	}
	if script == "test" {
		return "test"
	}
	return script
}

func frameworks(maps ...map[string]string) string {
	known := []string{"next", "astro", "@nestjs/core", "react", "vue", "svelte", "vite", "vitest", "jest", "playwright", "@playwright/test", "prisma", "drizzle-orm"}
	var found []string
	for _, k := range known {
		for _, m := range maps {
			if _, ok := m[k]; ok {
				found = append(found, strings.TrimPrefix(strings.TrimSuffix(k, "/core"), "@"))
				break
			}
		}
	}
	return strings.Join(found, ", ")
}

// openPlans names the odd/tasks files that still have unchecked boxes.
func openPlans(dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "odd", "tasks", "*.md"))
	sort.Strings(files)
	var open []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err == nil && strings.Contains(string(b), "- [ ]") {
			rel, _ := filepath.Rel(dir, f)
			open = append(open, filepath.ToSlash(rel))
		}
	}
	if len(open) > 3 {
		open = append(open[:3], "…")
	}
	return strings.Join(open, ", ")
}

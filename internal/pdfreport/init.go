package pdfreport

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// InitOptions drive `aval pdf init`.
type InitOptions struct {
	Dir         string       // the new report project
	Skill       string       // the pdf-report skill directory holding the template
	Assets      AssetOptions // brand options; Project is set to Dir
	BuildAssets func(AssetOptions) error
	Run         func(cwd, name string, args ...string) error
	Out         io.Writer
}

// SkillDir resolves the pdf-report skill: $AVAL_PDF_SKILL, else the copy the
// installer links under ~/.claude/skills.
func SkillDir() string {
	if d := os.Getenv("AVAL_PDF_SKILL"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "skills", "pdf-report")
}

// OSRun runs a command in cwd with its output discarded.
func OSRun(cwd, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Dir = cwd
	return c.Run()
}

func copyFile(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o644)
}

// Init scaffolds a PDF report project: the template, the renderer and a
// DESIGN.md, then the brand stylesheet and Playwright (headless Chromium).
func Init(o InitOptions) error {
	for from, to := range map[string]string{
		"assets/template.html":          "informe.html",
		"scripts/render.mjs":            "render.mjs",
		"references/DESIGN.template.md": ".agents/DESIGN.md",
	} {
		if err := copyFile(filepath.Join(o.Skill, from), filepath.Join(o.Dir, to)); err != nil {
			return fmt.Errorf("scaffolding from %s: %w", o.Skill, err)
		}
	}
	a := o.Assets
	a.Project = o.Dir
	if err := o.BuildAssets(a); err != nil {
		return err
	}
	if err := o.Run(o.Dir, "npm", "init", "-y"); err != nil {
		return fmt.Errorf("npm init: %w", err)
	}
	if err := o.Run(o.Dir, "npm", "i", "playwright"); err != nil {
		return fmt.Errorf("npm i playwright: %w", err)
	}
	fmt.Fprintf(o.Out, "\nReady in %s\n", o.Dir)
	fmt.Fprintln(o.Out, "  1. Edit informe.html (structure only: never touch the <style> block)")
	fmt.Fprintln(o.Out, "  2. node render.mjs")
	fmt.Fprintln(o.Out, "  3. Every guard must read 'none' before you ship")
	return nil
}

package main

import (
	"io"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/devops/dockerclean"
	"github.com/alvaroofernaandez/aval/internal/devops/kiri"
	"github.com/alvaroofernaandez/aval/internal/docs/pdfreport"
)

const (
	kiriUsage   = "usage: aval kiri up    (make Docker ready on Kiri; idempotent, deletes nothing)"
	dockerUsage = "usage: aval docker cleanup [--dry-run]    (safe prunes on the kiri context only)"
	pdfUsage    = `usage:
  aval pdf init <project-dir> [asset flags]    scaffold a PDF report project
  aval pdf assets [project-dir] [asset flags]  build <project-dir>/assets/brand.css
asset flags:
  --brand-dir <dir>   logo, banner.png and <name>.* portraits (default: brand)
  --people a,b        portrait names
  --logo-file <name>  logo inside --brand-dir (default: logo.png)
  --ink r,g,b         dark logo colourway (default: 23,20,31)
  --accent r,g,b      branded logo colourway (default: 110,59,106)`
)

// runKiri is `aval kiri up`.
func runKiri(args []string) error {
	if len(args) != 1 || args[0] != "up" {
		return newUsage(kiriUsage)
	}
	if kiri.Up(kiri.OSOptions()) != 0 {
		return errChecksFailed
	}
	return nil
}

// runDocker is `aval docker cleanup`.
func runDocker(args []string) error {
	if len(args) == 0 || args[0] != "cleanup" {
		return newUsage(dockerUsage)
	}
	pos, flags := splitFlags(args[1:])
	_, dry := flags["--dry-run"]
	if len(pos) != 0 || len(flags) > 1 || (len(flags) == 1 && !dry) {
		return newUsage(dockerUsage)
	}
	if dockerclean.Cleanup(dockerclean.OSOptions(dry)) != 0 {
		return errChecksFailed
	}
	return nil
}

func bad(flags map[string][]string) bool { _, ok := flags["--dry-run"]; return ok }

// parsePDFAssetFlags reads the asset flags; Project is left for the caller.
func parsePDFAssetFlags(args []string) (pdfreport.AssetOptions, []string, error) {
	pos, flags := splitFlags(args, "--brand-dir", "--people", "--logo-file", "--ink", "--accent")
	last := func(f, def string) string {
		if v := flags[f]; len(v) > 0 {
			return v[len(v)-1]
		}
		return def
	}
	o := pdfreport.AssetOptions{
		BrandDir: last("--brand-dir", "brand"),
		LogoFile: last("--logo-file", "logo.png"),
	}
	for _, p := range strings.Split(last("--people", ""), ",") {
		if p = strings.TrimSpace(p); p != "" {
			o.People = append(o.People, p)
		}
	}
	var err error
	if o.Ink, err = parseRGBFlag(last("--ink", ""), pdfreport.DefaultInk); err != nil {
		return o, pos, err
	}
	if o.Accent, err = parseRGBFlag(last("--accent", ""), pdfreport.DefaultAccent); err != nil {
		return o, pos, err
	}
	return o, pos, nil
}

func parseRGBFlag(v string, def [3]uint8) ([3]uint8, error) {
	if v == "" {
		return def, nil
	}
	return pdfreport.ParseRGB(v)
}

// runPDF is `aval pdf init|assets`.
func runPDF(args []string, out io.Writer) error {
	if len(args) == 0 {
		return newUsage(pdfUsage)
	}
	o, pos, err := parsePDFAssetFlags(args[1:])
	if err != nil {
		return err
	}
	o.Out = out
	switch args[0] {
	case "init":
		if len(pos) != 1 {
			return newUsage(pdfUsage)
		}
		return pdfreport.Init(pdfreport.InitOptions{
			Dir: pos[0], Skill: pdfreport.SkillDir(), Assets: o, Out: out,
			BuildAssets: pdfreport.Assets, Run: pdfreport.OSRun,
		})
	case "assets":
		if len(pos) > 1 {
			return newUsage(pdfUsage)
		}
		o.Project = "."
		if len(pos) == 1 {
			o.Project = pos[0]
		}
		return pdfreport.Assets(o)
	}
	return newUsage(pdfUsage)
}

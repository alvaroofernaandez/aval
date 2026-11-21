package pdfreport

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func skillDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	for p, body := range map[string]string{
		"assets/template.html":          "<html>T</html>",
		"scripts/render.mjs":            "// render",
		"references/DESIGN.template.md": "# design",
	} {
		full := filepath.Join(d, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func TestInitScaffolds(t *testing.T) {
	skill := skillDir(t)
	dir := filepath.Join(t.TempDir(), "propuestas", "acme")
	var ranIn, ran []string
	var assets AssetOptions
	var out bytes.Buffer
	err := Init(InitOptions{
		Dir: dir, Skill: skill, Out: &out,
		Assets:      AssetOptions{BrandDir: "brand", People: []string{"ana"}},
		BuildAssets: func(o AssetOptions) error { assets = o; return nil },
		Run: func(cwd string, name string, args ...string) error {
			ranIn = append(ranIn, cwd)
			ran = append(ran, name+" "+strings.Join(args, " "))
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{
		"informe.html":      "<html>T</html>",
		"render.mjs":        "// render",
		".agents/DESIGN.md": "# design",
	} {
		b, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil || string(b) != want {
			t.Errorf("%s: %q %v", p, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "build-assets.py")); err == nil {
		t.Error("the python asset builder must not be copied")
	}
	if assets.Project != dir || assets.BrandDir != "brand" || len(assets.People) != 1 {
		t.Errorf("assets options %+v", assets)
	}
	if got := strings.Join(ran, "|"); got != "npm init -y|npm i playwright" {
		t.Errorf("npm calls: %s", got)
	}
	for _, c := range ranIn {
		if c != dir {
			t.Errorf("npm ran in %s, want %s", c, dir)
		}
	}
	for _, want := range []string{"Ready in " + dir, "node render.mjs", "Every guard must read 'none'"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestInitMissingSkillFiles(t *testing.T) {
	err := Init(InitOptions{Dir: t.TempDir(), Skill: t.TempDir(), Out: &bytes.Buffer{},
		BuildAssets: func(AssetOptions) error { return nil }, Run: func(string, string, ...string) error { return nil }})
	if err == nil {
		t.Error("an unusable skill dir must fail")
	}
}

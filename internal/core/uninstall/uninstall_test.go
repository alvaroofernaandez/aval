package uninstall

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/manifest"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
	avalsync "github.com/alvaroofernaandez/aval/internal/core/sync"
)

const manifestText = `[all]
label = a
detail = d
platforms = all

zshrc      .zshrc
tmux       .config/tmux
tmux.conf  .tmux.conf
`

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func treeOf(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}
		if rel == ".aval-backup" {
			return filepath.SkipDir
		}
		fi, _ := os.Lstat(p)
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			tg, _ := os.Readlink(p)
			lines = append(lines, fmt.Sprintf("L %s -> %s", rel, tg))
		case fi.IsDir():
			lines = append(lines, fmt.Sprintf("D %s %o", rel, fi.Mode().Perm()))
		default:
			b, _ := os.ReadFile(p)
			lines = append(lines, fmt.Sprintf("F %s %o %q", rel, fi.Mode().Perm(), b))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func clock() func() time.Time {
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	n := 0
	return func() time.Time { n++; return t0.Add(time.Duration(n) * time.Second) }
}

// machine is a repo with a manifest and a HOME holding the user's own files.
func machine(t *testing.T) Options {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need Developer Mode on Windows")
	}
	repo, home := t.TempDir(), t.TempDir()
	put(t, filepath.Join(repo, "install.manifest"), manifestText)
	put(t, filepath.Join(repo, "zshrc"), "repo zshrc")
	put(t, filepath.Join(repo, "tmux/tmux.conf"), "repo tmux")
	put(t, filepath.Join(repo, "tmux.conf"), "repo tmux.conf")
	put(t, filepath.Join(home, ".zshrc"), "my zshrc\n")
	put(t, filepath.Join(home, ".keep/me"), "foreign")
	return Options{Repo: repo, Home: home, GOOS: "linux", Now: clock()}
}

func installAll(t *testing.T, opt Options) {
	t.Helper()
	m, err := manifest.Load(filepath.Join(opt.Repo, "install.manifest"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := plan.Build(m, plan.Options{Repo: opt.Repo, Home: opt.Home, GOOS: opt.GOOS})
	if err != nil {
		t.Fatal(err)
	}
	r := install.New(install.Options{Home: opt.Home, Symlinks: true, Now: opt.Now})
	for _, a := range p.SelectedActions() {
		if _, err := r.Apply(a); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestUninstallLeavesHomeIdenticalToBeforeInstall(t *testing.T) {
	opt := machine(t)
	before := treeOf(t, opt.Home)
	installAll(t, opt)
	if treeOf(t, opt.Home) == before {
		t.Fatal("install changed nothing; setup is wrong")
	}

	items, err := Run(opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(items) == 0 {
		t.Error("no items reported")
	}
	if after := treeOf(t, opt.Home); after != before {
		t.Errorf("home differs after uninstall\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestUninstallLeavesForeignContentAlone(t *testing.T) {
	opt := machine(t)
	installAll(t, opt)
	// The user later replaced two installed links with their own things.
	own := filepath.Join(opt.Home, ".tmux.conf")
	_ = os.Remove(own)
	if err := os.Symlink("/user/elsewhere", own); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(opt.Home, ".config/tmux")
	_ = os.Remove(dir)
	put(t, filepath.Join(dir, "mine.conf"), "user dir")

	if _, err := Run(opt); err != nil {
		t.Fatal(err)
	}
	if tg, _ := os.Readlink(own); tg != "/user/elsewhere" {
		t.Errorf("foreign link was touched: %q", tg)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "mine.conf")); string(b) != "user dir" {
		t.Error("user directory was touched")
	}
	if b, _ := os.ReadFile(filepath.Join(opt.Home, ".keep/me")); string(b) != "foreign" {
		t.Error("unrelated file was touched")
	}
	if b, _ := os.ReadFile(filepath.Join(opt.Home, ".zshrc")); string(b) != "my zshrc\n" {
		t.Errorf(".zshrc = %q, want the original restored", b)
	}
}

func TestUninstallDryRunChangesNothing(t *testing.T) {
	opt := machine(t)
	installAll(t, opt)
	before := treeOf(t, opt.Home)
	opt.DryRun = true
	items, err := Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	if treeOf(t, opt.Home) != before {
		t.Error("dry run changed the tree")
	}
	var planned bool
	for _, it := range items {
		if strings.HasPrefix(it.Action, "would") {
			planned = true
		}
	}
	if !planned {
		t.Errorf("dry run reported no planned action: %+v", items)
	}
}

func TestUninstallRemovesOnlyTheRulesBlock(t *testing.T) {
	opt := machine(t)
	codex := filepath.Join(opt.Home, ".codex/AGENTS.md")
	gemini := filepath.Join(opt.Home, ".gemini/GEMINI.md")
	for _, p := range []string{codex, gemini} {
		applied, _ := avalsync.ApplyRules("shared rules", "my own notes\n")
		put(t, p, applied+"trailing user text\n")
	}
	foreign := filepath.Join(opt.Home, ".codex/config.toml")
	put(t, foreign, "keep")

	if _, err := Run(opt); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{codex, gemini} {
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), "aval:agent-rules") || strings.Contains(string(b), "shared rules") {
			t.Errorf("%s still holds the aval block:\n%s", p, b)
		}
		if !strings.Contains(string(b), "my own notes") || !strings.Contains(string(b), "trailing user text") {
			t.Errorf("%s lost user content:\n%s", p, b)
		}
	}
	if b, _ := os.ReadFile(foreign); string(b) != "keep" {
		t.Error("an unrelated file was touched")
	}
}

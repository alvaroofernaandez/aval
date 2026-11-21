package sync

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/backup"
)

// treeOf describes a directory byte for byte, ignoring the backup store.
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
			lines = append(lines, fmt.Sprintf("D %s", rel))
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

func TestRunRecordsABackupRunThatRestoresEverything(t *testing.T) {
	opt := overlayLinks(t)
	write(t, filepath.Join(opt.Home, ".claude/settings.json"), `{"model":"sonnet"}`)
	write(t, filepath.Join(opt.Home, ".codex/AGENTS.md"), "mine\n")
	write(t, filepath.Join(opt.Home, ".claude/skills/secret/SKILL.md"), "mine")
	before := treeOf(t, opt.Home)

	if _, err := Run(opt); err != nil {
		t.Fatal(err)
	}
	if treeOf(t, opt.Home) == before {
		t.Fatal("sync changed nothing; the test setup is wrong")
	}

	store := backup.NewStore(opt.Home)
	run, err := store.Resolve("latest")
	if err != nil {
		t.Fatalf("sync left no backup run: %v", err)
	}
	if _, err := store.Restore(run, backup.RestoreOptions{Repo: opt.Repo}); err != nil {
		t.Fatal(err)
	}
	if after := treeOf(t, opt.Home); after != before {
		t.Errorf("home differs after restore\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

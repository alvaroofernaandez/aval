package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ~/.claude/settings.json is owned by `aval sync`: the installer seeds a real
// file, never a link, so sync can never write personal keys into the repo.
func TestSeedClaudeSettings(t *testing.T) {
	repo := t.TempDir()
	src := filepath.Join(repo, "config/claude/settings.json")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte(`{"hooks":{}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := map[string]func(home string){
		"missing": func(string) {},
		"old link": func(home string) {
			os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
			os.Symlink(src, filepath.Join(home, ".claude/settings.json"))
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			setup(home)
			if err := seedClaudeSettings(repo, home, false); err != nil {
				t.Fatal(err)
			}
			dst := filepath.Join(home, ".claude/settings.json")
			fi, err := os.Lstat(dst)
			if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
				t.Fatalf("want a regular file, got %v (err %v)", fi, err)
			}
		})
	}

	t.Run("existing file is kept", func(t *testing.T) {
		home := t.TempDir()
		dst := filepath.Join(home, ".claude/settings.json")
		os.MkdirAll(filepath.Dir(dst), 0o755)
		os.WriteFile(dst, []byte(`{"mine":true}`), 0o644)
		if err := seedClaudeSettings(repo, home, false); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(dst); string(b) != `{"mine":true}` {
			t.Fatalf("existing settings were overwritten: %s", b)
		}
	})

	t.Run("dry run writes nothing", func(t *testing.T) {
		home := t.TempDir()
		if err := seedClaudeSettings(repo, home, true); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(home, ".claude/settings.json")); err == nil {
			t.Fatal("dry run created the file")
		}
	})
}

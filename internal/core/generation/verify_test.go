package generation_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

// Files are read-only, dirs are not, so a plain rm deletes a generation; the
// content hash, not the permissions, is what proves it unchanged (owner: G8 = A).
func TestAGenerationIsDeletedByAPlainRemove(t *testing.T) {
	home := t.TempDir()
	g, _ := build(t, home, cfg("one"), t0)
	if err := os.RemoveAll(filepath.Join(generation.Root(home), g.ID)); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := generation.Read(home, g.ID); !errors.Is(err, generation.ErrUnknown) {
		t.Errorf("Read after remove = %v, want ErrUnknown", err)
	}
}

func TestVerifyDetectsATamperedGeneration(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"edited file": func(t *testing.T, dir string) {
			p := firstFile(t, dir)
			_ = os.Chmod(p, 0o644)
			if err := os.WriteFile(p, []byte("edited"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"added file": func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(filepath.Dir(firstFile(t, dir)), "extra.md"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"removed file": func(t *testing.T, dir string) {
			if err := os.Remove(firstFile(t, dir)); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			g, _ := build(t, home, cfg("one"), t0)
			if err := generation.Verify(home, g.ID); err != nil {
				t.Fatalf("Verify of an untouched generation = %v", err)
			}
			tamper(t, filepath.Join(generation.Root(home), g.ID))
			if err := generation.Verify(home, g.ID); !errors.Is(err, generation.ErrTampered) {
				t.Errorf("Verify = %v, want ErrTampered", err)
			}
		})
	}
}

// firstFile is a rendered file of the generation in dir.
func firstFile(t *testing.T, dir string) string {
	t.Helper()
	var found string
	_ = filepath.WalkDir(filepath.Join(dir, "fake"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && found == "" {
			found = p
		}
		return nil
	})
	if found == "" {
		t.Fatal("no rendered file")
	}
	return found
}

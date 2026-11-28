package generation

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CurrentName is the pointer to the active generation, inside Root.
const CurrentName = "current"

// Current returns the id of the active generation, "" when none is active.
// The pointer is a relative symlink (current -> gen-...) where the OS allows
// one, or a file holding the id (Windows without developer mode).
func Current(home string) (string, error) {
	p := filepath.Join(Root(home), CurrentName)
	st, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("generation: %w", err)
	}
	var id string
	if st.Mode()&fs.ModeSymlink != 0 {
		id, err = os.Readlink(p)
	} else {
		var b []byte
		b, err = os.ReadFile(p)
		id = strings.TrimSpace(string(b))
	}
	if err != nil {
		return "", fmt.Errorf("generation: %w", err)
	}
	if _, err := Read(home, id); err != nil {
		return id, fmt.Errorf("generation: current points at %w", err)
	}
	return id, nil
}

// SetCurrent makes id the active generation in one atomic step: the new
// pointer is made aside and renamed over the old one, so a crash leaves
// either the old or the new generation active, never neither.
func SetCurrent(home, id string) error {
	if _, err := Read(home, id); err != nil {
		return err
	}
	root := Root(home)
	tmp, err := os.CreateTemp(root, ".current-")
	if err != nil {
		return fmt.Errorf("generation: %w", err)
	}
	name := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(name)
	if err := pointer(name, id); err != nil {
		return fmt.Errorf("generation: %w", err)
	}
	if err := os.Rename(name, filepath.Join(root, CurrentName)); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("generation: %w", err)
	}
	clearLeftovers(root)
	return nil
}

// pointer makes the aside pointer: a symlink, or on Windows a file holding
// the id, because a directory symlink there needs developer mode and cannot
// be replaced by a rename.
func pointer(name, id string) error {
	if runtime.GOOS != "windows" {
		return os.Symlink(id, name)
	}
	return os.WriteFile(name, []byte(id+"\n"), 0o644)
}

// clearLeftovers removes the aside pointers of switches that crashed.
func clearLeftovers(root string) {
	ents, _ := os.ReadDir(root)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".current-") {
			_ = os.Remove(filepath.Join(root, e.Name()))
		}
	}
}

// Dir is the tree adapter rendered in generation id.
func Dir(home, id, adapter string) string { return filepath.Join(Root(home), id, adapter) }

// ActiveDir is Dir of the active generation, "" when none is active.
func ActiveDir(home, adapter string) (string, error) {
	id, err := Current(home)
	if id == "" || err != nil {
		return "", err
	}
	return Dir(home, id, adapter), nil
}

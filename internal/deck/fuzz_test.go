package deck

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"
)

// FuzzPlanGatePath checks the plan gate against an independent oracle: with an
// active plan and no deck, every file inside the repo that is not under odd/ or
// .git/ and is not a deck spec must be denied.
func FuzzPlanGatePath(f *testing.F) {
	for _, s := range []string{"..notes.go", "src/a.go", "odd/x.md", "a/../b.go", "x.deck.json"} {
		f.Add(s)
	}
	root, err := filepath.EvalSymlinks(f.TempDir())
	if err != nil {
		f.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		f.Fatal(err)
	}
	plan := filepath.Join(root, "odd", "tasks", "feature.md")
	if err := os.MkdirAll(filepath.Dir(plan), 0o755); err != nil {
		f.Fatal(err)
	}
	if err := os.WriteFile(plan, []byte(openPlan), 0o644); err != nil {
		f.Fatal(err)
	}
	cache := f.TempDir()
	now := time.Now().Add(time.Hour)

	f.Fuzz(func(t *testing.T, name string) {
		if !portableRelName(name) {
			t.Skip()
		}
		if !oracleMustDeny(root, name) {
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"tool_name":  "Edit",
			"cwd":        root,
			"tool_input": map[string]string{"file_path": name},
		})
		var out bytes.Buffer
		Gate(bytes.NewReader(payload), &out, GateOptions{Cache: cache, Now: now})
		if !bytes.Contains(out.Bytes(), []byte(`"permissionDecision":"deny"`)) {
			t.Fatalf("%q is inside the repo and must be denied, gate wrote %q", name, out.String())
		}
	})
}

// portableRelName keeps names that mean the same relative path on every OS.
func portableRelName(name string) bool {
	if name == "" || !utf8.ValidString(name) || filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) || strings.ContainsRune(`\:*?"<>|`, r) {
			return false
		}
	}
	return true
}

// oracleMustDeny decides from the cleaned path alone, without the gate's code.
func oracleMustDeny(root, name string) bool {
	target := filepath.Clean(filepath.Join(root, name))
	prefix := root + string(filepath.Separator)
	if !strings.HasPrefix(target, prefix) {
		return false // the root itself or outside it
	}
	rel := filepath.ToSlash(target[len(prefix):])
	return !strings.HasPrefix(rel, "odd/") && !strings.HasPrefix(rel, ".git/") && !strings.HasSuffix(rel, ".deck.json")
}

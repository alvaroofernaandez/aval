// Package herdrmods tests the herdr-app mods versioned in config/herdr-app/mods.
// The mods are JavaScript because that is herdr-app's mod platform; their pure
// logic carries node:test unit tests, run from here so `go test ./...` covers them.
package herdrmods

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestDiffReviewModIsComplete(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "config", "herdr-app", "mods", "diff-review")
	for _, f := range []string{"main.js", "logic.js", "logic.test.js", "mod.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
}

func TestDiffReviewModIsInstalledByTheManifest(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "install.manifest"))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^config/herdr-app/mods/diff-review\s+\.config/herdr-app/mods/diff-review$`)
	if !re.Match(b) {
		t.Error("install.manifest does not link config/herdr-app/mods/diff-review")
	}
}

func TestDiffReviewLogic(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir := filepath.Join(repoRoot(t), "config", "herdr-app", "mods", "diff-review")
	out, err := exec.Command(node, "--test", filepath.Join(dir, "logic.test.js")).CombinedOutput()
	if err != nil {
		t.Fatalf("node --test failed: %v\n%s", err, out)
	}
}

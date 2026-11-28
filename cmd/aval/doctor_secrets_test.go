package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/doctor"
	"github.com/alvaroofernaandez/aval/internal/core/secrets"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

const tokenSentinel = "plaintext-doctor-sentinel"

func TestFixSecretsKeepsTheTokenOutOfEveryFileAndOutput(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	mcp := filepath.Join(repo, ".mcp.json")
	if err := os.WriteFile(mcp, []byte(`{"mcpServers":{"gh":{"command":"npx","env":{"GITHUB_TOKEN":"`+tokenSentinel+`"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := secrets.NewMemory()
	var out bytes.Buffer
	if err := fixSecrets(doctor.Options{Repo: repo, Home: home}, store, home, &out, i18n.New("en")); err != nil {
		t.Fatal(err)
	}
	if v, _ := store.Get("gh.GITHUB_TOKEN"); v != tokenSentinel {
		t.Errorf("keychain holds %q", v)
	}
	if !strings.Contains(out.String(), "gh GITHUB_TOKEN") || !strings.Contains(out.String(), "aval secrets get gh.GITHUB_TOKEN --show") {
		t.Errorf("output lacks the moved key or the recovery path:\n%s", out.String())
	}
	leaks := []string{out.String()}
	_ = filepath.WalkDir(home, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			leaks = append(leaks, string(b))
		}
		return nil
	})
	b, _ := os.ReadFile(mcp)
	leaks = append(leaks, string(b))
	snaps := 0
	for _, l := range leaks {
		if strings.Contains(l, tokenSentinel) {
			t.Errorf("the token leaked:\n%s", l)
		}
		if strings.Contains(l, "<keychain:gh.GITHUB_TOKEN>") {
			snaps++
		}
	}
	if snaps != 1 {
		t.Errorf("want exactly one redacted snapshot under HOME, got %d", snaps)
	}
}

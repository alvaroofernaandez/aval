package doctor

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/secrets"
)

func migrationMachine(t *testing.T) (Options, string) {
	t.Helper()
	opt := secMachine(t)
	mcp := filepath.Join(opt.Repo, ".mcp.json")
	w(t, mcp, `{"mcpServers":{
	  "gh":{"command":"npx","args":["-y","gh-mcp"],"env":{"GITHUB_TOKEN":"`+secret+`","LOG_LEVEL":"debug"}},
	  "ok":{"command":"x","env":{"API_KEY":"${API_KEY}"}}}}`)
	return opt, mcp
}

func TestMigrateSecretsMovesTheValueToTheStore(t *testing.T) {
	opt, mcp := migrationMachine(t)
	store := secrets.NewMemory()
	var backed []string
	moved, err := MigrateSecrets(opt, store, func(p string, _ []byte) error {
		backed = append(backed, p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(mcp)
	if strings.Contains(string(b), secret) {
		t.Errorf("the value is still in the file:\n%s", b)
	}
	if v, err := store.Get("gh.GITHUB_TOKEN"); err != nil || v != secret {
		t.Errorf("store = %q, %v; want the value under gh.GITHUB_TOKEN", v, err)
	}
	for _, want := range []string{`"command": "aval"`, `"secrets"`, `"exec"`, `GITHUB_TOKEN=gh.GITHUB_TOKEN`, `"--"`, `"gh-mcp"`, `"LOG_LEVEL": "debug"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("rewritten file lacks %s:\n%s", want, b)
		}
	}
	if !slices.Equal(backed, []string{mcp}) {
		t.Errorf("backup hook calls = %v, want one for %s", backed, mcp)
	}
	if len(moved) != 1 || moved[0] != (Moved{"gh", "GITHUB_TOKEN", "gh.GITHUB_TOKEN"}) {
		t.Errorf("moved = %v, want one entry without the value", moved)
	}
}

func TestMigrateSecretsIsIdempotent(t *testing.T) {
	opt, mcp := migrationMachine(t)
	store := secrets.NewMemory()
	if _, err := MigrateSecrets(opt, store, func(string, []byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(mcp)
	calls := 0
	moved, err := MigrateSecrets(opt, store, func(string, []byte) error { calls++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(mcp)
	if string(first) != string(second) || len(moved) != 0 || calls != 0 {
		t.Errorf("second run changed something: moved=%v backups=%d", moved, calls)
	}
}

func TestMigrateSecretsStopsWhenTheBackupFails(t *testing.T) {
	opt, mcp := migrationMachine(t)
	before, _ := os.ReadFile(mcp)
	store := secrets.NewMemory()
	if _, err := MigrateSecrets(opt, store, func(string, []byte) error { return os.ErrPermission }); err == nil {
		t.Fatal("want the backup error")
	}
	after, _ := os.ReadFile(mcp)
	if string(before) != string(after) {
		t.Error("the file changed although the backup failed")
	}
}

func TestMigrateSecretsSnapshotNeverHoldsTheToken(t *testing.T) {
	opt, mcp := migrationMachine(t)
	var snap []byte
	_, err := MigrateSecrets(opt, secrets.NewMemory(), func(_ string, r []byte) error { snap = r; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(snap), secret) {
		t.Errorf("the snapshot holds the token:\n%s", snap)
	}
	if !strings.Contains(string(snap), "<keychain:gh.GITHUB_TOKEN>") || !strings.Contains(string(snap), `"command": "npx"`) {
		t.Errorf("snapshot should keep the original shape with a placeholder:\n%s", snap)
	}
	_ = mcp
}

func TestMigrateSecretsLeavesTheFileWhenTheKeychainCannotHoldTheValue(t *testing.T) {
	opt, mcp := migrationMachine(t)
	before, _ := os.ReadFile(mcp)
	if _, err := MigrateSecrets(opt, lossyStore{secrets.NewMemory()}, func(string, []byte) error { return nil }); err == nil {
		t.Fatal("want an error when the value does not read back")
	}
	if after, _ := os.ReadFile(mcp); string(before) != string(after) {
		t.Error("the file was rewritten although the keychain lost the value")
	}
}

// lossyStore accepts a Set and forgets it.
type lossyStore struct{ secrets.Store }

func (lossyStore) Set(string, string) error { return nil }

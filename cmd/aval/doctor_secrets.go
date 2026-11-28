package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/doctor"
	"github.com/alvaroofernaandez/aval/internal/core/secrets"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// fixSecrets is the secrets half of `aval doctor --security --fix`: inline MCP
// tokens move into the keychain. Before a file is rewritten its redacted copy
// (values replaced by <keychain:NAME>) is saved under ~/.aval-backup/secrets,
// so a backup never holds a token. Output names the key, never the value.
func fixSecrets(opt doctor.Options, store secrets.Store, home string, out io.Writer, tr i18n.Translator) error {
	dir := filepath.Join(home, ".aval-backup", "secrets", time.Now().UTC().Format("20060102T150405Z"))
	n := 0
	save := func(path string, redacted []byte) error {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		n++
		return os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d-%s", n, filepath.Base(path))), redacted, 0o600)
	}
	moved, err := doctor.MigrateSecrets(opt, store, save)
	for _, m := range moved {
		fmt.Fprintln(out, tr.T("cli.doctor.secret.moved", i18n.Args{"server": m.Server, "key": m.Key, "name": m.Name}))
	}
	if len(moved) > 0 {
		fmt.Fprintln(out, tr.T("cli.doctor.secret.recover", i18n.Args{"dir": short(home, dir)}))
	}
	return err
}

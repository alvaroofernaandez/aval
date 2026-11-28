package doctor

// Secret migration (aval doctor --security --fix). A stdio MCP server with a
// literal token in env gets the value copied into the keychain and its entry
// rewritten to `aval secrets exec VAR=NAME -- <original command>`. Headers and
// flag arguments are reported by the check but not rewritten: there is no
// process to wrap. Values never appear in a returned message.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/secrets"
)

type secretSet struct{ name, value string }

// Moved is one env key that went to the keychain. It carries no value.
type Moved struct{ Server, Key, Name string }

// MigrateSecrets rewrites every config file that holds an inline env secret.
// before is called with the file's path and a redacted copy of it (every secret
// value replaced by "<keychain:NAME>") just before it is written, so the caller
// can keep that as the backup, which never holds a token; if it fails nothing
// is written. Each value is stored and read back before the file is rewritten. The result names what moved
// as server, key and keychain name, never a value. A second run finds nothing to do.
func MigrateSecrets(opt Options, store secrets.Store, before func(path string, redacted []byte) error) ([]Moved, error) {
	var moved []Moved
	for _, path := range mcpFiles(opt) {
		doc, err := readJSONNumbers(path)
		if err != nil {
			continue
		}
		redacted, err := redactedCopy(doc)
		if err != nil {
			return moved, err
		}
		var sets []secretSet
		var names []Moved
		eachServer(doc, func(name string, srv map[string]any) {
			s, n := migrateServer(name, srv, false)
			sets, names = append(sets, s...), append(names, n...)
		})
		if len(sets) == 0 {
			continue
		}
		if err := before(path, redacted); err != nil {
			return moved, fmt.Errorf("doctor: back up %s: %w", path, err)
		}
		for _, s := range sets {
			if err := store.Set(s.name, s.value); err != nil {
				return moved, fmt.Errorf("doctor: store %s: %w", s.name, err)
			}
			if got, err := store.Get(s.name); err != nil || got != s.value {
				return moved, fmt.Errorf("doctor: %s did not read back from the keychain; %s left untouched", s.name, path)
			}
		}
		if err := writeJSONAtomic(path, doc); err != nil {
			return moved, err
		}
		moved = append(moved, names...)
	}
	return moved, nil
}

// mcpFiles lists the files that can declare MCP servers, in a stable order.
func mcpFiles(opt Options) []string {
	files := map[string]bool{filepath.Join(opt.Repo, ".mcp.json"): true}
	claudeJSON := filepath.Join(opt.Home, ".claude.json")
	if m, err := readJSON(claudeJSON); err == nil {
		files[claudeJSON] = true
		projects, _ := m["projects"].(map[string]any)
		for p := range projects {
			files[filepath.Join(p, ".mcp.json")] = true
		}
	}
	out := make([]string, 0, len(files))
	for f := range files {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// eachServer visits the servers of a document: its own and each project's.
func eachServer(doc map[string]any, visit func(name string, srv map[string]any)) {
	each := func(servers map[string]any) {
		names := make([]string, 0, len(servers))
		for n := range servers {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			if srv, ok := servers[n].(map[string]any); ok {
				visit(n, srv)
			}
		}
	}
	top, _ := doc["mcpServers"].(map[string]any)
	each(top)
	projects, _ := doc["projects"].(map[string]any)
	for _, p := range projects {
		pm, _ := p.(map[string]any)
		ps, _ := pm["mcpServers"].(map[string]any)
		each(ps)
	}
}

// migrateServer rewrites one stdio server in place and returns the secrets to
// store and the "server KEY" labels.
func migrateServer(server string, srv map[string]any, redact bool) ([]secretSet, []Moved) {
	cmd, _ := srv["command"].(string)
	env, _ := srv["env"].(map[string]any)
	if cmd == "" || len(env) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sets []secretSet
	var maps []string
	var labels []Moved
	for _, k := range keys {
		v, _ := env[k].(string)
		if v == "" || !literalSecret(k, v) {
			continue
		}
		name := secretNameFor(server, k)
		sets = append(sets, secretSet{name, v})
		maps = append(maps, k+"="+name)
		labels = append(labels, Moved{server, k, name})
		if redact {
			env[k] = "<keychain:" + name + ">"
		} else {
			delete(env, k)
		}
	}
	if len(sets) == 0 || redact {
		return sets, labels
	}
	if len(env) == 0 {
		delete(srv, "env")
	}
	args, _ := srv["args"].([]any)
	if cmd == "aval" && len(args) >= 4 && args[0] == "secrets" && args[1] == "exec" {
		args[2] = fmt.Sprint(args[2]) + "," + strings.Join(maps, ",")
		srv["args"] = args
		return sets, labels
	}
	wrapped := []any{"secrets", "exec", strings.Join(maps, ","), "--", cmd}
	srv["args"] = append(wrapped, args...)
	srv["command"] = "aval"
	return sets, labels
}

// secretNameFor is the keychain name of a server's env key: legal characters only.
func secretNameFor(server, key string) string {
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			if secrets.ValidName(string(r)) || r == '-' {
				return r
			}
			return '_'
		}, s)
	}
	return strings.TrimLeft(clean(server)+"."+clean(key), "-")
}

func readJSONNumbers(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// writeJSONAtomic writes next to the file and renames over it, keeping its mode.
func writeJSONAtomic(path string, doc map[string]any) error {
	b, err := marshalIndent(doc)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".aval-secrets-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// redactedCopy is doc as JSON with each migratable secret replaced by a
// "<keychain:NAME>" placeholder, on a deep copy: doc is untouched.
func redactedCopy(doc map[string]any) ([]byte, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var cp map[string]any
	if err := dec.Decode(&cp); err != nil {
		return nil, err
	}
	eachServer(cp, func(name string, srv map[string]any) { migrateServer(name, srv, true) })
	return marshalIndent(cp)
}

// marshalIndent keeps <, > and & as they are (a URL with & must not turn into
// &) and ends with a newline.
func marshalIndent(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

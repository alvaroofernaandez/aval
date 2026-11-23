package tests

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// brewLine is a formula or cask line of the Brewfile; the name is the last
// segment of a tapped name ("gentleman-programming/tap/engram" is engram).
var brewLine = regexp.MustCompile(`^(?:brew|cask)\s+"([^"]+)"`)

// inventoryNames is every name in internal/core/deps/deps.manifest: the
// sections and the bare lines of [personal]. tests/ imports nothing internal
// (ADR 0002), so the file is read as text.
func inventoryNames(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	section := ""
	for _, line := range strings.Split(read(t, "internal/core/deps/deps.manifest"), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			section = line[1 : len(line)-1]
			if section != "personal" {
				names[section] = true
			}
		case section == "personal":
			names[line] = true
		}
	}
	return names
}

func TestDepsManifestCoversTheBrewfile(t *testing.T) {
	names := inventoryNames(t)
	var missing []string
	seen := 0
	for _, line := range strings.Split(read(t, "Brewfile"), "\n") {
		m := brewLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		seen++
		name := m[1][strings.LastIndex(m[1], "/")+1:]
		if !names[name] {
			missing = append(missing, name)
		}
	}
	if seen == 0 {
		t.Fatal("no brew or cask line read from the Brewfile")
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("Brewfile tools neither in deps.manifest nor in its [personal] list:\n  %s", strings.Join(missing, "\n  "))
	}
}

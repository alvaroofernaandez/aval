package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// install.manifest is the single source of truth for what gets installed and
// where. The contract: it parses, every group declares a platform and a label,
// every source exists, and the Go installer installs exactly what it says.

type manifestEntry struct{ src, dest, group string }

type manifestDoc struct {
	groups       []string
	platforms    map[string]string
	labels       map[string]string
	entries      []manifestEntry
	fanoutSource string
	fanoutGroup  string
	fanoutDests  []string
}

// applies reports whether a group is installed on this OS: Windows takes only
// the platforms = all groups.
func (m manifestDoc) applies(group string) bool {
	return runtime.GOOS != "windows" || m.platforms[group] == "all"
}

var (
	reSection = regexp.MustCompile(`^\[([a-z-]+)\]$`)
	reKV      = regexp.MustCompile(`^([a-z-]+)\s*=\s*(.*)$`)
)

func parseManifest(text string) manifestDoc {
	m := manifestDoc{platforms: map[string]string{}, labels: map[string]string{}}
	cur := ""
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if s := reSection.FindStringSubmatch(line); s != nil {
			cur = s[1]
			m.groups = append(m.groups, cur)
			continue
		}
		if kv := reKV.FindStringSubmatch(line); kv != nil {
			k, v := kv[1], strings.TrimSpace(kv[2])
			switch k {
			case "platforms":
				m.platforms[cur] = strings.Fields(v + " ")[0]
			case "label":
				m.labels[cur] = v
			case "fanout-source":
				m.fanoutSource = v
				m.fanoutGroup = cur
			case "fanout-dests":
				m.fanoutDests = strings.Fields(v)
			}
			continue
		}
		if f := strings.Fields(line); len(f) == 2 {
			m.entries = append(m.entries, manifestEntry{f[0], f[1], cur})
		}
	}
	return m
}

func TestManifestEveryGroupDeclaresPlatformAndLabel(t *testing.T) {
	m := parseManifest(read(t, "install.manifest"))
	isTrue(t, "declares at least one group", len(m.groups) >= 1)
	for _, g := range m.groups {
		if pl := m.platforms[g]; pl != "unix" && pl != "all" {
			t.Errorf("group [%s] has platform %q (want unix or all)", g, pl)
		}
		if m.labels[g] == "" {
			t.Errorf("group [%s] has no label for the TUI", g)
		}
	}
}

// A manifest entry naming a renamed or deleted path makes the installer fail on
// the user's machine, halfway through installing.
func TestManifestEverySourceExists(t *testing.T) {
	m := parseManifest(read(t, "install.manifest"))
	isTrue(t, "manifest has entries", len(m.entries) > 0)
	for _, e := range m.entries {
		if !exists(p(e.src)) {
			t.Errorf("missing source: %s", e.src)
		}
	}
}

// A source that exists only in one checkout (an empty or ignored directory)
// is missing from every fresh clone: CI caught config/opencode/plugins, an
// empty local folder, failing `aval --yes` on a clean runner.
func TestManifestEverySourceIsTracked(t *testing.T) {
	m := parseManifest(read(t, "install.manifest"))
	for _, e := range m.entries {
		r := run(repo, nil, "", "git", "ls-files", "--", e.src)
		if r.Code == 0 && strings.TrimSpace(r.Out()) == "" {
			t.Errorf("manifest source not tracked by git (missing in a clean clone): %s", e.src)
		}
	}
}

func TestManifestFanoutSourceDirectoryExists(t *testing.T) {
	m := parseManifest(read(t, "install.manifest"))
	isTrue(t, "fanout-source is declared and a directory", m.fanoutSource != "" && isDir(p(m.fanoutSource)))
}

// The heart of it: `aval --yes --dry-run` prints one destination per line; the
// manifest plus its fan-out must expand to exactly that set.
func TestManifestMatchesWhatTheInstallerInstalls(t *testing.T) {
	aval := avalBin(t)
	m := parseManifest(read(t, "install.manifest"))

	want := map[string]bool{}
	for _, e := range m.entries {
		if m.applies(e.group) {
			want[e.dest] = true
		}
	}
	if m.fanoutSource != "" && m.applies(m.fanoutGroup) {
		for _, d := range m.fanoutDests {
			for _, s := range subdirs(t, m.fanoutSource) {
				want[d+"/"+s] = true
			}
		}
	}

	home := t.TempDir()
	r := run(repo, isoEnv(home), "", aval, "install", "--yes", "--dry-run", "--repo", repo)
	got := map[string]bool{}
	for _, d := range regexp.MustCompile(`~/\S+`).FindAllString(r.Stdout, -1) {
		got[strings.TrimPrefix(d, "~/")] = true
	}

	var missing, extra []string
	for d := range want {
		if !got[d] {
			missing = append(missing, d)
		}
	}
	for d := range got {
		if !want[d] {
			extra = append(extra, d)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing)+len(extra) > 0 {
		t.Errorf("installer and manifest disagree\n  declared but not installed: %v\n  installed but not declared: %v", missing, extra)
	}
	floor := 40
	if runtime.GOOS == "windows" {
		floor = 20 // the terminal groups are not installed there
	}
	isTrue(t, "the destination count is non-trivial", len(got) >= floor)
	if _, err := os.Stat(filepath.Join(home, ".claude")); err == nil {
		t.Errorf("--dry-run created %s/.claude", home)
	}
}

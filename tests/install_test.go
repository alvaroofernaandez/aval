package tests

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// End-to-end tests of the Go installer (`aval --yes`). Every run happens
// inside a throwaway $HOME, so the real dotfiles of whoever runs the suite are
// never touched. Unit-level cases live in internal/core/install and cmd/aval.

const backupRootName = ".aval-backup"

type sandbox struct {
	t    *testing.T
	home string
	repo string
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	return &sandbox{t: t, home: t.TempDir(), repo: repo}
}

func (s *sandbox) install(args ...string) result {
	s.t.Helper()
	a := append([]string{"--yes", "--repo", s.repo}, args...)
	return run(s.repo, isoEnv(s.home, withHookTools(s.t)), "", avalBin(s.t), a...)
}

func (s *sandbox) path(rel string) string { return filepath.Join(s.home, rel) }

func (s *sandbox) readlink(rel string) string {
	l, err := os.Readlink(s.path(rel))
	if err != nil {
		return "<" + err.Error() + ">"
	}
	return l
}

func (s *sandbox) isLink(rel string) bool {
	st, err := os.Lstat(s.path(rel))
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

func (s *sandbox) cat(rel string) string {
	b, err := os.ReadFile(s.path(rel))
	if err != nil {
		return "<" + err.Error() + ">"
	}
	return strings.TrimSpace(string(b))
}

// countBackups mirrors the original `find -mindepth 2 -maxdepth 4`.
func (s *sandbox) countBackups() int { return len(s.listBackups()) }

// listBackups names the backed-up items (relative to the backup root).
func (s *sandbox) listBackups() []string {
	root := s.path(backupRootName)
	var items []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		depth := strings.Count(rel, string(filepath.Separator)) + 1
		if depth >= 2 && depth <= 4 && d.Name() != "manifest.json" {
			items = append(items, rel)
		}
		return nil
	})
	return items
}

// backupOf finds a backed-up item whatever timestamp it got.
func (s *sandbox) backupOf(rel string) string {
	root := s.path(backupRootName)
	var found string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		r, _ := filepath.Rel(root, path)
		if strings.Count(r, string(filepath.Separator)) <= 2 && strings.HasSuffix(r, string(filepath.Separator)+rel) {
			found = path
		}
		return nil
	})
	return found
}

func (s *sandbox) mkdir(rel string) { s.t.Helper(); _ = os.MkdirAll(s.path(rel), 0o755) }

func (s *sandbox) put(rel, body string) { s.t.Helper(); write(s.t, s.path(rel), body) }

func globCount(t *testing.T, dir, pattern string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil {
			if ok, _ := filepath.Match(pattern, d.Name()); ok {
				n++
			}
		}
		return nil
	})
	return n
}

func TestInstallCleanMachine(t *testing.T) {
	testenv.SkipWindows(t, "asserts the Unix terminal groups and symlinks; Windows installs the platforms=all groups in copy mode (TestInstallCopyMode*)")
	s := newSandbox(t)
	s.install()
	for dest, src := range map[string]string{
		".config/yazi":         "config/yazi",
		".config/yazi-sidebar": "config/yazi-sidebar",
		".config/ghostty":      "config/ghostty",
		".tmux.conf":           "home/tmux.conf",
		".claude/themes":       "config/claude/themes",
	} {
		eq(t, "links ~/"+dest+" to the repo", p(src), s.readlink(dest))
	}
	eq(t, "creates no backups on a clean machine", 0, s.countBackups())
	// The link must resolve to readable content, not just exist.
	_, err := os.ReadFile(s.path(".config/yazi/yazi.toml"))
	isTrue(t, "linked config is readable through the symlink", err == nil)
}

func TestInstallExistingConfigIsPreservedNeverDestroyed(t *testing.T) {
	testenv.SkipWindows(t, "asserts the Unix terminal groups and symlinks; Windows installs the platforms=all groups in copy mode (TestInstallCopyMode*)")
	s := newSandbox(t)
	s.put(".config/yazi/precious.conf", "MINE\n")
	s.put(".tmux.conf", "MY TMUX\n")
	s.install()

	eq(t, "replaces the existing directory with the link", p("config/yazi"), s.readlink(".config/yazi"))
	dir := s.backupOf(".config/yazi")
	isTrue(t, "backs up the previous directory", dir != "")
	eq(t, "backup keeps the original contents intact", "MINE",
		strings.TrimSpace(readAbsOrEmpty(dir+"/precious.conf")))
	eq(t, "backs up an existing regular file", "MY TMUX", strings.TrimSpace(readAbsOrEmpty(s.backupOf(".tmux.conf"))))

	// Claude Code reads every entry under ~/.claude/skills: a "ship.bak.<stamp>"
	// there would be loaded as a second, duplicate skill.
	n := 0
	for _, d := range []string{".config", ".claude", ".opencode"} {
		n += globCount(t, s.path(d), "*.bak.*")
	}
	eq(t, "quarantines backups outside the destination tree", 0, n)
}

func readAbsOrEmpty(path string) string {
	if path == "" {
		return ""
	}
	b, _ := os.ReadFile(path)
	return string(b)
}

func TestInstallIsIdempotent(t *testing.T) {
	testenv.SkipWindows(t, "asserts the Unix terminal groups and symlinks; Windows installs the platforms=all groups in copy mode (TestInstallCopyMode*)")
	s := newSandbox(t)
	s.install()
	before := s.countBackups()
	out := s.install()
	eq(t, "second run creates no new backups", before, s.countBackups())
	eq(t, "second run still leaves a valid link", p("config/yazi"), s.readlink(".config/yazi"))
	mustHave(t, `(?i)already|ok|skip`, out.Out(), "second run reports links as already in place")
}

func TestInstallDryRunTouchesNothing(t *testing.T) {
	testenv.SkipWindows(t, "asserts the Unix terminal groups and symlinks; Windows installs the platforms=all groups in copy mode (TestInstallCopyMode*)")
	s := newSandbox(t)
	s.put(".config/yazi/precious.conf", "UNTOUCHED\n")
	out := s.install("--dry-run")
	isTrue(t, "dry-run creates no symlink", !s.isLink(".config/yazi"))
	eq(t, "dry-run creates no backups", 0, s.countBackups())
	eq(t, "dry-run leaves existing files alone", "UNTOUCHED", s.cat(".config/yazi/precious.conf"))
	mustHave(t, `(?i)tmux`, out.Out(), "dry-run still reports what it would do")
}

// ~/.claude holds credentials and gigabytes of session history next to the
// configs worth versioning, so only individual subpaths may be linked.
func TestInstallAgentConfigsAreLinkedPerPathNeverWholesale(t *testing.T) {
	testenv.SkipWindows(t, "asserts the Unix terminal groups and symlinks; Windows installs the platforms=all groups in copy mode (TestInstallCopyMode*)")
	s := newSandbox(t)
	s.put(".claude/.credentials.json", "SECRET\n")
	s.put(".claude/projects/session.jsonl", "log\n")
	s.put(".claude/skills/some-vendor-skill/SKILL.md", "vendor\n")

	skills := subdirs(t, "skills")
	var firstOwn string // a real skill (not the _shared helper dir)
	for _, d := range skills {
		if !strings.HasPrefix(d, "_") {
			firstOwn = d
			break
		}
	}
	// Pre-seed an own skill as a real directory: the case that produced the
	// duplicate-skill bug, since it forces a backup right inside the scanned dir.
	s.put(".claude/skills/"+firstOwn+"/SKILL.md", "old copy\n")

	s.install()

	for _, root := range []string{".claude/skills", ".config/opencode/skills", ".opencode/skills"} {
		for _, name := range []string{skills[0], firstOwn} {
			eq(t, "links own skill "+name+" into ~/"+root, p("skills", name), s.readlink(root+"/"+name))
		}
	}

	links := 0
	ents, _ := os.ReadDir(s.path(".claude/skills"))
	for _, e := range ents {
		if e.Type()&os.ModeSymlink != 0 {
			links++
		}
	}
	eq(t, "links every own skill, not just one", len(skills), links)
	isTrue(t, "never replaces the skills directory itself", !s.isLink(".claude/skills"))
	eq(t, "no backup pollutes the skills directory", 0, func() int {
		n := 0
		ents, _ := os.ReadDir(s.path(".claude/skills"))
		for _, e := range ents {
			if strings.Contains(e.Name(), ".bak.") {
				n++
			}
		}
		return n
	}())
	// The whole point: marketplace skills must keep working after install.
	eq(t, "leaves third-party skills untouched", "vendor", s.cat(".claude/skills/some-vendor-skill/SKILL.md"))

	isTrue(t, "keeps tool-specific commands separate",
		s.readlink(".claude/commands") != s.readlink(".config/opencode/commands"))
	eq(t, "links ~/.claude/agents", p("config/claude/agents"), s.readlink(".claude/agents"))
	eq(t, "links ~/.claude/CLAUDE.md", p("config/claude/CLAUDE.md"), s.readlink(".claude/CLAUDE.md"))

	// settings.json is owned by `aval sync`, never linked: it keeps personal
	// keys and the private overlay, which must never be written back into the repo.
	st, err := os.Lstat(s.path(".claude/settings.json"))
	isTrue(t, "~/.claude/settings.json is a real file, not a link", err == nil && st.Mode().IsRegular())
	var live, repoCfg map[string]any
	if err := json.Unmarshal([]byte(readAbsOrEmpty(s.path(".claude/settings.json"))), &live); err != nil {
		t.Fatalf("seeded settings.json is not JSON: %v", err)
	}
	_ = json.Unmarshal([]byte(read(t, "config/claude/settings.json")), &repoCfg)
	isTrue(t, "...seeded with the repo's hooks", live["hooks"] != nil && reflect.DeepEqual(live["hooks"], repoCfg["hooks"]))
	eq(t, "the manifest does not link settings.json", 0, count(`\.claude/settings\.json`, read(t, "install.manifest")))

	isTrue(t, "never links ~/.claude itself", !s.isLink(".claude"))
	eq(t, "leaves credentials untouched", "SECRET", s.cat(".claude/.credentials.json"))
	eq(t, "leaves session history untouched", "log", s.cat(".claude/projects/session.jsonl"))

	eq(t, "links ~/.config/opencode/agents", p("config/opencode/agents"), s.readlink(".config/opencode/agents"))
	eq(t, "links ~/.config/opencode/opencode.json", p("config/opencode/opencode.json"), s.readlink(".config/opencode/opencode.json"))
	isTrue(t, "never links ~/.config/opencode itself", !s.isLink(".config/opencode"))
}

func TestInstallLeavesOpencodeNodeModulesInPlace(t *testing.T) {
	s := newSandbox(t)
	s.put(".config/opencode/node_modules/marker", "dep\n")
	s.install()
	eq(t, "leaves opencode node_modules in place", "dep", s.cat(".config/opencode/node_modules/marker"))
}

func TestInstallMissingSourceFailsAndNamesThePath(t *testing.T) {
	s := newSandbox(t)
	notRepo := s.path("not-a-repo")
	write(t, notRepo+"/install.manifest", read(t, "install.manifest"))
	s.repo = notRepo
	r := s.install()
	isTrue(t, "fails when the source tree is missing", r.Code != 0)
	mustHave(t, `(?i)not-a-repo|missing|no such`, r.Out(), "says which path was missing")
}

// What install.sh used to do after linking: core.hooksPath turns the versioned
// pre-push hook on, in a git checkout only.
func TestInstallEnablesVersionedGitHooksInACheckout(t *testing.T) {
	needTool(t, "git")
	s := newSandbox(t)
	clone := s.path("clone")
	copyRepo(t, clone)
	git(t, clone, "init", "-q")
	_ = run(clone, nil, "", "git", "config", "--unset", "core.hooksPath")
	run(repo, isoEnv(s.home), "", avalBin(t), "--yes", "--repo", clone)
	eq(t, "enables the versioned git hooks in a checkout", ".githooks", git(t, clone, "config", "--get", "core.hooksPath"))
}

func TestInstallDryRunLeavesHooksPathAndSettingsAlone(t *testing.T) {
	needTool(t, "git")
	s := newSandbox(t)
	clone := s.path("clone")
	copyRepo(t, clone)
	git(t, clone, "init", "-q")
	run(repo, isoEnv(s.home), "", avalBin(t), "--yes", "--dry-run", "--repo", clone)
	r := run(clone, nil, "", "git", "config", "--get", "core.hooksPath")
	eq(t, "dry-run leaves core.hooksPath alone", "", strings.TrimSpace(r.Stdout))
	isTrue(t, "dry-run seeds no settings", !exists(s.path(".claude/settings.json")))
}

// copyRepo copies the working tree (not .git, not node_modules) to dst.
func copyRepo(t *testing.T, dst string) {
	t.Helper()
	err := filepath.WalkDir(repo, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(repo, path)
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type()&os.ModeSymlink != 0:
			l, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(l, target)
		default:
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			info, _ := d.Info()
			return os.WriteFile(target, b, info.Mode().Perm())
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Copy mode is what Windows without symlink rights gets, and the only mode a
// Windows runner can be relied on for. Forcing it with --copy makes every OS
// exercise the same path: platforms = all groups become real files, a rerun
// changes nothing, and nothing is ever deleted.
func TestInstallCopyModeProducesRealFilesAndIsIdempotent(t *testing.T) {
	s := newSandbox(t)
	s.put(".claude/skills/some-vendor-skill/SKILL.md", "vendor\n")
	r := s.install("--copy")
	eq(t, "exit code of install --copy", 0, r.Code)

	for _, dest := range []string{".claude/agents", ".claude/CLAUDE.md", ".config/opencode/agents", ".gitconfig"} {
		isTrue(t, "~/"+dest+" is a real file or directory, not a link", exists(s.path(dest)) && !s.isLink(dest))
	}
	isTrue(t, "~/.claude/settings.json is a real file", exists(s.path(".claude/settings.json")))
	eq(t, "agents are copied byte for byte", read(t, "config/claude/CLAUDE.md"), readAbsOrEmpty(s.path(".claude/CLAUDE.md")))
	eq(t, "leaves third-party skills untouched", "vendor", s.cat(".claude/skills/some-vendor-skill/SKILL.md"))

	before := s.listBackups()
	again := s.install("--copy")
	eq(t, "exit code of the second run", 0, again.Code)
	if after := s.listBackups(); len(after) != len(before) {
		t.Errorf("a second run displaces nothing: backups %v -> %v\n%s", before, after, again.Out())
	}
	mustHave(t, `(?i)already installed`, again.Out(), "the second run reports the copies as already installed")
}

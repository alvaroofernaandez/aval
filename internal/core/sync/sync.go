// Package sync keeps the live agent configuration aligned with the repo.
//
// Claude Code and other agent tools rewrite ~/.claude/settings.json, and it holds
// personal keys that cannot be published, so it is a real file rather than a
// link to the repo. sync is how the repo-owned part of it stays authoritative:
// it replaces or merges only the managed keys and leaves everything else alone.
package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/claude"
	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/core/manifest"
	"github.com/alvaroofernaandez/aval/internal/core/plan"
)

// Options configures a run. Everything is a parameter so tests can point at a
// fake HOME and a fake repo.
type Options struct {
	Repo       string
	Home       string
	PrivateDir string
	DryRun     bool
	// GOOS selects the hook flavour (see ForOS); empty means runtime.GOOS.
	GOOS string
	// AvalBin is the executable Windows hooks run; empty means ResolveAvalBin.
	AvalBin string
	// Now stamps the backup directory; nil means time.Now.
	Now func() time.Time
	// Recorder receives everything sync overwrites or creates. Nil means Run
	// opens its own backup run and commits it; a caller that passes one (the
	// transactional sync) owns the commit.
	Recorder *backup.Recorder
	// Generation is the directory of a generation's Claude Code rendering
	// (<generation>/claude-code). When set, the managed settings come from
	// its settings.json instead of the repo.
	Generation string
}

// SettingsResult describes what happened to ~/.claude/settings.json.
type SettingsResult = claude.SettingsResult

// RulesResult describes one agent-rules target.
type RulesResult struct {
	Path    string
	Changed bool
	Skipped bool
}

// Report is the outcome of Run.
type Report struct {
	Settings SettingsResult
	Rules    []RulesResult
	Links    []LinkResult
}

// adapter is the Claude Code adapter for opt.
func (opt Options) adapter() *claude.Adapter {
	return claude.New(claude.Options{GOOS: opt.GOOS, AvalBin: opt.AvalBin, Home: opt.Home, PrivateDir: opt.PrivateDir})
}

func (opt Options) claudeDir() string { return filepath.Join(opt.Home, ".claude") }

// repoModel is the settings part of the model, read from the repo.
func repoModel(opt Options) (agentcfg.Config, error) {
	cfg, err := agentcfg.LoadSettings(filepath.Join(opt.Repo, "config", "claude", "settings.json"))
	if err != nil {
		return cfg, fmt.Errorf("sync: repo settings: %w", err)
	}
	return cfg, nil
}

// RepoSettings is the repo's settings.json as it must look on opt.GOOS: the
// Claude adapter's rendering, with hooks and status line rewritten for
// Windows and untouched elsewhere.
func RepoSettings(opt Options) (map[string]any, error) {
	if opt.Generation != "" {
		_, r, err := render(opt)
		if err != nil {
			return nil, err
		}
		return agentcfg.DecodeJSON(r.Files[0].Data)
	}
	cfg, err := repoModel(opt)
	if err != nil {
		return nil, err
	}
	return opt.adapter().Settings(cfg), nil
}

func render(opt Options) (*claude.Adapter, agentcfg.Rendered, error) {
	if opt.Generation != "" {
		return generationRendering(opt)
	}
	cfg, err := repoModel(opt)
	if err != nil {
		return nil, agentcfg.Rendered{}, err
	}
	a := opt.adapter()
	r, err := a.Render(cfg)
	return a, r, err
}

// generationRendering is the settings a generation rendered, byte for byte:
// it was rendered for this OS when it was built, so it is not rendered again.
func generationRendering(opt Options) (*claude.Adapter, agentcfg.Rendered, error) {
	path := filepath.Join(opt.Generation, claude.SettingsFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, agentcfg.Rendered{}, fmt.Errorf("sync: generation settings: %w", err)
	}
	if _, err := agentcfg.DecodeJSON(b); err != nil {
		return nil, agentcfg.Rendered{}, fmt.Errorf("sync: generation settings %s: %w", path, err)
	}
	return opt.adapter(), agentcfg.Rendered{Files: []agentcfg.File{{Path: claude.SettingsFile, Data: b, Mode: 0o600}}}, nil
}

// SettingsDrift lists the top-level keys sync would change, writing nothing.
func SettingsDrift(opt Options) ([]string, error) {
	a, r, err := render(opt)
	if err != nil {
		return nil, err
	}
	changed, err := a.Changed(opt.claudeDir(), r)
	if err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}
	return changed, nil
}

func syncSettings(opt Options, rec *backup.Recorder) (SettingsResult, error) {
	a, r, err := render(opt)
	if err != nil {
		return SettingsResult{}, err
	}
	res, err := a.ApplySettings(opt.claudeDir(), r, agentcfg.ApplyOptions{DryRun: opt.DryRun, Recorder: rec})
	if err != nil {
		return res, fmt.Errorf("sync: %w", err)
	}
	return res, nil
}

const (
	dstBegin = "<!-- aval:agent-rules -->"
	dstEnd   = "<!-- /aval:agent-rules -->"
)

// ApplyRules puts block between the aval markers of content: it replaces an
// existing block or appends one, and never touches anything outside the
// markers. The bool reports whether content changed.
func ApplyRules(block, content string) (string, bool) {
	lines := strings.Split(content, "\n")
	begin, end := -1, -1
	for i, l := range lines {
		switch strings.TrimSpace(l) {
		case dstBegin:
			if begin < 0 {
				begin = i
			}
		case dstEnd:
			if begin >= 0 && end < 0 {
				end = i
			}
		}
	}

	var out string
	if begin >= 0 && end > begin {
		merged := append([]string{}, lines[:begin+1]...)
		merged = append(merged, strings.Split(block, "\n")...)
		merged = append(merged, lines[end:]...)
		out = strings.Join(merged, "\n")
	} else {
		prefix := content
		if prefix != "" && !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
		}
		out = prefix + "\n" + dstBegin + "\n" + block + "\n" + dstEnd + "\n"
	}
	return out, out != content
}

// RemoveRules is the inverse of ApplyRules: it cuts the aval block (and the one
// blank line ApplyRules put before it) and nothing else. The bool reports
// whether content changed.
func RemoveRules(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	begin, end := -1, -1
	for i, l := range lines {
		switch strings.TrimSpace(l) {
		case dstBegin:
			if begin < 0 {
				begin = i
			}
		case dstEnd:
			if begin >= 0 && end < 0 {
				end = i
			}
		}
	}
	if begin < 0 || end < begin {
		return content, false
	}
	cut := begin
	if cut > 0 && lines[cut-1] == "" {
		cut--
	}
	out := append(append([]string{}, lines[:cut]...), lines[end+1:]...)
	return strings.Join(out, "\n"), true
}

func syncRules(opt Options, rec *backup.Recorder) ([]RulesResult, error) {
	block, err := agentcfg.RulesBlock(filepath.Join(opt.Repo, "config", "claude", "CLAUDE.md"))
	if err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}
	targets := []string{
		filepath.Join(opt.Home, ".codex", "AGENTS.md"),
		filepath.Join(opt.Home, ".gemini", "GEMINI.md"),
	}
	var out []RulesResult
	for _, path := range targets {
		res := RulesResult{Path: path}
		if fi, err := os.Stat(filepath.Dir(path)); err != nil || !fi.IsDir() {
			res.Skipped = true
			out = append(out, res)
			continue
		}
		var content string
		if b, err := os.ReadFile(path); err == nil {
			content = string(b)
		} else if !os.IsNotExist(err) {
			return out, fmt.Errorf("sync: %w", err)
		}
		next, changed := ApplyRules(block, content)
		res.Changed = changed
		if changed && !opt.DryRun {
			if _, err := rec.Snapshot(path); err != nil {
				return out, fmt.Errorf("sync: backup: %w", err)
			}
			if err := backup.WriteFileAtomic(path, []byte(next), 0o644); err != nil {
				return out, fmt.Errorf("sync: %w", err)
			}
			if err := rec.Wrote(path); err != nil {
				return out, fmt.Errorf("sync: %w", err)
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// Run applies every sync step.
func Run(opt Options) (*Report, error) {
	rec := opt.Recorder
	if rec == nil && !opt.DryRun {
		st := backup.NewStore(opt.Home)
		st.Now = opt.Now
		rec = st.Begin()
		rec.SetOrigin("sync")
		defer func() { _, _, _ = rec.Commit() }()
	}
	if rec == nil { // dry run: nothing is written, nothing is recorded
		rec = backup.NewStore(opt.Home).Begin()
	}
	opt.Recorder = rec

	rep := &Report{}
	var err error
	if rep.Settings, err = syncSettings(opt, rec); err != nil {
		return rep, err
	}
	if rep.Rules, err = syncRules(opt, rec); err != nil {
		return rep, err
	}
	if rep.Links, err = syncLinks(opt); err != nil {
		return rep, err
	}
	return rep, nil
}

// LinkResult describes one private-overlay link.
type LinkResult struct {
	Source string
	Dest   string
	Status string
	Backup string
}

// PrivateLinks reads the overlay's links.manifest and resolves both sides to
// absolute paths. A missing manifest is not an error: the overlay is optional.
func PrivateLinks(privateDir, home string) ([]plan.Action, error) {
	if privateDir == "" {
		return nil, nil
	}
	f, err := os.Open(filepath.Join(privateDir, "links.manifest"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("sync: %w", err)
	}
	defer f.Close()
	entries, err := manifest.ParseLinks(f)
	if err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}
	out := make([]plan.Action, 0, len(entries))
	for _, e := range entries {
		out = append(out, plan.Action{
			Source: filepath.Join(privateDir, filepath.FromSlash(e.Source)),
			Dest:   filepath.Join(home, filepath.FromSlash(e.Dest)),
			Label:  "~/" + e.Dest,
		})
	}
	return out, nil
}

func syncLinks(opt Options) ([]LinkResult, error) {
	actions, err := PrivateLinks(opt.PrivateDir, opt.Home)
	if err != nil || len(actions) == 0 {
		return nil, err
	}
	runner := install.New(install.Options{
		Home:     opt.Home,
		Symlinks: true,
		DryRun:   opt.DryRun,
		Recorder: opt.Recorder,
	})
	var out []LinkResult
	for _, a := range actions {
		res, err := runner.Apply(a)
		if err != nil {
			return out, fmt.Errorf("sync: %w", err)
		}
		status := "linked"
		switch res.Status {
		case install.StatusAlready:
			status = "already linked"
		case install.StatusWould:
			status = "would link"
		}
		out = append(out, LinkResult{Source: a.Source, Dest: a.Dest, Status: status, Backup: res.Backup})
	}
	return out, nil
}

// PrivateDir is where the private overlay lives: $AVAL_PRIVATE_DIR, else
// ~/.config/aval/private. It is deliberately outside the repo.
func PrivateDir(home string) string {
	if d := os.Getenv("AVAL_PRIVATE_DIR"); d != "" {
		return d
	}
	return filepath.Join(home, ".config", "aval", "private")
}

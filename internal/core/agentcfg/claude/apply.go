package claude

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/backup"
)

// SettingsResult describes what Apply did to settings.json.
type SettingsResult struct {
	// Changed lists the top-level keys that differ (or would, on a dry run).
	Changed []string
	Created bool
	Written bool
	Backup  string
	// Warnings are overlay entries the merge refused to apply.
	Warnings []string
}

type settingsPlan struct {
	merged   map[string]any
	changed  []string
	exists   bool
	warnings []string
}

// plan merges the rendering into the live file and the overlay, writing
// nothing.
func (a *Adapter) plan(target string, r agentcfg.Rendered) (settingsPlan, error) {
	var p settingsPlan
	var data []byte
	for _, f := range r.Files {
		if f.Path == SettingsFile {
			data = f.Data
		}
	}
	if data == nil {
		return p, ErrNoSettings
	}
	managed, err := agentcfg.DecodeJSON(data)
	if err != nil {
		return p, fmt.Errorf("claude: %w", err)
	}
	live, exists, err := a.Live(target)
	if err != nil {
		return p, err
	}
	overlay, err := a.Overlay()
	if err != nil {
		return p, err
	}
	p.exists = exists
	p.merged, p.changed, p.warnings = MergeSettingsWarn(managed, live, overlay)
	return p, nil
}

// Changed lists the top-level keys an Apply of r would change, writing nothing.
func (a *Adapter) Changed(target string, r agentcfg.Rendered) ([]string, error) {
	p, err := a.plan(target, r)
	return p.changed, err
}

// ApplySettings is Apply with the settings-level result. The live file is
// written atomically, mode 0600, after opt.Recorder saved the previous one
// (under the name older backups used, claude-settings.json).
func (a *Adapter) ApplySettings(target string, r agentcfg.Rendered, opt agentcfg.ApplyOptions) (SettingsResult, error) {
	var res SettingsResult
	p, err := a.plan(target, r)
	if err != nil {
		return res, err
	}
	res.Changed, res.Warnings, res.Created = p.changed, p.warnings, !p.exists
	if (len(p.changed) == 0 && p.exists) || opt.DryRun {
		return res, nil
	}
	data, err := encode(p.merged)
	if err != nil {
		return res, err
	}
	live := filepath.Join(target, SettingsFile)
	if err := a.prepare(live, p.exists, opt.Recorder, &res); err != nil {
		return res, err
	}
	if err := backup.WriteFileAtomic(live, data, 0o600); err != nil {
		return res, fmt.Errorf("write settings: %w", err)
	}
	if opt.Recorder != nil {
		if err := opt.Recorder.Wrote(live); err != nil {
			return res, err
		}
	}
	res.Written = true
	return res, nil
}

func (a *Adapter) prepare(live string, exists bool, rec *backup.Recorder, res *SettingsResult) error {
	if rec == nil {
		return os.MkdirAll(filepath.Dir(live), 0o755)
	}
	saved, err := rec.SnapshotAs(live, "claude-settings.json")
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if exists {
		res.Backup = saved
	}
	return rec.MkdirAll(filepath.Dir(live), 0o755)
}

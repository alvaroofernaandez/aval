package doctor

import (
	"path/filepath"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/generation"
	avalsync "github.com/alvaroofernaandez/aval/internal/core/sync"
)

// desired is what the live Claude Code settings must match (owner: G6 = A):
// opt.Generation, else the active generation, else the repo when none is
// active. source names it in messages.
func desired(opt Options) (avalsync.Options, string, error) {
	sopt := avalsync.Options{Repo: opt.Repo, Home: opt.Home, PrivateDir: opt.PrivateDir, GOOS: opt.GOOS, Generation: opt.Generation}
	if sopt.Generation == "" {
		dir, err := generation.ActiveDir(opt.Home, agentcfg.ClaudeCode)
		if err != nil {
			return sopt, "", err
		}
		sopt.Generation = dir
	}
	if sopt.Generation == "" {
		return sopt, "the repo", nil
	}
	return sopt, "generation " + filepath.Base(filepath.Dir(sopt.Generation)), nil
}

func checkSettingsDrift(opt Options) Result {
	sopt, source, err := desired(opt)
	if err != nil {
		return result("settings-drift", Fail, false, "cannot read the active generation: %v", err)
	}
	keys, err := avalsync.SettingsDrift(sopt)
	if err != nil {
		return result("settings-drift", Fail, false, "%v", err)
	}
	if len(keys) == 0 {
		return result("settings-drift", OK, false, "managed settings keys match %s", source)
	}
	return withDetails(result("settings-drift", Fail, true, "live settings differ in: %s", strings.Join(keys, ", ")), keys)
}

// checkHooksDrift compares the live hook commands with the desired ones.
func checkHooksDrift(opt Options, cmds []hookCmd) Result {
	sopt, source, err := desired(opt)
	if err != nil {
		return result("hooks-drift", Fail, false, "cannot read the active generation: %v", err)
	}
	repo, err := avalsync.RepoSettings(sopt)
	if err != nil {
		return result("hooks-drift", Fail, false, "cannot read %s settings: %v", source, err)
	}
	want, got := map[string]int{}, map[string]int{}
	for _, h := range hookCommands(repo) {
		want[h.event+" "+h.command]++
	}
	for _, h := range cmds {
		got[h.event+" "+h.command]++
	}
	var missing, extra int
	for k, n := range want {
		if got[k] < n {
			missing += n - got[k]
		}
	}
	for k, n := range got {
		if want[k] < n {
			extra += n - want[k]
		}
	}
	if missing == 0 && extra == 0 {
		return result("hooks-drift", OK, false, "live hooks match %s", source)
	}
	return result("hooks-drift", Fail, true, "live hooks differ from %s (%d missing, %d extra)", source, missing, extra)
}

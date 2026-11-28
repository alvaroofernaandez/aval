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

// checkGeneration re-hashes the active generation against its contentHash:
// its files are read-only, but only the hash proves them unchanged.
func checkGeneration(opt Options) Result {
	id, err := generation.Current(opt.Home)
	switch {
	case err != nil:
		return result("generation", Fail, true, "%v", err)
	case id == "":
		return result("generation", OK, false, "no generation is active; the repo is the desired state")
	}
	if err := generation.Verify(opt.Home, id); err != nil {
		return result("generation", Fail, true, "%v: it no longer matches its contentHash; aval sync activates a fresh build", err)
	}
	return result("generation", OK, false, "%s is active and matches its contentHash", id)
}

package update

import (
	"context"
	"strings"
	"testing"
)

const manifestOld = "[claude]\nlabel = c\ndetail = d\nplatforms = all\n\nconfig/claude/agents .claude/agents\nconfig/claude/old .claude/old\n"
const manifestNew = "[claude]\nlabel = c\ndetail = d\nplatforms = all\n\nconfig/claude/agents .claude/agents\nconfig/claude/new .claude/new\n"

func previewRepo() *fakeGit {
	g := cleanRepo()
	g.answers["diff --stat HEAD..@{upstream} -- install.manifest config/claude/agents"] = " config/claude/agents/a.md | 2 +-\n 1 file changed"
	g.answers["show HEAD:install.manifest"] = manifestOld
	g.answers["show @{upstream}:install.manifest"] = manifestNew
	return g
}

func TestDryRunShowsDiffStatAndManifestChanges(t *testing.T) {
	g := previewRepo()
	p, err := DryRun(context.Background(), g, []string{"install.manifest", "config/claude/agents"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Behind != 3 {
		t.Errorf("Behind = %d, want 3", p.Behind)
	}
	if !strings.Contains(p.Stat, "config/claude/agents/a.md") {
		t.Errorf("Stat = %q", p.Stat)
	}
	if len(p.Added) != 1 || !strings.Contains(p.Added[0], "config/claude/new") {
		t.Errorf("Added = %v", p.Added)
	}
	if len(p.Removed) != 1 || !strings.Contains(p.Removed[0], "config/claude/old") {
		t.Errorf("Removed = %v", p.Removed)
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	g := previewRepo()
	if _, err := DryRun(context.Background(), g, []string{"install.manifest", "config/claude/agents"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"pull", "merge", "checkout", "reset", "stash"} {
		if g.called(bad) {
			t.Errorf("dry run ran git %s", bad)
		}
	}
	if !g.called("fetch") {
		t.Error("dry run must fetch to know what is upstream")
	}
}

func TestDryRunRefusesADirtyTree(t *testing.T) {
	g := previewRepo()
	g.answers["status --porcelain"] = " M config/claude/settings.json"
	_, err := DryRun(context.Background(), g, nil)
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("err = %v, want the dirty-tree refusal", err)
	}
}

func TestDryRunWhenUpToDate(t *testing.T) {
	g := previewRepo()
	g.answers["rev-list --count HEAD..@{upstream}"] = "0"
	p, err := DryRun(context.Background(), g, []string{"install.manifest"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Behind != 0 || p.Stat != "" || len(p.Added)+len(p.Removed) != 0 {
		t.Errorf("nothing to preview, got %+v", p)
	}
}

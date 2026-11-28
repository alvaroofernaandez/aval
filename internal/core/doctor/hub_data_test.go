package doctor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
)

// jsonFixture is a machine with two dangling links and one unreadable agent
// model, so the JSON carries a fail, a fixable result and a long-ish list.
func jsonFixture(t *testing.T) Options {
	o := healthy(t)
	link(t, filepath.Join(o.Home, "gone"), filepath.Join(o.Home, ".claude/skills/dead"))
	link(t, filepath.Join(o.Home, "gone2"), filepath.Join(o.Home, ".local/bin/kiri"))
	return o
}

func TestRenderJSONIsByteIdentical(t *testing.T) {
	var b bytes.Buffer
	if err := RenderJSON(&b, Run(jsonFixture(t))); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	if strings.Contains(got, "Details") || strings.Contains(got, "Fix\"") || strings.Contains(got, "details") {
		t.Errorf("JSON leaks the new fields:\n%s", got)
	}
	if got != goldenJSON {
		t.Errorf("JSON changed.\ngot:\n%s\nwant:\n%s", got, goldenJSON)
	}
}

func TestDetailsHoldTheFullListBehindATruncatedMessage(t *testing.T) {
	o := healthy(t)
	var want []string
	for _, n := range []string{"a", "b", "c", "d", "e", "f"} {
		p := filepath.Join(o.Home, ".claude/skills", "dead-"+n)
		link(t, filepath.Join(o.Home, "gone-"+n), p)
		want = append(want, "~/.claude/skills/dead-"+n)
	}
	r := get(t, Run(o), "broken-links")
	if !strings.Contains(r.Message, "+2 more") {
		t.Fatalf("message should truncate at four: %q", r.Message)
	}
	if len(r.Details) != 6 {
		t.Fatalf("Details = %v, want all six", r.Details)
	}
	for i, d := range want {
		if r.Details[i] != d {
			t.Errorf("Details[%d] = %q, want %q", i, r.Details[i], d)
		}
	}
}

func TestFixKindsPerCheck(t *testing.T) {
	o := jsonFixture(t)
	w(t, filepath.Join(o.Home, ".claude/settings.json"), `{"hooks":{}}`)
	rs := Run(o)
	for id, want := range map[string]FixKind{
		"broken-links":   FixBrokenLinks,
		"hooks-drift":    FixSync,
		"settings-drift": FixSync,
		"links":          FixNone,
		"agents":         FixNone,
		"skills":         FixNone,
		"plugins":        FixNone,
		"hooks-resolve":  FixNone,
	} {
		if got := get(t, rs, id).Fix; got != want {
			t.Errorf("%s Fix = %v, want %v", id, got, want)
		}
	}
	// Unreadable live settings are sync-repairable on all three hook checks.
	os.Remove(filepath.Join(o.Home, ".claude/settings.json"))
	rs = Run(o)
	for _, id := range []string{"hooks-resolve", "hooks-retired", "hooks-drift"} {
		if got := get(t, rs, id).Fix; got != FixSync {
			t.Errorf("%s (no live settings) Fix = %v, want FixSync", id, got)
		}
	}
	// Invariant: Fix is none exactly when not Fixable.
	for _, r := range rs {
		if (r.Fix != FixNone) != r.Fixable {
			t.Errorf("%s: Fix=%v Fixable=%v disagree", r.ID, r.Fix, r.Fixable)
		}
	}
}

func TestEveryRunIDHasADescription(t *testing.T) {
	o := jsonFixture(t)
	o.Security = true
	o.ConfigPath = filepath.Join(o.Home, "aval.toml")
	seen := map[string]bool{}
	for _, r := range Run(o) {
		seen[r.ID] = true
	}
	for _, id := range []string{"hooks-resolve", "hooks-retired", "hooks-drift", "overlay-links", "deps"} {
		seen[id] = true
	}
	if len(seen) < 15 {
		t.Fatalf("expected the full set of ids, got %d", len(seen))
	}
	for id := range seen {
		title, why, how := Describe(id)
		if title == "" || why == "" || how == "" {
			t.Errorf("Describe(%q) = %q, %q, %q: every part must be set", id, title, why, how)
		}
	}
	if title, _, _ := Describe("no-such-check"); title != "" {
		t.Errorf("unknown id should describe nothing, got %q", title)
	}
}

func TestRemoveBrokenSnapshotsThenRemovesAndRestoreBringsThemBack(t *testing.T) {
	o := healthy(t)
	dead := filepath.Join(o.Home, ".claude/skills/dead")
	deadBin := filepath.Join(o.Home, ".local/bin/kiri")
	link(t, filepath.Join(o.Home, "gone"), dead)
	link(t, filepath.Join(o.Home, "gone2"), deadBin)

	store := backup.NewStore(o.Home)
	rec := store.Begin()
	removed, err := RemoveBroken(o, rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed = %v", removed)
	}
	for _, p := range []string{dead, deadBin} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("%s still exists", p)
		}
	}
	run, _, err := rec.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if run.Dir == "" {
		t.Fatal("no backup run was kept")
	}
	if _, err := store.Restore(run, backup.RestoreOptions{Repo: o.Repo}); err != nil {
		t.Fatal(err)
	}
	for p, target := range map[string]string{dead: filepath.Join(o.Home, "gone"), deadBin: filepath.Join(o.Home, "gone2")} {
		got, err := os.Readlink(p)
		if err != nil || got != target {
			t.Errorf("%s not restored: %q, %v", p, got, err)
		}
	}
}

func TestRemoveBrokenWithNothingBrokenLeavesNoRun(t *testing.T) {
	o := healthy(t)
	store := backup.NewStore(o.Home)
	rec := store.Begin()
	removed, err := RemoveBroken(o, rec)
	if err != nil || len(removed) != 0 {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if run, _, _ := rec.Commit(); run.Dir != "" {
		t.Errorf("an empty run was kept: %s", run.Dir)
	}
}

const goldenJSON = `[
  {
    "id": "links",
    "status": "ok",
    "message": "1 manifest links in place",
    "fixable": false
  },
  {
    "id": "broken-links",
    "status": "fail",
    "message": "2 broken: ~/.claude/skills/dead, ~/.local/bin/kiri",
    "fixable": true
  },
  {
    "id": "hooks-resolve",
    "status": "ok",
    "message": "1 hook commands resolve",
    "fixable": false
  },
  {
    "id": "hooks-retired",
    "status": "ok",
    "message": "no retired SDD hooks",
    "fixable": false
  },
  {
    "id": "hooks-drift",
    "status": "ok",
    "message": "live hooks match the repo",
    "fixable": false
  },
  {
    "id": "agents",
    "status": "ok",
    "message": "1 agents use model aliases",
    "fixable": false
  },
  {
    "id": "skills",
    "status": "ok",
    "message": "1 skills have SKILL.md with name and description",
    "fixable": false
  },
  {
    "id": "plugins",
    "status": "ok",
    "message": "1 enabled plugins are installed",
    "fixable": false
  },
  {
    "id": "settings-drift",
    "status": "ok",
    "message": "managed settings keys match the repo",
    "fixable": false
  },
  {
    "id": "generation",
    "status": "ok",
    "message": "no generation is active; the repo is the desired state",
    "fixable": false
  },
  {
    "id": "overlay",
    "status": "ok",
    "message": "absent (optional): ~/.config/aval/private",
    "fixable": false
  }
]
`

// A doctor fix names itself in the backup run, so the hub's Backups view
// can say which command made it.
func TestRemoveBrokenRecordsDoctorAsTheRunOrigin(t *testing.T) {
	o := healthy(t)
	link(t, filepath.Join(o.Home, "gone"), filepath.Join(o.Home, ".claude/skills/dead"))
	store := backup.NewStore(o.Home)
	rec := store.Begin()
	if _, err := RemoveBroken(o, rec); err != nil {
		t.Fatal(err)
	}
	if _, _, err := rec.Commit(); err != nil {
		t.Fatal(err)
	}
	runs, err := store.List()
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs = %v, %v", runs, err)
	}
	if runs[0].Origin != "doctor" {
		t.Errorf("Origin = %q, want doctor", runs[0].Origin)
	}
}

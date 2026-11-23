package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/doctor"
	avalsync "github.com/alvaroofernaandez/aval/internal/core/sync"
	"github.com/alvaroofernaandez/aval/internal/core/txsync"
)

// syncProbe counts what the view asked of the fake services.
type syncProbe struct{ previews, runs int }

func driftOutcome() *txsync.Outcome {
	return &txsync.Outcome{Report: &avalsync.Report{
		Settings: avalsync.SettingsResult{Changed: []string{"hooks", "statusLine", "env"}},
		Rules: []avalsync.RulesResult{
			{Path: "/home/u/.codex/AGENTS.md"},
			{Path: "/home/u/.gemini/GEMINI.md", Changed: true},
		},
		Links: []avalsync.LinkResult{{Dest: "/home/u/.claude/skills/private-skill", Status: "already linked"}},
	}}
}

func cleanOutcome() *txsync.Outcome {
	return &txsync.Outcome{Report: &avalsync.Report{
		Rules: []avalsync.RulesResult{{Path: "/home/u/.codex/AGENTS.md"}},
	}}
}

func syncServices(p *syncProbe, preview *txsync.Outcome) Services {
	return Services{
		Home: "/home/u",
		SyncPreview: func() (*txsync.Outcome, error) {
			p.previews++
			return preview, nil
		},
		Sync: func() (*txsync.Outcome, error) {
			p.runs++
			out := *preview
			out.RunID = "20261123-101402"
			return &out, nil
		},
	}
}

func loadedSync(t *testing.T, svc Services) screen {
	t.Helper()
	return loaded(t, newSyncView(svc))
}

func syncPhase(t *testing.T, s screen) opPhase { return opPhaseOf[*txsync.Outcome](t, s) }

func TestSyncOpeningCallsOnlyThePreview(t *testing.T) {
	p := &syncProbe{}
	s := loadedSync(t, syncServices(p, driftOutcome()))
	if p.previews != 1 || p.runs != 0 {
		t.Fatalf("after opening: previews=%d runs=%d, want 1 and 0", p.previews, p.runs)
	}
	text := viewText(s)
	for _, want := range []string{
		"! would change", "settings", "hooks, statusLine, env",
		"√ up to date", "rules", "~/.codex/AGENTS.md",
		"! would update", "~/.gemini/GEMINI.md",
		"√ linked", "overlay", "~/.claude/skills/private-skill",
		"› Apply sync", "  Back",
		"The previous files are saved to ~/.aval-backup · undo: aval restore latest",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("preview lacks %q:\n%s", want, text)
		}
	}
}

func TestSyncNoDriftOffersOnlyBack(t *testing.T) {
	p := &syncProbe{}
	s := loadedSync(t, syncServices(p, cleanOutcome()))
	text := viewText(s)
	if !strings.Contains(text, "√ nothing to sync") {
		t.Errorf("no drift:\n%s", text)
	}
	if strings.Contains(text, "Apply sync") {
		t.Errorf("no drift still offers the action:\n%s", text)
	}
	if km := s.keys(); len(km) != 0 {
		t.Errorf("no drift has keys %v, want none (esc back only)", km)
	}
	_, cmd := opStep(s, key("enter"))
	if p.runs != 0 || len(cmd) != 0 {
		t.Errorf("enter with no drift acted: runs=%d msgs=%v", p.runs, cmd)
	}
}

func TestSyncActionCallsSyncOnce(t *testing.T) {
	p := &syncProbe{}
	s := loadedSync(t, syncServices(p, driftOutcome()))
	s, _ = opStep(s, key("enter")) // the cursor is on the action
	if p.runs != 1 || p.previews != 1 {
		t.Fatalf("previews=%d runs=%d, want 1 and 1", p.previews, p.runs)
	}
	if got := syncPhase(t, s); got != phaseResult {
		t.Fatalf("phase = %v, want result", got)
	}
	text := viewText(s)
	if !strings.Contains(text, "20261123-101402") || !strings.Contains(text, "aval restore latest") {
		t.Errorf("result lacks the backup id or the undo:\n%s", text)
	}
}

func TestSyncBackOnTheInlineConfirmLeavesWithoutRunning(t *testing.T) {
	p := &syncProbe{}
	s := loadedSync(t, syncServices(p, driftOutcome()))
	s, _ = opStep(s, key("down"))
	_, msgs := opStep(s, key("enter"))
	if p.runs != 0 {
		t.Errorf("Back ran sync %d times", p.runs)
	}
	var popped bool
	for _, m := range msgs {
		_, ok := m.(popMsg)
		popped = popped || ok
	}
	if !popped {
		t.Errorf("Back did not pop the view: %v", msgs)
	}
}

func TestSyncRollbackShowsTheFailedChecksAndTheBackupID(t *testing.T) {
	p := &syncProbe{}
	svc := syncServices(p, driftOutcome())
	svc.Sync = func() (*txsync.Outcome, error) {
		p.runs++
		return &txsync.Outcome{
			RolledBack: true,
			RunID:      "20261123-101402",
			Failed: []doctor.Result{
				{ID: "settings", Status: doctor.Fail, Message: "~/.claude/settings.json has no hooks"},
				{ID: "links", Status: doctor.Fail, Message: "2 links are broken"},
			},
		}, errors.New("sync rolled back: doctor checks failed after sync")
	}
	s := loadedSync(t, svc)
	s, _ = opStep(s, key("enter"))
	text := viewText(s)
	for _, want := range []string{"× rolled back", "settings", "has no hooks", "links", "2 links are broken", "20261123-101402"} {
		if !strings.Contains(text, want) {
			t.Errorf("rollback lacks %q:\n%s", want, text)
		}
	}
}

func TestSyncRunErrorWithoutRollbackIsShown(t *testing.T) {
	svc := syncServices(&syncProbe{}, driftOutcome())
	svc.Sync = func() (*txsync.Outcome, error) { return nil, errors.New("disk full") }
	s := loadedSync(t, svc)
	s, _ = opStep(s, key("enter"))
	if text := viewText(s); !strings.Contains(text, "disk full") {
		t.Errorf("result hides the error:\n%s", text)
	}
}

func TestSyncHasNoSpinner(t *testing.T) {
	s := loadedSync(t, syncServices(&syncProbe{}, driftOutcome()))
	if s.busy() != busyIdle {
		t.Errorf("busy before the run = %v, want idle", s.busy())
	}
	o := s.(glyphSepScreen).screen.(operation[*txsync.Outcome])
	if o.spinning() {
		t.Error("sync spins; it is local")
	}
}

func init() {
	type golden = struct {
		name  string
		build func(t *testing.T, unicode bool) tea.Model
	}
	goldenViews = append(goldenViews,
		golden{"sync", func(t *testing.T, unicode bool) tea.Model {
			return goldenOpViewApp(t, unicode, newSyncView(syncServices(&syncProbe{}, driftOutcome())), false)
		}},
		golden{"sync_clean", func(t *testing.T, unicode bool) tea.Model {
			return goldenOpViewApp(t, unicode, newSyncView(syncServices(&syncProbe{}, cleanOutcome())), false)
		}},
		golden{"sync_rollback", func(t *testing.T, unicode bool) tea.Model {
			svc := syncServices(&syncProbe{}, driftOutcome())
			svc.Sync = func() (*txsync.Outcome, error) {
				return &txsync.Outcome{
					RolledBack: true,
					RunID:      "20261123-101402",
					Failed:     []doctor.Result{{ID: "settings", Status: doctor.Fail, Message: "~/.claude/settings.json has no hooks"}},
				}, errors.New("sync rolled back: doctor checks failed after sync")
			}
			return goldenOpViewApp(t, unicode, newSyncView(svc), true)
		}},
	)
}

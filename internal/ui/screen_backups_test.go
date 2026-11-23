package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
)

func init() {
	goldenViews = append(goldenViews,
		struct {
			name  string
			build func(t *testing.T, unicode bool) tea.Model
		}{"backups", func(t *testing.T, unicode bool) tea.Model {
			a, _ := backupsApp(t, unicode, newBackupsFake())
			return a
		}},
		struct {
			name  string
			build func(t *testing.T, unicode bool) tea.Model
		}{"backups_preview", func(t *testing.T, unicode bool) tea.Model {
			a, _ := backupsApp(t, unicode, newBackupsFake())
			a, _ = routerStep(a, key("enter"))
			return a
		}},
	)
}

const backupsHome = "/home/u"

// backupsFake is the backup store as the view sees it, with every call
// recorded.
type backupsFake struct {
	repo     string
	runs     []backup.Run
	previews int
	restores int
	pins     []string
	// restoreErr fails the restore after its first action.
	restoreErr error
}

func utcAt(y, mo, d, h, mi, s int) time.Time {
	return time.Date(y, time.Month(mo), d, h, mi, s, 0, time.UTC)
}

func newBackupsFake() *backupsFake {
	dir := backupsHome + "/.aval-backup/"
	return &backupsFake{
		repo: backupsHome + "/.aval",
		// Out of order on purpose: the view lists newest first whatever the
		// service returns.
		runs: []backup.Run{
			{ID: "20261120-184000", Dir: dir + "20261120-184000", Time: utcAt(2026, 11, 20, 18, 40, 0), Entries: 14, Origin: "uninstall"},
			{ID: "20261123-101402", Dir: dir + "20261123-101402", Time: utcAt(2026, 11, 23, 10, 14, 2), Entries: 3, Origin: "sync"},
			{ID: "20261119-081100", Dir: dir + "20261119-081100", Time: utcAt(2026, 11, 19, 8, 11, 0), Entries: 2, Origin: "sync", Failed: true, Reason: "doctor failed"},
			{ID: "20261123-090200", Dir: dir + "20261123-090200", Time: utcAt(2026, 11, 23, 9, 2, 0), Entries: 12, Origin: "install", Pinned: true},
		},
	}
}

func (f *backupsFake) services() Services {
	return Services{
		tr:   testTr,
		Home: backupsHome,
		Repo: f.repo,
		Backups: func() ([]backup.Run, error) {
			return append([]backup.Run(nil), f.runs...), nil
		},
		RunEntries: func(run backup.Run) ([]backup.Entry, error) {
			return []backup.Entry{
				{Path: backupsHome + "/.claude/settings.json", Existed: true, Kind: backup.KindFile},
				{Path: backupsHome + "/.codex/AGENTS.md", Existed: true, Kind: backup.KindFile},
				{Path: backupsHome + "/.gemini/GEMINI.md", Kind: backup.KindFile},
			}, nil
		},
		RestorePreview: func(run backup.Run) ([]backup.Action, error) {
			f.previews++
			return []backup.Action{
				{Path: backupsHome + "/.claude/settings.json", Action: "would restore", Kind: backup.ActRestore},
				{Path: backupsHome + "/.gemini/GEMINI.md", Action: "would remove", Kind: backup.ActRemove},
				{Path: backupsHome + "/.codex/AGENTS.md", Action: "unchanged", Kind: backup.ActUnchanged},
			}, nil
		},
		Restore: func(run backup.Run) ([]backup.Action, error) {
			f.restores++
			if f.restoreErr != nil {
				return []backup.Action{{Path: backupsHome + "/.claude/settings.json", Kind: backup.ActRestore}}, f.restoreErr
			}
			return []backup.Action{
				{Path: backupsHome + "/.claude/settings.json", Action: "restored", Kind: backup.ActRestore},
				{Path: backupsHome + "/.gemini/GEMINI.md", Action: "removed", Kind: backup.ActRemove},
				{Path: backupsHome + "/.codex/AGENTS.md", Action: "unchanged", Kind: backup.ActUnchanged},
			}, nil
		},
		Pin: func(id string, on bool) error {
			state := "off"
			if on {
				state = "on"
			}
			f.pins = append(f.pins, id+" "+state)
			for i := range f.runs {
				if f.runs[i].ID == id {
					f.runs[i].Pinned = on
				}
			}
			return nil
		},
	}
}

// backupsApp is the view as the program runs it: a fake hub at the bottom
// and the backups view pushed on top.
func backupsApp(t *testing.T, unicode bool, f *backupsFake) (App, *backupsFake) {
	t.Helper()
	ch := chrome{tr: testTr, theme: goldenTheme(t, unicode), version: "0.2.0", platform: "macOS (arm64)", width: 80, height: 40}
	ctor, ok := viewFor(viewBackups)
	if !ok {
		t.Fatal("no view registered under backups")
	}
	a := newApp(ch, Services{}, newFake("hub"))
	a, _ = routerStep(a, pushMsg{ctor(f.services())})
	return a, f
}

func backupsLine(out, s string) int {
	for i, l := range strings.Split(out, "\n") {
		if strings.Contains(l, s) {
			return i
		}
	}
	return -1
}

func TestBackupsListsRunsNewestFirstWithOriginAndPin(t *testing.T) {
	a, _ := backupsApp(t, true, newBackupsFake())
	out := a.View()
	order := []string{"2026-11-23 10:14", "2026-11-23 09:02", "2026-11-20 18:40", "2026-11-19 08:11"}
	prev := -1
	for _, ts := range order {
		i := backupsLine(out, ts)
		if i <= prev {
			t.Fatalf("%s is at line %d, after %d: runs are not newest first:\n%s", ts, i, prev, out)
		}
		prev = i
	}
	lines := strings.Split(out, "\n")
	for ts, want := range map[string][]string{
		"2026-11-23 10:14": {"sync", "3 entries"},
		"2026-11-23 09:02": {"install", "12 entries", "pinned"},
		"2026-11-20 18:40": {"uninstall", "14 entries"},
		"2026-11-19 08:11": {"sync", "2 entries", "× failed, rolled back"},
	} {
		line := lines[backupsLine(out, ts)]
		for _, w := range want {
			if !strings.Contains(line, w) {
				t.Errorf("row %q lacks %q", line, w)
			}
		}
	}
	if strings.Contains(lines[backupsLine(out, "2026-11-23 10:14")], "pinned") {
		t.Errorf("an unpinned run reads pinned:\n%s", out)
	}
	if !strings.Contains(out, "› 2026-11-23 10:14") {
		t.Errorf("the cursor does not start on the newest run:\n%s", out)
	}
	if !strings.Contains(out, "20261123-101402 holds") || !strings.Contains(out, "~/.claude/settings.json") || !strings.Contains(out, "created by aval") {
		t.Errorf("the lower pane does not list what the cursor's run holds:\n%s", out)
	}
	if !strings.Contains(out, "~/.aval-backup · 4 runs") {
		t.Errorf("the heading does not name the store and the count:\n%s", out)
	}
	if want := "The newest 5 unpinned runs are kept"; !strings.Contains(out, want) || backup.Retain != 5 {
		t.Errorf("status line wants %q from backup.Retain (%d):\n%s", want, backup.Retain, out)
	}
}

func TestBackupsPTogglesThePin(t *testing.T) {
	a, f := backupsApp(t, true, newBackupsFake())
	a, _ = routerStep(a, key("p"))
	a, _ = routerStep(a, key("down"))
	a, _ = routerStep(a, key("p"))
	want := []string{"20261123-101402 on", "20261123-090200 off"}
	if strings.Join(f.pins, ",") != strings.Join(want, ",") {
		t.Fatalf("Pin calls = %v, want %v", f.pins, want)
	}
	out := a.View()
	if line := strings.Split(out, "\n")[backupsLine(out, "2026-11-23 10:14")]; !strings.Contains(line, "pinned") {
		t.Errorf("the pinned run does not read pinned after the reload: %q", line)
	}
	if line := strings.Split(out, "\n")[backupsLine(out, "2026-11-23 09:02")]; strings.Contains(line, "pinned") {
		t.Errorf("the unpinned run still reads pinned: %q", line)
	}
	if !strings.Contains(out, "› 2026-11-23 09:02") {
		t.Errorf("the cursor left the run it pinned:\n%s", out)
	}
}

func TestBackupsLegacyAndPendingRunsOfferNoPreview(t *testing.T) {
	f := newBackupsFake()
	dir := backupsHome + "/.aval-backup/"
	f.runs = []backup.Run{
		{ID: "20261123-101402", Dir: dir + "20261123-101402", Time: utcAt(2026, 11, 23, 10, 14, 2), Entries: 3, Origin: "sync", Pending: true},
		{ID: "20240101120000", Dir: dir + "20240101120000", Time: utcAt(2024, 1, 1, 12, 0, 0), Legacy: true},
	}
	a, _ := backupsApp(t, true, f)
	for i, want := range []string{"interrupted, the next sync restores it", "restore by hand from ~/.aval-backup/20240101120000"} {
		if i > 0 {
			a, _ = routerStep(a, key("down"))
		}
		if out := a.View(); !strings.Contains(out, want) {
			t.Errorf("run %d does not say %q:\n%s", i, want, out)
		}
		if _, ok := a.keys().lookup("enter"); ok {
			t.Errorf("run %d offers enter", i)
		}
		if strings.Contains(a.View(), "enter preview restore") {
			t.Errorf("run %d hints a preview", i)
		}
		a, _ = routerStep(a, key("enter"))
		if a.depth() != 2 || f.previews != 0 {
			t.Errorf("run %d: enter pushed (depth %d) or previewed (%d)", i, a.depth(), f.previews)
		}
	}
}

func TestBackupsPreviewOnlyPreviews(t *testing.T) {
	a, f := backupsApp(t, true, newBackupsFake())
	a, _ = routerStep(a, key("enter"))
	if f.previews != 1 || f.restores != 0 {
		t.Fatalf("enter: %d previews, %d restores; want 1 and 0", f.previews, f.restores)
	}
	out := a.View()
	for _, want := range []string{"! would restore", "! would remove", "√ unchanged", "~/.claude/settings.json", "1 to restore · 1 to remove · 1 unchanged", "Nothing has changed yet"} {
		if !strings.Contains(out, want) {
			t.Errorf("the preview lacks %q:\n%s", want, out)
		}
	}
}

func TestBackupsRestoreConfirmOpensOnCancelAndNamesDisplaced(t *testing.T) {
	a, f := backupsApp(t, true, newBackupsFake())
	a, _ = routerStep(a, key("enter"))
	a, _ = routerStep(a, key("enter"))
	out := a.View()
	for _, want := range []string{"› Cancel", "  Restore 2 paths"} {
		if !strings.Contains(out, want) {
			t.Errorf("the confirm lacks %q:\n%s", want, out)
		}
	}
	// The full line is wider than the 76-column cap, so it wraps in the
	// prompt; the status line keeps a short form that still names .displaced.
	safety := "Anything aval did not write moves to the run's .displaced folder, not deleted"
	if flat := strings.Join(strings.Fields(out), " "); !strings.Contains(flat, safety) {
		t.Errorf("the confirm does not name the safety net %q:\n%s", safety, out)
	}
	if status := strings.Split(out, "\n"); !strings.Contains(status[len(status)-2], ".displaced") {
		t.Errorf("the status line does not name .displaced: %q", status[len(status)-2])
	}
	a, _ = routerStep(a, key("enter"))
	if f.restores != 0 {
		t.Errorf("enter on Cancel restored %d times", f.restores)
	}
}

func TestBackupsConfirmRunsRestoreOnce(t *testing.T) {
	a, f := backupsApp(t, true, newBackupsFake())
	for _, k := range []string{"enter", "enter", "up", "enter"} {
		a, _ = routerStep(a, key(k))
	}
	if f.restores != 1 {
		t.Fatalf("Restore ran %d times, want 1", f.restores)
	}
	out := a.View()
	for _, want := range []string{"Restored backup 20261123-101402", "1 restored", "1 removed", "1 unchanged"} {
		if !strings.Contains(out, want) {
			t.Errorf("the result lacks %q:\n%s", want, out)
		}
	}
}

func TestBackupsRestoreErrorShowsThePathAndTheCommand(t *testing.T) {
	f := newBackupsFake()
	f.restoreErr = errors.New("permission denied")
	a, _ := backupsApp(t, true, f)
	for _, k := range []string{"enter", "enter", "up", "enter"} {
		a, _ = routerStep(a, key(k))
	}
	out := a.View()
	for _, want := range []string{"× could not restore ~/.claude/settings.json", "permission denied", "aval restore 20261123-101402"} {
		if !strings.Contains(out, want) {
			t.Errorf("the error lacks %q:\n%s", want, out)
		}
	}
}

func TestBackupsWithoutARepositoryListButExplainRestore(t *testing.T) {
	f := newBackupsFake()
	f.repo = ""
	a, _ := backupsApp(t, true, f)
	if out := a.View(); !strings.Contains(out, "2026-11-23 10:14") {
		t.Fatalf("the list does not work without a repository:\n%s", out)
	}
	a, _ = routerStep(a, key("enter"))
	out := a.View()
	if !strings.Contains(out, "Restoring needs the aval repository") || !strings.Contains(out, "aval install") {
		t.Errorf("the preview does not explain that restoring needs a repository:\n%s", out)
	}
	if f.previews != 0 || f.restores != 0 {
		t.Errorf("without a repository: %d previews, %d restores; want none", f.previews, f.restores)
	}
}

package ui

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"

	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/design/gate"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// Hub v2 (DESIGN.md section 10, Hub icons): one-cell icons, badges that carry
// data, a detail pane that follows the cursor and a letter per entry.

func lines(s string) []string { return strings.Split(s, "\n") }

// menuRow is the rendered line holding the menu entry with this icon and
// name, cut at the pane divider so only the list part is returned.
func menuRow(t *testing.T, out, icon, name string) string {
	t.Helper()
	for _, l := range lines(out) {
		if i := strings.Index(l, icon+" "+name); i >= 0 {
			for _, div := range []string{"│", "|"} {
				if j := strings.Index(l, div); j > i {
					l = l[:j]
				}
			}
			return strings.TrimRight(l, " ")
		}
	}
	t.Fatalf("no menu row for %s %s:\n%s", icon, name, out)
	return ""
}

// pane is the detail pane: what follows the divider on every row that has
// one, trimmed on the right.
func pane(out string) []string {
	var p []string
	for _, l := range lines(out) {
		if _, rest, ok := strings.Cut(l, "│ "); ok {
			p = append(p, strings.TrimRight(rest, " "))
		} else if strings.HasSuffix(strings.TrimRight(l, " "), "│") {
			p = append(p, "")
		}
	}
	return p
}

func TestHubIconsPassTheGateAndAreOneCell(t *testing.T) {
	dir := t.TempDir()
	for _, e := range hubMenu {
		if e.icon == "" {
			t.Errorf("%s has no icon", e.name)
			continue
		}
		if w := lipgloss.Width(e.icon); w != 1 {
			t.Errorf("%s icon %q is %d cells wide, want 1", e.name, e.icon, w)
		}
		cjk := runewidth.NewCondition()
		cjk.EastAsianWidth = true
		for _, r := range e.icon {
			if runewidth.IsAmbiguousWidth(r) || cjk.RuneWidth(r) != 1 {
				t.Errorf("%s icon %q (U+%04X) has an ambiguous East Asian width", e.name, e.icon, r)
			}
		}
		f := filepath.Join(dir, e.view+".md")
		if err := os.WriteFile(f, []byte(e.icon+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := gate.Emoji(io.Discard, []string{f}); code != 0 {
			t.Errorf("%s icon %q fails aval design gate emoji", e.name, e.icon)
		}
	}
}

func TestHubASCIIIconsArePureASCII(t *testing.T) {
	seen := map[string]string{}
	for _, e := range hubMenu {
		if len(e.ascii) != 1 || e.ascii[0] < 0x21 || e.ascii[0] > 0x7e {
			t.Errorf("%s ASCII icon %q is not one printable ASCII character", e.name, e.ascii)
		}
		if other, dup := seen[e.ascii]; dup {
			t.Errorf("%s and %s share the ASCII icon %q", e.name, other, e.ascii)
		}
		seen[e.ascii] = e.name
	}
}

func TestHubDetailPaneFollowsTheCursor(t *testing.T) {
	withHubViews(t)
	a := startedHub(t, true, hubServices())
	p := pane(renderModelAt(a, 80, 24))
	if len(p) < 2 || !strings.HasPrefix(p[0], "⇣ Install") || !strings.HasSuffix(p[0], " i") {
		t.Fatalf("the pane does not open on Install with its letter:\n%s", strings.Join(p, "\n"))
	}
	if p[1] != "Link the repo configs into this home." {
		t.Errorf("Install pane description = %q", p[1])
	}
	for range 3 {
		a, _ = routerStep(a, key("down"))
	}
	p = pane(renderModelAt(a, 80, 24))
	if len(p) < 2 || !strings.HasPrefix(p[0], "⊞ Doctor") || !strings.HasSuffix(p[0], " d") {
		t.Fatalf("the pane did not follow the cursor to Doctor:\n%s", strings.Join(p, "\n"))
	}
	if p[1] != "Check this machine against the repo." {
		t.Errorf("Doctor pane description = %q", p[1])
	}
}

func TestHubLetterOpensItsView(t *testing.T) {
	want := map[string]string{"i": viewInstall, "u": viewUpdate, "s": viewSync, "d": viewDoctor, "b": viewBackups, "x": viewUninstall, ",": viewSettings, "v": viewDev, "a": viewAbout}
	for _, e := range hubMenu {
		if want[e.key] != e.view {
			t.Errorf("%s has key %q, want the one that maps to it", e.name, e.key)
		}
	}
	for k, v := range want {
		withHubViews(t)
		a := startedHub(t, false, hubServices())
		a, _ = routerStep(a, key(k))
		if a.depth() != 2 {
			t.Errorf("%q did not open a view (depth %d)", k, a.depth())
			continue
		}
		if f, ok := a.top().(fakeScreen); !ok || f.name != v {
			t.Errorf("%q opened %v, want %s", k, a.top(), v)
		}
	}
	// An unavailable entry's letter moves the cursor there and opens nothing.
	withHubViews(t)
	svc := hubServices()
	svc.Repo, svc.RepoErr = "", errNoRepo
	a := startedHub(t, false, svc)
	a, _ = routerStep(a, key("u"))
	if a.depth() != 1 || a.stack[0].(hub).cursor != 1 {
		t.Errorf("u without a repo: depth %d cursor %d, want the hub on Update", a.depth(), a.stack[0].(hub).cursor)
	}
}

func TestHubBadgesOnlyWithData(t *testing.T) {
	withHubViews(t)
	bare := hubServices()
	bare.RepoStatus, bare.Sandboxes = nil, nil
	bare.Backups = func() ([]backup.Run, error) { return nil, nil }
	a := hubApp(t, false, bare)
	pending := drainCmd(a.Init())
	out := renderModelAt(a, 80, 40)
	for _, r := range []struct{ icon, name string }{{"@", "Update"}, {"+", "Doctor"}, {"o", "Backups"}, {"#", "Dev sandbox"}} {
		if row := menuRow(t, out, r.icon, r.name); !strings.HasSuffix(row, r.name) {
			t.Errorf("no data, yet %s has a badge: %q", r.name, row)
		}
	}
	for _, m := range pending {
		a, _ = routerStep(a, m)
	}
	if row := menuRow(t, renderModelAt(a, 80, 40), "+", "Doctor"); !strings.HasSuffix(row, "Doctor     ! 1") {
		t.Errorf("doctor badge after the run: %q, want the worst state and its count", row)
	}

	for _, rs := range []struct {
		behind      int
		known, show bool
	}{{3, false, false}, {0, true, false}, {3, true, true}} {
		svc := hubServices()
		svc.RepoStatus = func() (int, bool, bool) { return rs.behind, rs.known, false }
		row := menuRow(t, renderModelAt(startedHub(t, false, svc), 80, 40), "@", "Update")
		if got := strings.HasSuffix(row, "^ 3"); got != rs.show {
			t.Errorf("behind %d known %v: update row %q, badge shown %v want %v", rs.behind, rs.known, row, got, rs.show)
		}
	}
	full := renderModelAt(startedHub(t, false, hubServices()), 80, 40)
	if row := menuRow(t, full, "o", "Backups"); !strings.HasSuffix(row, "Backups      4") {
		t.Errorf("backups badge: %q, want the run count", row)
	}
	if row := menuRow(t, full, "#", "Dev sandbox"); !strings.HasSuffix(row, "Dev sandbox  2") {
		t.Errorf("dev badge: %q, want the sandbox count", row)
	}
}

var sgrRe = regexp.MustCompile("\x1b\\[([0-9;]*)m")

func TestHubSelectedRowHasNoReverseVideo(t *testing.T) {
	withHubViews(t)
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	th, err := NewTheme(DefaultThemeName, caps.Caps{Profile: termenv.TrueColor, Unicode: true, Dark: true}, r)
	if err != nil {
		t.Fatal(err)
	}
	svc := hubServices()
	c := chrome{theme: th, version: svc.Build.Version, platform: "macOS (arm64)", width: 80, height: 24}
	a := newApp(c, svc, newHubAt(svc, func() time.Time { return hubNow }))
	for _, m := range drainCmd(a.Init()) {
		a, _ = routerStep(a, m)
	}
	out := renderModelAt(a, 80, 24)
	var row string
	for _, l := range lines(out) {
		if strings.Contains(l, "Install") && strings.Contains(l, "Set up") {
			row = l
		}
	}
	if row == "" {
		t.Fatalf("no selected Install row:\n%s", out)
	}
	bold := false
	for _, m := range sgrRe.FindAllStringSubmatch(row, -1) {
		for _, p := range strings.Split(m[1], ";") {
			switch p {
			case "7":
				t.Errorf("the selected row uses reverse video: %q", row)
			case "1":
				bold = true
			}
		}
	}
	if !bold {
		t.Errorf("the selected row is not bold: %q", row)
	}
}

func TestShortenPathKeepsTheTail(t *testing.T) {
	c := chrome{theme: goldenTheme(t, true)}
	for _, tc := range []struct {
		home, path string
		w          int
		want       string
	}{
		{"/home/u", "/home/u/.aval", 25, "~/.aval"},
		{"/home/u", "/Users/someone/.aval-dev/sandboxes/dev/default/repo", 20, "…/dev/default/repo"},
		{"/home/u", "/home/u/projects/clients/acme/dev/default/repo", 22, "~/…/dev/default/repo"},
		{"/home/u", "/opt/a/averyveryverylongsegmentname", 12, "…segmentname"},
	} {
		got := c.shortenPath(tc.home, tc.path, tc.w)
		if got != tc.want {
			t.Errorf("shortenPath(%q, %d) = %q, want %q", tc.path, tc.w, got, tc.want)
		}
		if lipgloss.Width(got) > tc.w {
			t.Errorf("shortenPath(%q, %d) is %d wide", tc.path, tc.w, lipgloss.Width(got))
		}
	}
}

func TestHubBackupRowIsRelative(t *testing.T) {
	for _, tc := range []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 11, 23, 12, 54, 0, 0, time.UTC), "today 12:54"},
		{time.Date(2026, 11, 23, 0, 5, 0, 0, time.UTC), "today 00:05"},
		{time.Date(2026, 11, 22, 18, 40, 0, 0, time.UTC), "yesterday 18:40"},
		{time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC), "2026-11-20 09:00"},
		{time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC), "2026-11-24 09:00"},
	} {
		if got := relDay(hubNow, tc.at); got != tc.want {
			t.Errorf("relDay(%v) = %q, want %q", tc.at, got, tc.want)
		}
	}
	withHubViews(t)
	out := renderModelAt(startedHub(t, true, hubServices()), 80, 24)
	if !strings.Contains(out, "Backup   today 12:54 · install") {
		t.Errorf("the panel backup row is not relative with its origin:\n%s", out)
	}
}

func TestHubTargetStructureAt80x24(t *testing.T) {
	withHubViews(t)
	out := renderModelAt(startedHub(t, true, hubServices()), 80, 24)
	ls := lines(out)
	if len(ls) > 24 {
		t.Fatalf("80x24 is %d lines", len(ls))
	}
	if !strings.Contains(ls[0], "██  Version  0.3.0") {
		t.Errorf("the panel does not start beside the wordmark's first row: %q", ls[0])
	}
	for _, want := range []string{
		"██  Repo     ~/.aval",
		"██  Doctor   ! 1 warn",
		"██  Backup   today 12:54 · install",
		"██  Theme    mono",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("panel: missing %q", want)
		}
	}
	rows := []string{
		"  Set up  › ⇣ Install        │ ⇣ Install",
		"            ⟳ Update     ↑ 3 │ Link the repo configs into this home.",
		"            ⇄ Sync           │",
		"  Health    ⊞ Doctor     ! 1 │ Repo      ~/.aval",
		"  Undo      ◷ Backups      4 │ Last run  today 12:54",
		"            ⌫ Uninstall      │",
		"  Tools     ⎈ Settings       │ enter  pick what to link",
		"            ⧉ Dev sandbox  2 │",
		"            ◉ About          │",
	}
	start := -1
	for i, l := range ls {
		if strings.HasPrefix(l, rows[0]) {
			start = i
		}
	}
	if start < 0 {
		t.Fatalf("no Install row %q:\n%s", rows[0], out)
	}
	if !strings.HasSuffix(ls[start], " i") || lipgloss.Width(ls[start]) != 78 {
		t.Errorf("the pane header does not end with its letter at the content edge: %q", ls[start])
	}
	for i, want := range rows {
		if got := strings.TrimRight(ls[start+i], " "); !strings.HasPrefix(got+" ", want+" ") && got != want {
			t.Errorf("menu row %d = %q, want %q", i, got, want)
		}
	}
	for _, want := range []string{
		"  1 warning · open Doctor to see it",
		"  ↑↓ move · enter open · i u s d b x , v a jump · ? help · q quit",
	} {
		found := false
		for _, l := range ls {
			found = found || l == want
		}
		if !found {
			t.Errorf("missing line %q:\n%s", want, out)
		}
	}
}

func TestHubPaneCollapsesUnder80(t *testing.T) {
	withHubViews(t)
	a := startedHub(t, true, hubServices())
	for _, w := range []int{79, 60, 45} {
		out := renderModelAt(a, w, 40)
		if strings.Contains(out, "│") {
			t.Errorf("%d columns still draws the side pane:\n%s", w, out)
		}
		ls := lines(out)
		about := -1
		for i, l := range ls {
			if strings.Contains(l, "◉ About") {
				about = i
			}
		}
		if about < 0 || about+1 >= len(ls) || strings.TrimSpace(ls[about+1]) != "Link the repo configs into this home." {
			t.Errorf("%d columns: the line under the list is not the description:\n%s", w, out)
		}
	}
	for range 3 {
		a, _ = routerStep(a, key("down"))
	}
	out := renderModelAt(a, 79, 40)
	if !strings.Contains(out, "! plugins: 1 missing") {
		t.Errorf("79 columns on Doctor: the collapsed pane does not show the urgent state:\n%s", out)
	}
	if strings.Contains(out, "Check this machine") {
		t.Errorf("79 columns on Doctor: the collapsed pane is more than one line:\n%s", out)
	}
	if compact := renderModelAt(a, 36, 40); strings.Contains(compact, "plugins: 1 missing") || strings.Contains(compact, "Check this machine") {
		t.Errorf("compact shows the pane:\n%s", compact)
	}
}

func TestHubPaneStateBlockPerEntry(t *testing.T) {
	want := map[string][]string{
		viewInstall:   {"Repo      ~/.aval", "Last run  today 12:54", "enter  pick what to link"},
		viewUpdate:    {"Upstream  ↑ 3 commits behind", "aval      ! 0.3.1 available"},
		viewSync:      {"Repo      ~/.aval", "Last run  yesterday 18:40"},
		viewDoctor:    {"Last run  today 13:30", "          √ 10 ok  ! 1 warn  × 0 fail", "Needs you ! plugins: 1 missing", "enter  open the full report", "f      fix what is safe to fix here"},
		viewBackups:   {"Newest    today 12:54 · install", "          yesterday 18:40 · sync", "          2026-11-20 09:00 · sync"},
		viewUninstall: {"Remove aval's links, put back what was there.", "Safety    backs up first · aval restore latest"},
		viewSettings:  {"Theme     mono", "Glyphs    unicode", "Motion    off"},
		viewDev:       {"Sandboxes default · scratch"},
		viewAbout:     {"Version   0.3.0", "Channel   stable"},
	}
	for i, e := range hubMenu {
		withHubViews(t)
		a := startedHub(t, true, hubServices())
		for range i {
			a, _ = routerStep(a, key("down"))
		}
		p := pane(renderModelAt(a, 80, 24))
		if len(p) > len(hubMenu) {
			t.Errorf("%s pane is %d rows, taller than the menu", e.name, len(p))
		}
		joined := strings.Join(p, "\n")
		for _, w := range want[e.view] {
			if !strings.Contains(joined, w) {
				t.Errorf("%s pane: missing %q:\n%s", e.name, w, joined)
			}
		}
	}
}

func TestHubHelpListsEveryShortcut(t *testing.T) {
	withHubViews(t)
	a := startedHub(t, true, hubServices())
	a, _ = routerStep(a, key("?"))
	out := renderModelAt(a, 80, 40)
	for _, e := range hubMenu {
		if !strings.Contains(out, "open "+e.name) {
			t.Errorf("help does not list %q for %s:\n%s", e.key, e.name, out)
		}
	}
}

func TestHubInvariantsAcrossSizes(t *testing.T) {
	withHubViews(t)
	for _, unicode := range []bool{true, false} {
		a := startedHub(t, unicode, hubServices())
		for w := 20; w <= 120; w++ {
			for h := 8; h <= 40; h++ {
				ls := lines(renderModelAt(a, w, h))
				if len(ls) > h {
					t.Errorf("unicode=%v %dx%d: %d lines tall", unicode, w, h, len(ls))
				}
				for i, l := range ls {
					if lw := lipgloss.Width(l); lw > w {
						t.Errorf("unicode=%v %dx%d: line %d is %d wide: %q", unicode, w, h, i, lw, l)
						break
					}
				}
			}
		}
	}
}

func TestHubGutterKeepsASpaceBeforeTheCursor(t *testing.T) {
	withHubViews(t)
	for _, unicode := range []bool{true, false} {
		for _, w := range []int{80, 120} {
			ls := lines(renderModelAt(startedHub(t, unicode, hubServices()), w, 24))
			for _, group := range []string{"Set up", "Health", "Undo", "Tools"} {
				found := false
				for _, l := range ls {
					rest, ok := strings.CutPrefix(l, "  "+group)
					if !ok {
						continue
					}
					found = true
					if !strings.HasPrefix(rest, " ") {
						t.Errorf("unicode=%v width=%d: no space after group %q: %q", unicode, w, group, l)
					}
				}
				if !found {
					t.Errorf("unicode=%v width=%d: no row for group %q", unicode, w, group)
				}
			}
		}
	}
}

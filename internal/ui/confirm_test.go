package ui

import (
	"strings"
	"testing"
)

const testSafety = "Files in the way move to ~/.aval-backup first · undo: aval restore latest"

func testConfirm(t *testing.T, mode confirmMode) confirm {
	t.Helper()
	c, err := newConfirm(mode, confirmSpec{Action: "Remove 2 links", Safety: testSafety})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func testChrome(t *testing.T, unicode bool) chrome {
	t.Helper()
	return chrome{tr: testTr, theme: goldenTheme(t, unicode), version: "0.2.0", platform: "macOS (arm64)", width: 80, height: 24}
}

func TestDestructiveConfirmOpensOnCancel(t *testing.T) {
	c := testConfirm(t, confirmDestructive)
	if !c.onCancel() {
		t.Fatalf("destructive confirm opens with cursor %d, want Cancel", c.cursor)
	}
	ch := testChrome(t, true)
	got := strings.Join(c.lines(ch, ch.layout()), "\n")
	if !strings.Contains(got, "› Cancel") || !strings.Contains(got, "  Remove 2 links") {
		t.Errorf("rows do not put the cursor on Cancel:\n%s", got)
	}
}

func TestInlineConfirmOpensOnTheAction(t *testing.T) {
	c := testConfirm(t, confirmInline)
	if c.onCancel() {
		t.Fatal("inline confirm opens on the cancel row, want the action")
	}
	ch := testChrome(t, true)
	got := strings.Join(c.lines(ch, ch.layout()), "\n")
	if !strings.Contains(got, "› Remove 2 links") || !strings.Contains(got, "  Back") {
		t.Errorf("inline rows want the cursor on the action and a Back row:\n%s", got)
	}
}

func TestConfirmRequiresASafetyLine(t *testing.T) {
	for _, safety := range []string{"", "   "} {
		if _, err := newConfirm(confirmDestructive, confirmSpec{Action: "Remove", Safety: safety}); err == nil {
			t.Errorf("newConfirm with safety %q = nil error, want one", safety)
		}
	}
	if _, err := newConfirm(confirmDestructive, confirmSpec{Safety: testSafety}); err == nil {
		t.Error("newConfirm without an action = nil error, want one")
	}
}

func TestConfirmNamesTheSafetyNetInTheStatus(t *testing.T) {
	if got := testConfirm(t, confirmDestructive).status(); got != testSafety {
		t.Errorf("status = %q, want the safety line", got)
	}
}

func TestConfirmArrowsMoveAndEnterChooses(t *testing.T) {
	c := testConfirm(t, confirmDestructive)
	c, ch := c.update(key("up"))
	if ch != choiceNone || c.onCancel() {
		t.Fatalf("up: choice %v, onCancel %v; want the action row", ch, c.onCancel())
	}
	if _, ch = c.update(key("enter")); ch != choiceAction {
		t.Errorf("enter on the action = %v, want choiceAction", ch)
	}
	c, _ = c.update(key("down"))
	if _, ch = c.update(key("enter")); ch != choiceCancel {
		t.Errorf("enter on Cancel = %v, want choiceCancel", ch)
	}
}

func TestDestructiveConfirmEscIsCancel(t *testing.T) {
	c := testConfirm(t, confirmDestructive)
	c, _ = c.update(key("up"))
	if _, ch := c.update(escKey()); ch != choiceCancel {
		t.Errorf("esc = %v, want choiceCancel", ch)
	}
}

// The confirm keys never include a y key: the choice is the rows.
func TestConfirmHasNoYesKey(t *testing.T) {
	for _, mode := range []confirmMode{confirmDestructive, confirmInline} {
		if _, ok := testConfirm(t, mode).keys().lookup("y"); ok {
			t.Errorf("mode %v binds y", mode)
		}
		if _, ok := testConfirm(t, mode).keys().lookup("enter"); !ok {
			t.Errorf("mode %v does not bind enter", mode)
		}
	}
}

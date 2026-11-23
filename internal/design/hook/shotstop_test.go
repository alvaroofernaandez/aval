package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const stopBlock = `{"decision":"block","reason":"UI files were edited this session but .design/shots has no render newer than the last edit. Run ` +
	"`node ~/.claude/skills/design-pipeline/scripts/shot.mjs <url|file.html>`" +
	` and quote its paths and numbers, or say \"not visually verified\" in your reply."}` + "\n"

var old = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

type stopEnv struct {
	cache, proj string
}

func newStopEnv(t *testing.T) stopEnv {
	t.Helper()
	e := stopEnv{cache: t.TempDir(), proj: t.TempDir()}
	os.MkdirAll(filepath.Join(e.cache, "design-gate"), 0o755)
	os.MkdirAll(filepath.Join(e.proj, ".design", "shots"), 0o755)
	return e
}

func (e stopEnv) stamp(t *testing.T, sid string, at time.Time) string {
	t.Helper()
	p := StampPath(e.cache, sid)
	if err := os.WriteFile(p, []byte(e.proj+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(p, at, at)
	return p
}

func (e stopEnv) stop(payload string, off bool) string {
	var out bytes.Buffer
	ShotStop(strings.NewReader(payload), &out, StopOptions{Cache: e.cache, Off: off})
	return out.String()
}

func stopPayload(sid string, active bool) string {
	b, _ := json.Marshal(map[string]any{"session_id": sid, "hook_event_name": "Stop", "stop_hook_active": active, "cwd": "/x"})
	return string(b)
}

func TestShotStopBlocksOncePerStamp(t *testing.T) {
	e := newStopEnv(t)
	if got := e.stop(stopPayload("s-none", false), false); got != "" {
		t.Errorf("no UI edit this session must be silent, got %q", got)
	}
	e.stamp(t, "s1", old)
	if got := e.stop(stopPayload("s1", false), false); got != stopBlock {
		t.Fatalf("UI edited and no shot: got %q want the exact block", got)
	}
	if got := e.stop(stopPayload("s1", false), false); got != "" {
		t.Errorf("it blocks only once per stamp, got %q", got)
	}
	// A later UI edit re-stamps: blocked again, once.
	blocked := StampPath(e.cache, "s1") + ".blocked"
	os.Chtimes(blocked, old, old)
	now := time.Now()
	os.Chtimes(StampPath(e.cache, "s1"), now, now)
	if got := e.stop(stopPayload("s1", false), false); got != stopBlock {
		t.Errorf("a newer stamp blocks again, got %q", got)
	}
	if got := e.stop(stopPayload("s1", false), false); got != "" {
		t.Errorf("...and only once again, got %q", got)
	}
}

func TestShotStopRenders(t *testing.T) {
	e := newStopEnv(t)
	shots := filepath.Join(e.proj, ".design", "shots")

	e.stamp(t, "newer", old)
	os.WriteFile(filepath.Join(shots, "home-390.png"), nil, 0o644)
	if got := e.stop(stopPayload("newer", false), false); got != "" {
		t.Errorf("a render newer than the stamp is silent, got %q", got)
	}

	os.Remove(filepath.Join(shots, "home-390.png"))
	p := filepath.Join(shots, "old-390.png")
	os.WriteFile(p, nil, 0o644)
	os.Chtimes(p, old, old)
	e.stamp(t, "older", old.Add(24*time.Hour))
	if got := e.stop(stopPayload("older", false), false); got != stopBlock {
		t.Errorf("a render older than the stamp still blocks, got %q", got)
	}

	hidden := filepath.Join(shots, ".hidden")
	os.WriteFile(hidden, nil, 0o644)
	e.stamp(t, "dot", old)
	if got := e.stop(stopPayload("dot", false), false); got != stopBlock {
		t.Errorf("dotfiles are not renders (shell glob skips them), got %q", got)
	}
}

func TestShotStopSilentCases(t *testing.T) {
	e := newStopEnv(t)
	e.stamp(t, "active", old)
	if got := e.stop(stopPayload("active", true), false); got != "" {
		t.Errorf("stop_hook_active never blocks, got %q", got)
	}
	e.stamp(t, "off", old)
	if got := e.stop(stopPayload("off", false), true); got != "" {
		t.Errorf("bypass is silent, got %q", got)
	}
	for _, bad := range []string{"", "garbage", `{"hook_event_name":"Stop"}`, `{"session_id":"a/b"}`} {
		if got := e.stop(bad, false); got != "" {
			t.Errorf("malformed %q: got %q", bad, got)
		}
	}
	// A stamp that names no project root fails open.
	p := StampPath(e.cache, "noroot")
	os.WriteFile(p, []byte("\n"), 0o644)
	if got := e.stop(stopPayload("noroot", false), false); got != "" {
		t.Errorf("empty root: got %q", got)
	}
}

// The .blocked marker must cover its stamp whatever the clocks say: a file's
// mtime on Linux comes from a coarse kernel clock that can read earlier than
// time.Now(), and a stamp can carry any mtime.
func TestShotStopBlocksOnceEvenWhenTheStampIsAhead(t *testing.T) {
	e := newStopEnv(t)
	ahead := time.Now().Add(time.Second)
	e.stamp(t, "s1", ahead)
	if got := e.stop(stopPayload("s1", false), false); got != stopBlock {
		t.Fatalf("UI edited and no shot: got %q want the exact block", got)
	}
	if got := e.stop(stopPayload("s1", false), false); got != "" {
		t.Errorf("it blocks only once per stamp, even one ahead of the clock, got %q", got)
	}
}

//go:build !windows

package e2e

import (
	"testing"
	"time"
)

// The bounded-wait helpers replace fixed sleeps, so their contract is what the
// other suites stand on:
//
//  1. return as soon as the condition holds, WITHOUT burning the timeout,
//     otherwise every assertion pays the worst case and the run becomes too slow
//  2. keep retrying while it does not hold, and still succeed if it becomes true
//     late, which is the actual race being removed
//  3. on timeout, hand back the LAST value seen rather than an empty string, so
//     the failing assertion reports what the pane really contained
//
// Pure Go: these run under -short too.

func fastPoll(t *testing.T) {
	t.Helper()
	old := pollInterval
	pollInterval = 5 * time.Millisecond
	t.Cleanup(func() { pollInterval = old })
}

func TestWaitUntil(t *testing.T) {
	fastPoll(t)

	t.Run("matches text already present, without burning the timeout", func(t *testing.T) {
		start := time.Now()
		out, ok := waitUntil(10*time.Second, "ready", func() string { return "the pane is ready" })
		eq(t, "matched", true, ok)
		eq(t, "value", "the pane is ready", out)
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("returned after %v, want immediately", d)
		}
	})

	t.Run("keeps polling until the value settles", func(t *testing.T) {
		n := 0
		out, ok := waitUntil(10*time.Second, "settled", func() string {
			n++
			if n > 2 {
				return "settled path"
			}
			return "stale path"
		})
		eq(t, "matched", true, ok)
		eq(t, "value", "settled path", out)
	})

	t.Run("times out and reports what it last saw", func(t *testing.T) {
		out, ok := waitUntil(100*time.Millisecond, "never appears", func() string { return "still empty" })
		eq(t, "matched", false, ok)
		eq(t, "last value, not an empty string", "still empty", out)
	})

	t.Run("tolerates a command that outputs nothing", func(t *testing.T) {
		// A pane that never paints reads as empty: time out with that value
		// rather than hang or error.
		out, ok := waitUntil(100*time.Millisecond, "anything", func() string { return "" })
		eq(t, "matched", false, ok)
		eq(t, "value", "", out)
	})

	t.Run("gives up close to the deadline", func(t *testing.T) {
		start := time.Now()
		waitUntil(300*time.Millisecond, "never appears", func() string { return "x" })
		d := time.Since(start)
		if d < 300*time.Millisecond || d > 1500*time.Millisecond {
			t.Errorf("took %v for a 300ms budget", d)
		}
	})
}

func TestWaitFor(t *testing.T) {
	fastPoll(t)

	t.Run("retries until the condition holds", func(t *testing.T) {
		n := 0
		eq(t, "ok", true, waitFor(10*time.Second, func() bool { n++; return n > 2 }))
	})
	t.Run("false on timeout", func(t *testing.T) {
		eq(t, "ok", false, waitFor(100*time.Millisecond, func() bool { return false }))
	})
	t.Run("returns immediately when already true", func(t *testing.T) {
		start := time.Now()
		eq(t, "ok", true, waitFor(10*time.Second, func() bool { return true }))
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("returned after %v, want immediately", d)
		}
	})
}

func TestStable(t *testing.T) {
	fastPoll(t)

	t.Run("waits out a value that is still being rewritten", func(t *testing.T) {
		n := 0
		got := stable(10*time.Second, 50*time.Millisecond, func() string {
			n++
			if n < 6 {
				return "changing " + string(rune('a'+n))
			}
			return "final"
		})
		eq(t, "value", "final", got)
	})
	t.Run("a value that never settles is returned at the deadline", func(t *testing.T) {
		n := 0
		got := stable(100*time.Millisecond, time.Hour, func() string { n++; return "v" + string(rune('a'+n%20)) })
		if got == "" {
			t.Error("want the last value, got empty")
		}
	})
}

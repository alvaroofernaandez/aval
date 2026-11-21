//go:build !windows

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The routing decision, the clipboard backend choice and each branch's tmux
// calls are unit-tested exhaustively in internal/term/tmux. What only a real process
// can show is the launchd trap: run-shell hands the router a PATH with no
// Homebrew in it, and tmux MUST still be found or the binding dies with
// "returned 127". `env -i` reproduces that here, with a stand-in tmux (TMUX_BIN)
// recording what it was asked to do.

const (
	clipScreenshot = "«class PNGf», 703838, «class AVIF», 164185, «class 8BPS», 5245480, GIF picture, 265978, TIFF picture, 23758990"
	clipText       = "«class utxt», 10, «class ut16», 22"
)

func TestPasteRouterUnderMinimalPATH(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the built binary (skipped under -short)")
	}
	run := func(t *testing.T, clip string, extra ...string) (rc int, calls string) {
		t.Helper()
		dir := t.TempDir()
		env := append([]string{
			"PATH=" + minimalPATH, "HOME=" + t.TempDir(), "E2E_FAKE_DIR=" + dir,
			"TMUX_BIN=" + fakeTool("tmux-stub"), "CLIPBOARD_INFO=" + clip, "PASTE_OS=Darwin", // the fixtures are macOS `clipboard info`; the host OS must not pick the parser
		}, extra...)
		r := runCmd(t, env, 30*time.Second, avalBin, "tmux", "paste-router")
		b, _ := os.ReadFile(filepath.Join(dir, "tmux-stub.txt"))
		return r.rc, string(b)
	}

	t.Run("the image path exits cleanly and sends the 0x16 byte", func(t *testing.T) {
		rc, calls := run(t, clipScreenshot)
		eq(t, "rc", 0, rc)
		wantText(t, "sends 0x16 through send-keys", "send-keys", calls)
		wantText(t, "carries the 0x16 byte", "0x16", calls)
	})

	// The same decision on the Linux parser (wl-paste/xclip list MIME targets),
	// pinned so it runs identically on every host.
	t.Run("the image path on the Linux parser also sends 0x16", func(t *testing.T) {
		rc, calls := run(t, "image/png\nimage/bmp", "PASTE_OS=Linux")
		eq(t, "rc", 0, rc)
		wantText(t, "sends 0x16 through send-keys", "send-keys", calls)
		wantText(t, "carries the 0x16 byte", "0x16", calls)
	})

	t.Run("the text path never sends 0x16", func(t *testing.T) {
		rc, calls := run(t, clipText)
		eq(t, "rc", 0, rc)
		eq(t, "no 0x16", false, strings.Contains(calls, "0x16"))
		// Pasting needs a clipboard tool (pbpaste on macOS, always in /usr/bin);
		// elsewhere the text path is a quiet no-op, which the unit tests cover.
		if _, err := os.Stat("/usr/bin/pbpaste"); err == nil {
			wantText(t, "pastes through tmux", "paste-buffer", calls)
		}
	})

	// tmux must be located without help from PATH.
	t.Run("resolves a real tmux when PATH does not provide one", func(t *testing.T) {
		if realTmux == "" {
			t.Skip("tmux is not installed")
		}
		r := runCmd(t, []string{"PATH=" + minimalPATH, "HOME=" + t.TempDir(), "RESOLVE_ONLY=1"}, 30*time.Second, avalBin, "tmux", "paste-router")
		eq(t, "rc", 0, r.rc)
		st, err := os.Stat(r.out)
		eq(t, "prints the path of an executable", true, err == nil && st.Mode().Perm()&0o111 != 0)
	})
}

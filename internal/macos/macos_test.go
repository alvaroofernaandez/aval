package macos

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recorder struct{ calls []string }

func (r *recorder) run(name string, args ...string) error {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	return nil
}

func TestApplyDefaults(t *testing.T) {
	r := &recorder{}
	if err := ApplyDefaults(r.run); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.calls, "\n")
	for _, want := range []string{
		"defaults write NSGlobalDomain KeyRepeat -int 2",
		"defaults write NSGlobalDomain InitialKeyRepeat -int 15",
		"defaults write NSGlobalDomain AppleShowAllExtensions -bool true",
		"defaults write com.apple.finder ShowPathbar -bool true",
		"killall Finder",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
}

func put(t *testing.T, p, body string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestCopyNewestScreenshot(t *testing.T) {
	dir, state := t.TempDir(), t.TempDir()
	now := time.Now()
	put(t, filepath.Join(dir, "Captura 1.png"), "old", now.Add(-time.Hour))
	put(t, filepath.Join(dir, "Captura 2.png"), "new", now)
	put(t, filepath.Join(dir, "notes.txt"), "txt", now.Add(time.Minute))

	var copied []string
	opt := CopyOptions{Dir: dir, StateDir: state, Clip: func(p string) error { copied = append(copied, p); return nil }}

	if err := CopyNewest(opt); err != nil {
		t.Fatal(err)
	}
	if len(copied) != 1 || copied[0] != filepath.Join(dir, "Captura 2.png") {
		t.Fatalf("copied = %v", copied)
	}

	if err := CopyNewest(opt); err != nil || len(copied) != 1 {
		t.Fatalf("the same screenshot was copied twice: %v %v", copied, err)
	}

	put(t, filepath.Join(dir, ".Captura 3.png"), "tmp", now.Add(2*time.Minute))
	if err := CopyNewest(opt); err != nil || len(copied) != 1 {
		t.Fatalf("hidden macOS temp file was copied: %v %v", copied, err)
	}

	put(t, filepath.Join(dir, "Captura 4.JPG"), "jpg", now.Add(3*time.Minute))
	if err := CopyNewest(opt); err != nil || len(copied) != 2 || !strings.HasSuffix(copied[1], "Captura 4.JPG") {
		t.Fatalf("jpg not copied: %v %v", copied, err)
	}
}

func TestCopyNewestEmptyFolderAndClipFailure(t *testing.T) {
	dir, state := t.TempDir(), t.TempDir()
	if err := CopyNewest(CopyOptions{Dir: dir, StateDir: state, Clip: func(string) error { t.Fatal("nothing to copy"); return nil }}); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(dir, "a.png"), "x", time.Now())
	fail := CopyOptions{Dir: dir, StateDir: state, Clip: func(string) error { return os.ErrPermission }}
	if err := CopyNewest(fail); err == nil {
		t.Fatal("a failing clipboard must fail the run")
	}
	var n int
	ok := CopyOptions{Dir: dir, StateDir: state, Clip: func(string) error { n++; return nil }}
	if err := CopyNewest(ok); err != nil || n != 1 {
		t.Fatalf("a failed copy must not be remembered: n=%d err=%v", n, err)
	}
}

func TestClipScriptEscapesAndPicksClass(t *testing.T) {
	for path, class := range map[string]string{
		"/a/b.png": "«class PNGf»", "/a/b.JPEG": "JPEG picture", "/a/b.heic": "TIFF picture",
	} {
		s := clipScript(path)
		if !strings.Contains(s, class) || !strings.Contains(s, path) {
			t.Errorf("%s: %s", path, s)
		}
	}
	if s := clipScript(`/a/"q"\.png`); !strings.Contains(s, `\"q\"\\.png`) {
		t.Errorf("quotes not escaped: %s", s)
	}
}

func TestInstallClipboardAgentWritesPlistPointingAtAval(t *testing.T) {
	home := t.TempDir()
	r := &recorder{}
	err := InstallClipboardAgent(InstallOptions{Home: home, Bin: filepath.Join(home, ".local/bin/aval"), SkipDefaults: true, SkipLaunchctl: true, Run: r.run, UID: 501})
	if err != nil {
		t.Fatal(err)
	}
	shots := filepath.Join(home, "Pictures", "Screenshots")
	if fi, err := os.Stat(shots); err != nil || !fi.IsDir() {
		t.Fatal("screenshot folder not created")
	}
	plist := filepath.Join(home, "Library/LaunchAgents/dev.aval.screenshot-clipboard.plist")
	b, err := os.ReadFile(plist)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<string>dev.aval.screenshot-clipboard</string>",
		"<string>" + filepath.Join(home, ".local/bin/aval") + "</string>",
		"<string>macos</string>", "<string>screenshot-clipboard</string>", "<string>copy</string>",
		"<string>" + shots + "</string>", "WatchPaths", "<key>ThrottleInterval</key>",
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("plist missing %q:\n%s", want, b)
		}
	}
	if len(r.calls) != 0 {
		t.Fatalf("skip flags ignored: %v", r.calls)
	}
	if _, err := exec.LookPath("plutil"); err == nil {
		if out, err := exec.Command("plutil", "-lint", plist).CombinedOutput(); err != nil {
			t.Fatalf("plist invalid: %s", out)
		}
	}
}

func TestInstallClipboardAgentRunsDefaultsAndLaunchctl(t *testing.T) {
	home := t.TempDir()
	r := &recorder{}
	if err := InstallClipboardAgent(InstallOptions{Home: home, Bin: "/x/aval", Run: r.run, UID: 501}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.calls, "\n")
	for _, want := range []string{
		"defaults write com.apple.screencapture location " + filepath.Join(home, "Pictures", "Screenshots"),
		"defaults write com.apple.screencapture type png",
		"killall SystemUIServer",
		"launchctl bootout gui/501/dev.aval.screenshot-clipboard",
		"launchctl bootstrap gui/501 ",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
}

func TestPlistEscapesXML(t *testing.T) {
	if s := agentPlist("l", "/a&b/aval", "/c<d"); !strings.Contains(s, "/a&amp;b/aval") || !strings.Contains(s, "/c&lt;d") {
		t.Fatalf("not escaped: %s", s)
	}
}

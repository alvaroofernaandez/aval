package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The pre-push hook is the last gate before anything reaches the public repo.
//
// Twice on 2026-11-21 employer material reached a commit: once a subagent
// committed an uncommitted file, once a task log named work repositories and was
// pushed without running the suite. Both times the check existed and was simply
// not run; a hook makes it unskippable by default.
//
// The scanning itself is unit-tested in internal/agent/prepush and cmd/aval; what is
// pinned here is the shim and a real `git push` through it.

func TestPrePushHookShim(t *testing.T) {
	hook := p(".githooks", "pre-push")
	st, err := os.Stat(hook)
	if err != nil {
		t.Fatal(err)
	}
	// Git silently ignores a hook that is not executable.
	isTrue(t, "the pre-push hook is versioned and executable", st.Mode()&0o111 != 0)
	body := readAbs(t, hook)
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	eq(t, "the hook is a thin exec of the Go binary", `exec "$a" hook pre-push "$@"`, lines[len(lines)-1])
	// Same resolution as the Claude hooks: ~/.local/bin/aval first, PATH second,
	// so a push works even when ~/.local/bin is not on PATH (GUI git clients).
	mustContain(t, body, `a="$HOME/.local/bin/aval"; [ -x "$a" ] || a=aval`, "the hook prefers ~/.local/bin/aval")
	eq(t, "the hook carries no logic of its own (lines)", 3, strings.Count(body, "\n"))
}

func TestPrePushThisCloneUsesTheVersionedHooks(t *testing.T) {
	needGit(t)
	r := run(repo, nil, "", "git", "config", "--get", "core.hooksPath")
	eq(t, "core.hooksPath of this clone", ".githooks", strings.TrimSpace(r.Stdout))
}

// Behaviour: in a scratch clone, a commit carrying employer material is refused,
// and sync drift warns (or blocks under AVAL_PREPUSH_STRICT=1).
func TestPrePushRealGitPushThroughTheHook(t *testing.T) {
	t.Parallel()
	needGit(t)
	slow(t)
	aval := avalBin(t)
	work := t.TempDir()
	home := filepath.Join(work, "home")
	_ = os.MkdirAll(home, 0o755)

	stub := func(name, script string) string {
		f := filepath.Join(work, name)
		write(t, f, script)
		_ = os.Chmod(f, 0o755)
		return f
	}
	okStub := stub("aval-ok", "#!/bin/sh\nexit 0\n")
	driftStub := stub("aval-drift", "#!/bin/sh\necho \"drift  settings  hooks\"\nexit 1\n")

	// Real drift is the machine's business: default to a binary that reports none.
	env := func(kv ...string) []string {
		return isoEnv(home, append([]string{"PATH=" + filepath.Dir(aval) + ":" + os.Getenv("PATH"), "AVAL_BIN=" + okStub,
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"}, kv...)...)
	}
	clone := filepath.Join(work, "clone")
	if r := run(work, env(), "", "git", "clone", "-q", "--no-hardlinks", repo, clone); r.Code != 0 {
		t.Fatalf("clone failed: %s", r.Out())
	}
	// Test the working copy of the hook, not whatever is committed.
	installHook := func() {
		write(t, clone+"/.githooks/pre-push", read(t, ".githooks/pre-push"))
		_ = os.Chmod(clone+"/.githooks/pre-push", 0o755)
	}
	installHook()
	git(t, clone, "config", "core.hooksPath", ".githooks")
	git(t, work, "init", "-q", "--bare", "remote.git")
	git(t, clone, "remote", "add", "scratch", work+"/remote.git")

	commit := func(file, content, msg string) {
		write(t, filepath.Join(clone, file), content)
		git(t, clone, "add", file)
		git(t, clone, "commit", "-qm", msg)
	}
	push := func(kv ...string) result {
		return run(clone, env(kv...), "", "git", "push", "-q", "scratch", "HEAD:main")
	}

	// Positive control: a clean commit must go through, or a hook that always
	// fails would pass the refusal assertion below.
	commit("clean.md", "clean note\n", "test: clean")
	r := push()
	eq(t, "a clean push goes through\n"+r.Out(), 0, r.Code)

	h := "haga"
	commit("leak.md", "notes about "+h+"link-projects\n", "test: leak")
	r = push()
	isTrue(t, "a push carrying employer material is refused", r.Code != 0)
	mustContain(t, r.Out(), "employer material matching", "...for the right reason")

	// aval sync --check: a WARNING by default (another machine's ~/.claude must
	// not block a push), blocking only with AVAL_PREPUSH_STRICT=1, silent when the
	// binary is absent. A stub stands in for the real binary.
	git(t, clone, "reset", "-q", "--hard", "HEAD~1") // drop the refused leak commit
	installHook()                                    // reset restores the committed hook file

	commit("clean2.md", "second clean note\n", "test: clean 2")
	r = push("AVAL_BIN=" + driftStub)
	eq(t, "sync drift only warns: the push goes through\n"+r.Out(), 0, r.Code)
	mustHave(t, `warning.*aval sync --check`, r.Out(), "sync drift is reported as a warning")

	commit("clean3.md", "third clean note\n", "test: clean 3")
	r = push("AVAL_PREPUSH_STRICT=1", "AVAL_BIN="+driftStub)
	isTrue(t, "AVAL_PREPUSH_STRICT=1 makes sync drift block the push", r.Code != 0)

	r = push("AVAL_PREPUSH_STRICT=1", "AVAL_BIN="+okStub)
	eq(t, "strict mode with no drift pushes\n"+r.Out(), 0, r.Code)

	commit("clean4.md", "fourth clean note\n", "test: clean 4")
	r = push("AVAL_PREPUSH_STRICT=1", "AVAL_BIN="+filepath.Join(work, "does-not-exist"))
	eq(t, "no aval binary: nothing to check, even in strict mode\n"+r.Out(), 0, r.Code)
}

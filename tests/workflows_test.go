package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// GitHub Actions runs in a public repo: every workflow is attack surface.
// These invariants are the reason Actions could be switched on at all:
//   - third-party code only by immutable commit SHA, and only GitHub-owned actions;
//   - read-only token by default, write only where a job needs it;
//   - nothing that runs untrusted PR code with secrets (pull_request_target);
//   - publishing never runs on a plain push: tags or manual dispatch, behind the
//     "release" environment, which requires the owner's approval.

func workflows(t *testing.T) map[string]string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(repo, ".github", "workflows", "*.yml"))
	if len(files) == 0 {
		t.Fatal("no workflows found")
	}
	out := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(f)] = string(b)
	}
	return out
}

var (
	usesRe    = regexp.MustCompile(`(?m)^\s*-?\s*uses:\s*([^\s#]+)`)
	pinnedRe  = regexp.MustCompile(`^actions/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)
	topPermRe = regexp.MustCompile(`(?m)^permissions:[ \t]*\n((?:[ \t]+.+\n)+)`)
)

func TestWorkflowsAreHardened(t *testing.T) {
	for name, wf := range workflows(t) {
		for _, m := range usesRe.FindAllStringSubmatch(wf, -1) {
			if !pinnedRe.MatchString(m[1]) {
				t.Errorf("%s: %q must be a GitHub-owned action pinned to a full commit SHA", name, m[1])
			}
		}
		if strings.Contains(wf, "pull_request_target") {
			t.Errorf("%s: pull_request_target runs fork code with secrets", name)
		}
		top := topPermRe.FindStringSubmatch(wf)
		if top == nil {
			t.Errorf("%s: needs a top-level permissions block", name)
		} else if strings.Contains(top[1], "write") {
			t.Errorf("%s: top-level permissions must be read-only; grant write per job", name)
		}
		checkouts := strings.Count(wf, "actions/checkout@")
		if got := strings.Count(wf, "persist-credentials: false"); got < checkouts {
			t.Errorf("%s: every checkout needs persist-credentials: false (%d of %d)", name, got, checkouts)
		}
		for _, bad := range []string{"@latest", "| sh", "| bash", "releases/latest/download"} {
			if strings.Contains(wf, bad) {
				t.Errorf("%s: unpinned download or piped install (%q)", name, bad)
			}
		}
	}
}

func TestPublishingNeedsATagOrApproval(t *testing.T) {
	for name, wf := range workflows(t) {
		publishes := strings.Contains(wf, "npm publish") || strings.Contains(wf, "gh release")
		if !publishes {
			continue
		}
		if regexp.MustCompile(`(?m)^\s+branches:`).MatchString(wf) {
			t.Errorf("%s: publishes on a branch push; use tags or workflow_dispatch", name)
		}
		if !strings.Contains(wf, "environment: release") {
			t.Errorf("%s: publishing jobs must run in the approval-gated \"release\" environment", name)
		}
	}
}

func TestCIRunsOnPullRequestsWithoutSecrets(t *testing.T) {
	ci, ok := workflows(t)["ci.yml"]
	if !ok {
		t.Fatal("missing .github/workflows/ci.yml")
	}
	if !strings.Contains(ci, "pull_request:") {
		t.Error("ci.yml must run on pull_request")
	}
	if strings.Contains(ci, "secrets.") {
		t.Error("ci.yml must not use secrets: it runs on pull requests")
	}
}

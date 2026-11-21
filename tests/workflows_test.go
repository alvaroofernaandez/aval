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

var (
	jobHeadRe = regexp.MustCompile(`(?m)^  ([a-z][a-z0-9_-]*):[ \t]*$`)
	// A step that publishes: to npm, to a GitHub release, or GoReleaser's real
	// (non --snapshot) release.
	publishRe = regexp.MustCompile(`npm publish|gh release (create|upload|edit)|goreleaser release`)
)

// jobs splits the `jobs:` section into name -> body.
func jobs(wf string) map[string]string {
	_, section, ok := strings.Cut(wf, "\njobs:\n")
	if !ok {
		return nil
	}
	heads := jobHeadRe.FindAllStringSubmatchIndex(section, -1)
	out := map[string]string{}
	for i, h := range heads {
		end := len(section)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		out[section[h[2]:h[3]]] = section[h[1]:end]
	}
	return out
}

func publishes(job string) bool {
	for _, line := range strings.Split(job, "\n") {
		if publishRe.MatchString(line) && !strings.Contains(line, "--snapshot") {
			return true
		}
	}
	return false
}

func TestPublishingNeedsATagOrApproval(t *testing.T) {
	for name, wf := range workflows(t) {
		var publishing []string
		for jobName, body := range jobs(wf) {
			if !publishes(body) {
				continue
			}
			publishing = append(publishing, jobName)
			if !strings.Contains(body, "environment: release") {
				t.Errorf("%s: job %q publishes, so it must run in the approval-gated \"release\" environment", name, jobName)
			}
		}
		if len(publishing) > 0 && regexp.MustCompile(`(?m)^\s+branches:`).MatchString(wf) {
			t.Errorf("%s: publishes on a branch push; use tags or workflow_dispatch", name)
		}
	}
}

// GoReleaser is downloaded, not installed from a third-party action: the URL
// must name an exact version, and the tarball's sha256 is checked before it runs.
func TestGoReleaserIsPinnedAndVerified(t *testing.T) {
	urlRe := regexp.MustCompile(`https://github\.com/goreleaser/goreleaser/releases/download/\$\{GORELEASER_VERSION\}/\S+`)
	versionRe := regexp.MustCompile(`(?m)^  GORELEASER_VERSION: v\d+\.\d+\.\d+$`)
	shaRe := regexp.MustCompile(`(?m)^  GORELEASER_SHA256: [0-9a-f]{64}$`)

	found := false
	for name, wf := range workflows(t) {
		for _, line := range strings.Split(wf, "\n") {
			if !strings.Contains(line, "curl") && !strings.Contains(line, "wget") {
				continue
			}
			if !urlRe.MatchString(line) {
				t.Errorf("%s: download %q must be the version-pinned GoReleaser URL", name, strings.TrimSpace(line))
			}
		}
		if !strings.Contains(wf, "goreleaser") {
			continue
		}
		found = true
		if !versionRe.MatchString(wf) || !shaRe.MatchString(wf) {
			t.Errorf("%s: GORELEASER_VERSION (exact vX.Y.Z) and GORELEASER_SHA256 (64 hex) must be set at the top level", name)
		}
		if !strings.Contains(wf, "sha256sum -c") {
			t.Errorf("%s: the GoReleaser tarball must be verified with sha256sum -c before it is extracted", name)
		}
	}
	if !found {
		t.Error("no workflow runs goreleaser")
	}
}

// The binaries npm ships and the ones GoReleaser builds are the same six.
func TestGoReleaserTargetsMatchNpm(t *testing.T) {
	cfg, err := os.ReadFile(filepath.Join(repo, ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	list := func(key string) []string {
		m := regexp.MustCompile(`(?m)^\s+` + key + `: \[([^\]]+)\]`).FindStringSubmatch(string(cfg))
		if m == nil {
			t.Fatalf(".goreleaser.yaml has no %s list", key)
		}
		return strings.Split(strings.ReplaceAll(m[1], " ", ""), ",")
	}
	want := map[string]bool{}
	for _, goos := range list("goos") {
		for _, goarch := range list("goarch") {
			want[goos+"/"+goarch] = true
		}
	}

	resolve, err := os.ReadFile(filepath.Join(repo, "npm", "cli", "lib", "resolve.js"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range regexp.MustCompile(`goos: "(\w+)", goarch: "(\w+)"`).FindAllStringSubmatch(string(resolve), -1) {
		got[m[1]+"/"+m[2]] = true
	}
	for target := range want {
		if !got[target] {
			t.Errorf("GoReleaser builds %s but npm/cli/lib/resolve.js does not package it", target)
		}
	}
	for target := range got {
		if !want[target] {
			t.Errorf("npm packages %s but .goreleaser.yaml does not build it", target)
		}
	}
	if len(got) != 6 {
		t.Errorf("expected 6 targets in resolve.js, found %d", len(got))
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

package check

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func noTools(string) (string, error) { return "", exec.ErrNotFound }

func TestMissingToolsAreReportedOnceWithThePinnedInstallCommand(t *testing.T) {
	err := Preflight(plan(t, PlanOptions{GOOS: "linux"}), noTools)
	var m *MissingToolsError
	if !errors.As(err, &m) || !errors.Is(err, ErrMissingTool) {
		t.Fatalf("err = %v, want *MissingToolsError wrapping ErrMissingTool", err)
	}
	var got []string
	for _, tool := range m.Tools {
		got = append(got, tool.Name)
	}
	want := []string{"gofmt", "go", "node", "actionlint", "npx", "gitleaks"}
	if !slices.Equal(got, want) {
		t.Errorf("missing = %v, want %v", got, want)
	}
	for _, tool := range m.Tools {
		if tool.Name == "actionlint" && tool.Install != "go install github.com/rhysd/actionlint/cmd/actionlint@"+ActionlintVersion {
			t.Errorf("actionlint install = %q", tool.Install)
		}
		if tool.Name == "gitleaks" && tool.Install != "go install github.com/zricethezav/gitleaks/v8@"+GitleaksVersion {
			t.Errorf("gitleaks install = %q", tool.Install)
		}
	}
}

func TestNothingIsMissingWhenEveryToolIsOnThePath(t *testing.T) {
	found := func(n string) (string, error) { return "/bin/" + n, nil }
	if err := Preflight(plan(t, PlanOptions{GOOS: "linux"}), found); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightIgnoresSkippedSteps(t *testing.T) {
	// --fast leaves gitleaks and npx out, so their absence is fine.
	look := func(n string) (string, error) {
		if n == "gitleaks" || n == "npx" {
			return "", exec.ErrNotFound
		}
		return "/bin/" + n, nil
	}
	if err := Preflight(plan(t, PlanOptions{GOOS: "linux", Fast: true}), look); err != nil {
		t.Fatalf("fast run: %v", err)
	}
	if err := Preflight(plan(t, PlanOptions{GOOS: "linux"}), look); !errors.Is(err, ErrMissingTool) {
		t.Fatalf("full run: err = %v, want ErrMissingTool", err)
	}
}

func TestEveryToolOfTheTableHasAPinnedInstallHint(t *testing.T) {
	for _, s := range Steps {
		for _, tool := range append([]string{s.Tool}, s.Needs...) {
			p, ok := Pins[tool]
			if !ok {
				t.Errorf("step %s: tool %q has no pin", s.Name, tool)
				continue
			}
			if p.Version == "" || p.Install == "" {
				t.Errorf("pin %q needs a version and an install hint: %+v", tool, p)
			}
			if strings.Contains(strings.ToLower(p.Install), "latest") || strings.Contains(p.Install, "@") && !strings.Contains(p.Install, "@"+p.Version) {
				t.Errorf("pin %q install %q is not exactly %s", tool, p.Install, p.Version)
			}
		}
	}
}

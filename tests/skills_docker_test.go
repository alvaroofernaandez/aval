package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Kiri-only Docker skills. The cleanup command itself is covered by Go
// tests (internal/dockerclean). Kiri replaced OrbStack (which replaced Colima)
// on 2026-11-17: no skill may instruct starting either of them.

func skill(t *testing.T, name string) string { return read(t, "shared/skills/"+name+"/SKILL.md") }

func TestDockerSkillsHaveValidFrontmatterAndNameNoRetiredRuntime(t *testing.T) {
	for _, s := range []string{"kiri-docker", "docker-space-optimizer", "kiri-health", "docker-build-optimized"} {
		t.Run(s, func(t *testing.T) {
			f := skill(t, s)
			mustHave(t, `^name: `+s+`$`, f, s+": frontmatter name matches dir")
			mustHave(t, `^description: ".+"$`, f, s+": description is one quoted line")
			// Colima/OrbStack may only be named as something to detect or refuse,
			// never as a runtime to use.
			mustNotHave(t, `(?i)(colima start|brew install colima|--context colima|brew install --cask orbstack|orb start|orbctl start|--context orbstack|context use orbstack)`,
				f, s+": instructs to start/install Colima or OrbStack")
			mustNotHave(t, `orbctl`, f, s+": orbctl invocation")
		})
	}
}

func TestDockerSkillsContent(t *testing.T) {
	isTrue(t, "old orbstack-docker skill is gone", !exists(p("shared/skills/orbstack-docker")))
	isTrue(t, "old orbstack-health skill is gone", !exists(p("shared/skills/orbstack-health")))
	mustHave(t, `com\.docker\.socket`, skill(t, "kiri-health"),
		"kiri-health detects Docker Desktop's socket helper hijacking /var/run/docker.sock")
	mustContain(t, skill(t, "kiri-docker"), "BUILDX_BUILDER=multiarch", "kiri-docker documents BUILDX_BUILDER=multiarch")
	mustContain(t, skill(t, "docker-build-optimized"), "image inspect -f '{{.Architecture}}'",
		"docker-build-optimized verifies the image architecture")
	mustContain(t, skill(t, "docker-space-optimizer"), "aval docker cleanup", "docker-space-optimizer points to aval docker cleanup")
	mustContain(t, skill(t, "kiri-docker"), "aval kiri up", "kiri-docker points to aval kiri up")

	var scripts []string
	for _, s := range []string{"kiri-docker", "docker-space-optimizer"} {
		_ = filepath.WalkDir(p("shared/skills", s), func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".sh") {
				scripts = append(scripts, path)
			}
			return nil
		})
	}
	eq(t, "shell scripts left in the docker skills", "", strings.Join(scripts, " "))
}

package tests

import (
	"bufio"
	"crypto/rand"
	"math/big"
	"os/exec"
	"strings"
	"testing"
)

// Guards against committing credentials. A token once reached GitHub from this
// repo because a hand-rolled scan missed ENGRAM_CLOUD_TOKEN; a dedicated
// scanner replaced that guesswork, and the canary below proves it detects.

func TestSecretsGitleaksFindsNothingInTreeOrHistory(t *testing.T) {
	t.Parallel()
	needTool(t, "gitleaks")
	needGit(t)
	slow(t)
	// Judged by exit status: gitleaks prints "no leaks found" on success.
	r := run(repo, nil, "", "gitleaks", "detect", "--source", ".", "--redact", "--no-banner")
	eq(t, "gitleaks exit code (0 = clean)\n"+r.Out(), 0, r.Code)
}

// The value is reassembled at run time: spelled in full, the history scan
// below would find this very file and report the token as still present.
func TestSecretsRotatedEngramTokenAbsentFromEveryCommit(t *testing.T) {
	t.Parallel()
	needGit(t)
	slow(t)
	rotated := "7c346f3e53d1b1a79c6aacaa4f10302" + "49c42901779c304c24ba409a5ef00e045"

	cmd := exec.Command("git", "log", "--all", "-p")
	cmd.Dir = repo
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	hits := 0
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		if strings.Contains(sc.Text(), rotated) {
			hits++
		}
	}
	// Drain so git is never killed by SIGPIPE; its status is irrelevant here.
	_ = cmd.Wait()
	eq(t, "commit lines carrying the rotated token", 0, hits)
}

func TestSecretsTokenIsReferencedByEnvVarNotByValue(t *testing.T) {
	mustHave(t, `"ENGRAM_CLOUD_TOKEN"\s*:\s*"\{env:`, read(t, "config/opencode/opencode.json"),
		"opencode.json must reference the token through {env:...}")
}

// Without this a silently broken scanner would report success forever, which
// is precisely how the token got through the first time. The canary is built at
// run time so no real token pattern sits in this file.
func TestSecretsPlantedCredentialIsDetected(t *testing.T) {
	needTool(t, "gitleaks")
	const alnum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	var sb strings.Builder
	for i := 0; i < 36; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alnum))))
		if err != nil {
			t.Fatal(err)
		}
		sb.WriteByte(alnum[n.Int64()])
	}
	canary := "gh" + "p_" + sb.String()
	dir := t.TempDir()
	write(t, dir+"/planted.json", "{\n  \"environment\": {\n    \"SOME_SERVICE_TOKEN\": \""+canary+"\"\n  }\n}\n")
	r := run(dir, nil, "", "gitleaks", "detect", "--source", dir, "--no-git", "--redact", "--no-banner")
	eq(t, "gitleaks exit code on a planted token (1 = findings)\n"+r.Out(), 1, r.Code)
}

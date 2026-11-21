package tests

import (
	"os"
	"strings"
	"testing"
)

// ~/.zshrc carries no credentials, so it can be versioned. Credentials live in
// ~/.config/zsh/secrets.zsh, outside the repo.

func TestZshrcSecretsVersionedZshrcExists(t *testing.T) {
	isTrue(t, "home/zshrc exists", exists(p("home", "zshrc")))
}

// gitleaks found only 2 of the 4 secret-bearing lines in the original file, so
// it is paired with explicit patterns rather than trusted on its own.
func TestZshrcSecretsGitleaksFindsNoCredentials(t *testing.T) {
	needTool(t, "gitleaks")
	dir := t.TempDir()
	write(t, dir+"/zshrc.sh", read(t, "home/zshrc"))
	// Exit 0 clean, 1 findings; the output text is not parsed.
	r := run(dir, nil, "", "gitleaks", "detect", "--source", dir, "--no-git", "--redact", "--no-banner")
	eq(t, "gitleaks exit code (0 = clean)\n"+r.Out(), 0, r.Code)
}

func TestZshrcSecretsNoLiteralSecretInTheFile(t *testing.T) {
	zshrc := read(t, "home/zshrc")
	mustNotHave(t, `(TOKEN|API_KEY|SECRET|PASSWORD)\s*=\s*"?[A-Za-z0-9_-]{20,}`, zshrc,
		"an assignment carries a long literal secret")
	for _, pat := range []string{`apikey[A-Za-z0-9]{20,}`, `ghp_[A-Za-z0-9]{20,}`, `sk-[A-Za-z0-9]{20,}`, `[0-9a-f]{56,}`} {
		mustNotHave(t, pat, zshrc, "value matching /"+pat+"/")
	}
}

func TestZshrcSecretsLoadedFromOutsideTheRepoAndGuarded(t *testing.T) {
	zshrc := read(t, "home/zshrc")
	mustHave(t, `secrets\.zsh`, zshrc, "sources an out-of-repo secrets file")
	mustHave(t, `\[ *-[fr] *"?\$\{?HOME.*secrets\.zsh`, zshrc,
		"the source is guarded so a missing file does not break the shell")
}

func TestZshrcSecretsFileIsNeverVersioned(t *testing.T) {
	needGit(t)
	r := run(repo, nil, "", "git", "ls-files")
	eq(t, "tracked files named secrets.zsh", 0, count(`secrets\.zsh`, r.Stdout))
	mustHave(t, `secrets\.zsh|zsh/secrets`, read(t, ".gitignore"), "the zsh secrets path is gitignored")
}

// The split from the old zshrc did not lose anything. This inspects the real
// ~/.config/zsh/secrets.zsh of the machine running the suite, so it is opt-in
// (AVAL_LIVE_CHECKS=1): a test that reads $HOME must not do it by default.
func TestZshrcSecretsLiveSecretsFileIsIntact(t *testing.T) {
	if os.Getenv("AVAL_LIVE_CHECKS") != "1" {
		t.Skip("reads the real $HOME; set AVAL_LIVE_CHECKS=1 to run")
	}
	home, _ := os.UserHomeDir()
	path := home + "/.config/zsh/secrets.zsh"
	st, err := os.Stat(path)
	if err != nil {
		t.Skipf("%s absent", path)
	}
	body := readAbs(t, path)
	for _, v := range []string{"ENGRAM_CLOUD_TOKEN", "DOKPLOY_API_KEY"} {
		isTrue(t, v+" still defined in the secrets file", strings.Contains(body, v))
	}
	eq(t, "the secrets file mode", os.FileMode(0o600), st.Mode().Perm())
}

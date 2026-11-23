package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// cloneURL is the canonical remote, as `git remote -v` and the README give it.
const cloneURL = "https://github.com/alvaroofernaandez/aval.git"

// gitCloner clones the repository with the git on PATH. It is the welcome
// screen's real Cloner; tests inject a fake and never reach the network.
type gitCloner struct {
	ctx context.Context
	url string
}

// Clone runs `git clone <url> <dest>`. Credential prompts are disabled: the
// alt screen would hide them and the clone would hang. The error carries
// git's last line of output, which names the cause.
func (g gitCloner) Clone(dest string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("git is not installed. Install git, then try again")
	}
	cmd := exec.CommandContext(g.ctx, "git", "clone", "--quiet", g.url, dest)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if reason := lastLine(string(out)); reason != "" {
		return fmt.Errorf("%s", strings.TrimPrefix(reason, "fatal: "))
	}
	return err
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

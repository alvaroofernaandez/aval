package main

import (
	"fmt"
	"io"
	"os"

	"github.com/alvaroofernaandez/aval/internal/core/deps"
)

// cmdIO is what a command gets from the process: its streams and its
// environment. main passes the real ones; tests pass buffers and a map, so a
// command runs in process with nothing shared between two calls.
type cmdIO struct {
	stdin          io.Reader
	stdout, stderr io.Writer
	env            func(string) string
	// prober finds the tools of the dependency inventory; nil is the real
	// machine, probed through env's PATH.
	prober deps.Prober
}

// exitCode is an error that ends run with exactly this code and prints
// nothing: tmux and `aval dev run` hand back the code of what they ran.
type exitCode int

func (c exitCode) Error() string { return fmt.Sprintf("exit status %d", int(c)) }

// runStandalone runs the commands that need no repository and no update
// check: the skill tooling, dev and deps. handled is false for any other
// command, which dispatch then resolves as before.
func runStandalone(name string, sub []string, repoFlag string, ui uiFlags, c cmdIO) (handled bool, err error) {
	switch name {
	case "kiri":
		return true, runKiri(sub)
	case "docker":
		return true, runDocker(sub)
	case "pdf":
		return true, runPDF(sub, c.stdout)
	case "deck":
		return true, runDeck(sub, c.stdout, realDeckDeps())
	case "dev":
		return true, runDev(repoFlag, sub)
	case "deps":
		return true, runDeps(sub, ui, c)
	case "relay":
		return true, runRelay(sub, ui, c)
	case "context":
		wd, _ := os.Getwd()
		return true, runContext(sub, wd, c)
	}
	return false, nil
}

//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
)

// execReplace runs the child and ends with its exit code: Windows has no exec.
func execReplace(path string, argv, env []string) error {
	cmd := exec.Command(path, argv[1:]...)
	cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = env, os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return exitCode(ee.ExitCode())
		}
		return err
	}
	return nil
}

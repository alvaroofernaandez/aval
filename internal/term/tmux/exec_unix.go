//go:build !windows

package tmux

import (
	"math/rand/v2"
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

func goos() string { return runtime.GOOS }

func randInt(n int) int { return rand.IntN(n) }

// execReplace replaces this process, so the terminal's pty belongs to tmux or
// the shell directly and signals reach them untouched.
func execReplace(path string, argv []string) error {
	return syscall.Exec(path, argv, os.Environ())
}

// spawnDetached starts a child that outlives us: its own session, no stdio.
// A background child holding the parent's descriptors would block whoever
// reads them, which hangs tmux's run-shell and any pipeline around it.
func spawnDetached(path string, args, env []string) error {
	cmd := exec.Command(path, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

//go:build !windows

package reaper

import "syscall"

func killPID(pid int) bool { return syscall.Kill(pid, syscall.SIGKILL) == nil }

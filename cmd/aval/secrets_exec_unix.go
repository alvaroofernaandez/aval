//go:build !windows

package main

import "syscall"

// execReplace replaces the process, so signals and the exit code are the
// child's own.
func execReplace(path string, argv, env []string) error { return syscall.Exec(path, argv, env) }

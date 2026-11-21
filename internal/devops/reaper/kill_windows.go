//go:build windows

package reaper

import "os"

func killPID(pid int) bool {
	p, err := os.FindProcess(pid)
	return err == nil && p.Kill() == nil
}

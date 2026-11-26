package facts

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// maxProcDepth bounds the walk up the process tree.
const maxProcDepth = 12

// ClaudePID returns the id of the nearest ancestor of start whose name
// contains "claude" (the process that survives /clear; hooks run under it,
// possibly through a shell), or 0 when there is none. parent returns the
// parent id and the name of a process.
func ClaudePID(start int, parent func(pid int) (ppid int, name string, ok bool)) int {
	pid, seen := start, map[int]bool{}
	for range maxProcDepth {
		ppid, _, ok := parent(pid)
		if !ok || ppid <= 1 || seen[ppid] {
			return 0
		}
		seen[pid] = true
		_, name, ok := parent(ppid)
		if !ok {
			return 0
		}
		if strings.Contains(strings.ToLower(name), "claude") {
			return ppid
		}
		pid = ppid
	}
	return 0
}

// OwnerID is the id of the Claude Code process above this one as text, or ""
// when it cannot be found.
func OwnerID() string {
	if pid := ClaudePID(os.Getpid(), psParent); pid > 0 {
		return strconv.Itoa(pid)
	}
	return ""
}

// psParent reads the parent id and command of pid with ps (macOS and Linux).
func psParent(pid int) (int, string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, "", false
	}
	f := strings.Fields(strings.TrimSpace(string(out)))
	if len(f) < 2 {
		return 0, "", false
	}
	ppid, err := strconv.Atoi(f[0])
	if err != nil {
		return 0, "", false
	}
	return ppid, strings.Join(f[1:], " "), true
}

// Package reaper ends TEST processes left orphaned (adopted by init/launchd,
// PPID 1) for longer than a minimum age.
//
// A test run that is killed or panics can leave its workers behind (vitest and
// tinypool forks, jest workers, cargo test binaries, Playwright's headless
// Chromium, fake sidecars). They pile up silently and on 2026-11-09 pushed a
// 16 GB Mac into a memory-compression freeze. Only orphans that match a test
// pattern are touched: servers, apps and anything with a live parent never are.
package reaper

import (
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// pattern matches the full command line of a test process.
var pattern = regexp.MustCompile(`fake-[a-z0-9_-]+\.sh|/vitest/|vitest\.mjs|tinypool|jest-worker|/target/(debug|release)/deps/|ms-playwright/|chrome-headless-shell`)

// Proc is one row of `ps -axo pid=,ppid=,etime=,command=`.
type Proc struct {
	PID, PPID int
	Etime     string
	Cmd       string
}

// ParsePS parses `ps -axo pid=,ppid=,etime=,command=` output.
func ParsePS(out string) []Proc {
	var procs []Proc
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		procs = append(procs, Proc{PID: pid, PPID: ppid, Etime: f[2], Cmd: strings.Join(f[3:], " ")})
	}
	return procs
}

// ParseEtime converts ps's [[dd-]hh:]mm:ss to seconds.
func ParseEtime(t string) (int, error) {
	days := 0
	if d, rest, ok := strings.Cut(t, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil {
			return 0, fmt.Errorf("bad etime %q", t)
		}
		days, t = n, rest
	}
	parts := strings.Split(t, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("bad etime %q", t)
	}
	secs := 0
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, fmt.Errorf("bad etime %q", t)
		}
		secs = secs*60 + n
	}
	return days*86400 + secs, nil
}

// Select keeps the orphaned test processes at least minAge seconds old.
func Select(procs []Proc, minAge, self int) []Proc {
	var out []Proc
	for _, p := range procs {
		if p.PPID != 1 || p.PID == self || !pattern.MatchString(p.Cmd) {
			continue
		}
		if age, err := ParseEtime(p.Etime); err != nil || age < minAge {
			continue
		}
		out = append(out, p)
	}
	return out
}

// Options configure Run; the zero value of every func is replaced by the real one.
type Options struct {
	MinAge       int // seconds
	Dry          bool
	Self         int
	PS           func() (string, error)
	KillChildren func(pid int)
	Kill         func(pid int) bool
	Now          func() time.Time
}

// Run ends the orphans, printing one line per process. KILL because these
// loops often ignore TERM.
func Run(opt Options, out io.Writer) error {
	if opt.PS == nil {
		opt.PS = func() (string, error) {
			b, err := exec.Command("ps", "-axo", "pid=,ppid=,etime=,command=").Output()
			return string(b), err
		}
	}
	if opt.KillChildren == nil {
		opt.KillChildren = func(pid int) { _ = exec.Command("pkill", "-9", "-P", strconv.Itoa(pid)).Run() }
	}
	if opt.Kill == nil {
		opt.Kill = killPID
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}

	raw, err := opt.PS()
	if err != nil {
		return fmt.Errorf("ps: %w", err)
	}
	ended := 0
	for _, p := range Select(ParsePS(raw), opt.MinAge, opt.Self) {
		cmd := p.Cmd
		if len(cmd) > 160 {
			cmd = cmd[:160]
		}
		if opt.Dry {
			fmt.Fprintf(out, "would end %d (%s): %s\n", p.PID, p.Etime, cmd)
			continue
		}
		opt.KillChildren(p.PID) // its own children first (a fake's `sleep`), then itself
		if opt.Kill(p.PID) {
			ended++
			fmt.Fprintf(out, "%s ended %d (%s): %s\n", opt.Now().Format("2006-01-02 15:04:05"), p.PID, p.Etime, cmd)
		}
	}
	if !opt.Dry && ended > 0 {
		fmt.Fprintf(out, "%s ended %d orphaned test process(es)\n", opt.Now().Format("2006-01-02 15:04:05"), ended)
	}
	return nil
}

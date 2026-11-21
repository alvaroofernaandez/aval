// Package kiri makes Docker ready on Kiri, the only container runtime on this
// Mac. `aval kiri up` is idempotent and never deletes anything: it does not
// touch containers, images or volumes.
package kiri

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// BuildkitConfig bounds the `multiarch` builder's cache: BuildKit
// garbage-collects the least recently used layers past maxUsedSpace.
//
//go:embed buildkitd.toml
var BuildkitConfig string

const builder = "multiarch"

// Options are every side effect Up has, injectable for tests.
type Options struct {
	KiriSock   string // Kiri's docker socket
	SystemSock string // /var/run/docker.sock, which should point at KiriSock
	DockerHost string
	Timeout    time.Duration // how long to wait for Kiri to answer
	Interval   time.Duration
	GOOS       string
	UID        int
	Run        func(name string, args ...string) error           // output discarded
	Output     func(name string, args ...string) (string, error) // stdout
	Readlink   func(path string) (string, error)
	Sleep      func(time.Duration)
	Out, Err   io.Writer
}

// OSOptions reads the KIRI_UP_* overrides (kept from the shell version) and
// wires real processes.
func OSOptions() Options {
	home, _ := os.UserHomeDir()
	secs := func(env string, def int) time.Duration {
		if n, err := strconv.Atoi(os.Getenv(env)); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
		return time.Duration(def) * time.Second
	}
	pick := func(env, def string) string {
		if v := os.Getenv(env); v != "" {
			return v
		}
		return def
	}
	return Options{
		KiriSock:   pick("KIRI_UP_SOCK", home+"/.kiri/run/docker.sock"),
		SystemSock: pick("KIRI_UP_SYSTEM_SOCK", "/var/run/docker.sock"),
		DockerHost: os.Getenv("DOCKER_HOST"),
		Timeout:    secs("KIRI_UP_TIMEOUT", 60),
		Interval:   secs("KIRI_UP_INTERVAL", 1),
		GOOS:       runtime.GOOS,
		UID:        os.Getuid(),
		Run:        func(name string, args ...string) error { return exec.Command(name, args...).Run() },
		Output: func(name string, args ...string) (string, error) {
			b, err := exec.Command(name, args...).Output()
			return string(b), err
		},
		Readlink: os.Readlink,
		Sleep:    time.Sleep,
		Out:      os.Stdout,
		Err:      os.Stderr,
	}
}

// Up returns the exit code: 0 ready, 1 Docker could not be brought up.
func Up(o Options) int {
	say := func(f string, a ...any) { fmt.Fprintf(o.Out, "kiri up: "+f+"\n", a...) }
	warn := func(f string, a ...any) { fmt.Fprintf(o.Err, "kiri up: WARN "+f+"\n", a...) }
	fail := func(f string, a ...any) int { fmt.Fprintf(o.Err, "kiri up: WARN "+f+"\n", a...); return 1 }
	answers := func() bool { return o.Run("docker", "info") == nil }

	if o.DockerHost != "" {
		warn("DOCKER_HOST=%s overrides the kiri context; unset it (the context is the single source of truth).", o.DockerHost)
	}

	// 1. Docker answers.
	if !answers() {
		if o.GOOS != "darwin" {
			return fail("Docker does not answer and this is not macOS: start the Docker daemon.")
		}
		say("Docker does not answer; starting Kiri...")
		if o.Run("launchctl", "kickstart", fmt.Sprintf("gui/%d/dev.kiri.daemon", o.UID)) != nil {
			_ = o.Run("open", "-ga", "Kiri")
		}
		for waited := time.Duration(0); !answers(); waited += o.Interval {
			if waited >= o.Timeout {
				return fail("Kiri did not answer in %ds. Check: kiri diag logs -n 50 --errors", int(o.Timeout/time.Second))
			}
			o.Sleep(o.Interval)
		}
	}

	// 2. Context.
	if ctx, _ := o.Output("docker", "context", "show"); strings.TrimSpace(ctx) != "kiri" {
		say("switching docker context to kiri")
		if err := o.Run("docker", "context", "use", "kiri"); err != nil {
			return fail("cannot switch docker context to kiri: %v", err)
		}
	}

	// 3. Builder.
	if o.Run("docker", "buildx", "inspect", builder) != nil {
		say("creating the %s builder (bounded cache)", builder)
		cfg, err := os.CreateTemp("", "buildkitd-*.toml")
		if err != nil {
			return fail("cannot write the builder config: %v", err)
		}
		defer os.Remove(cfg.Name())
		_, werr := cfg.WriteString(BuildkitConfig)
		if cerr := cfg.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return fail("cannot write the builder config: %v", werr)
		}
		if err := o.Run("docker", "buildx", "create", "--name", builder, "--driver", "docker-container",
			"--driver-opt", "default-load=true", "--buildkitd-config", cfg.Name(), "--bootstrap", "kiri"); err != nil {
			return fail("cannot create the %s builder: %v", builder, err)
		}
	}

	// 4. System socket.
	if target, _ := o.Readlink(o.SystemSock); target != o.KiriSock {
		if target == "" {
			target = "missing"
		}
		warn("%s -> %s, not Kiri (%s). Tools that ignore the context will fail.", o.SystemSock, target, o.KiriSock)
		warn("If /Library/LaunchDaemons/com.docker.socket.plist exists, Docker Desktop's helper re-links it on every boot:")
		warn("remove com.docker.socket/com.docker.vmnetd (sudo) and run: sudo ln -sfn %s %s", o.KiriSock, o.SystemSock)
	}

	say("ready (context kiri, builder %s)", builder)
	return 0
}

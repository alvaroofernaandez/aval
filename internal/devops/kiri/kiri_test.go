package kiri

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// fake is a scripted docker/launchctl/open: it records every call.
type fake struct {
	calls       []string
	downTries   int // `docker info` calls that fail before it answers
	infoCalls   int
	context     string
	hasBuilder  bool
	cfgAtCreate string // contents of --buildkitd-config when buildx create ran
}

func (f *fake) run(name string, args ...string) error {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, strings.TrimSpace(call))
	switch {
	case name == "docker" && args[0] == "info":
		f.infoCalls++
		if f.infoCalls <= f.downTries {
			return errors.New("down")
		}
	case name == "docker" && args[0] == "buildx" && args[1] == "inspect":
		if !f.hasBuilder {
			return errors.New("no builder")
		}
	case name == "docker" && args[0] == "buildx" && args[1] == "create":
		for i, a := range args {
			if a == "--buildkitd-config" {
				b, _ := os.ReadFile(args[i+1])
				f.cfgAtCreate = string(b)
			}
		}
	}
	return nil
}

func (f *fake) output(name string, args ...string) (string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return f.context, nil
}

func (f *fake) joined() string { return strings.Join(f.calls, "\n") }

func opts(f *fake, out, errw *bytes.Buffer) Options {
	return Options{
		KiriSock:   "/kiri.sock",
		SystemSock: "/sys.sock",
		Timeout:    3 * time.Second,
		Interval:   time.Second,
		GOOS:       "darwin",
		UID:        501,
		Run:        f.run,
		Output:     f.output,
		Readlink:   func(string) (string, error) { return "/kiri.sock", nil },
		Sleep:      func(time.Duration) {},
		Out:        out,
		Err:        errw,
	}
}

func up(f *fake, mod func(*Options)) (code int, out, errw string) {
	var o, e bytes.Buffer
	op := opts(f, &o, &e)
	if mod != nil {
		mod(&op)
	}
	code = Up(op)
	return code, o.String(), e.String()
}

func TestUpHealthyDoesNothing(t *testing.T) {
	f := &fake{context: "kiri\n", hasBuilder: true}
	code, out, errw := up(f, nil)
	if code != 0 || errw != "" {
		t.Fatalf("code %d stderr %q", code, errw)
	}
	for _, bad := range []string{"launchctl", "buildx create", "context use"} {
		if strings.Contains(f.joined(), bad) {
			t.Errorf("healthy run called %q:\n%s", bad, f.joined())
		}
	}
	if !strings.Contains(out, "ready (context kiri, builder multiarch)") {
		t.Errorf("no ready line: %q", out)
	}
}

func TestUpStartsKiriWhenDown(t *testing.T) {
	f := &fake{context: "kiri", hasBuilder: true, downTries: 2}
	code, _, _ := up(f, nil)
	if code != 0 {
		t.Fatalf("did not recover, code %d", code)
	}
	if !strings.Contains(f.joined(), "launchctl kickstart gui/501/dev.kiri.daemon") {
		t.Errorf("no kickstart of dev.kiri.daemon:\n%s", f.joined())
	}
}

func TestUpNeverAnswers(t *testing.T) {
	f := &fake{context: "kiri", hasBuilder: true, downTries: 999}
	code, _, errw := up(f, nil)
	if code == 0 || !strings.Contains(errw, "kiri diag logs") || !strings.Contains(errw, "3s") {
		t.Errorf("code %d stderr %q", code, errw)
	}
}

func TestUpDownOffMac(t *testing.T) {
	f := &fake{context: "kiri", downTries: 999}
	code, _, errw := up(f, func(o *Options) { o.GOOS = "linux" })
	if code == 0 || !strings.Contains(errw, "not macOS") || strings.Contains(f.joined(), "launchctl") {
		t.Errorf("code %d stderr %q calls %s", code, errw, f.joined())
	}
}

func TestUpFixesContext(t *testing.T) {
	f := &fake{context: "desktop-linux", hasBuilder: true}
	if code, _, _ := up(f, nil); code != 0 || !strings.Contains(f.joined(), "docker context use kiri") {
		t.Errorf("code %d\n%s", code, f.joined())
	}
}

func TestUpCreatesBoundedBuilder(t *testing.T) {
	f := &fake{context: "kiri"}
	if code, _, _ := up(f, nil); code != 0 {
		t.Fatalf("code %d", code)
	}
	for _, want := range []string{"docker buildx create --name multiarch", "--driver docker-container", "default-load=true", "--buildkitd-config", "--bootstrap kiri"} {
		if !strings.Contains(f.joined(), want) {
			t.Errorf("create lacks %q:\n%s", want, f.joined())
		}
	}
	if !strings.Contains(f.cfgAtCreate, "maxUsedSpace") {
		t.Errorf("builder config does not bound the cache: %q", f.cfgAtCreate)
	}
}

func TestUpForeignSystemSocketWarns(t *testing.T) {
	f := &fake{context: "kiri", hasBuilder: true}
	code, _, errw := up(f, func(o *Options) {
		o.Readlink = func(string) (string, error) { return "", errors.New("missing") }
	})
	if code != 0 || !strings.Contains(errw, "WARN") || !strings.Contains(errw, "com.docker.socket") || !strings.Contains(errw, "missing") {
		t.Errorf("code %d stderr %q", code, errw)
	}
}

func TestUpDockerHostWarns(t *testing.T) {
	f := &fake{context: "kiri", hasBuilder: true}
	_, _, errw := up(f, func(o *Options) { o.DockerHost = "tcp://x:2375" })
	if !strings.Contains(errw, "DOCKER_HOST=tcp://x:2375 overrides") {
		t.Errorf("stderr %q", errw)
	}
}

func TestBuildkitConfigBoundsCache(t *testing.T) {
	if !strings.Contains(BuildkitConfig, "maxUsedSpace") {
		t.Error("embedded buildkitd config has no maxUsedSpace")
	}
}

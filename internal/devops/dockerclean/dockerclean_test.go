package dockerclean

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type fake struct {
	calls   []string
	context string
	dataImg bool
	failOn  string // a substring of a command that fails
}

func (f *fake) opts(out, errw *bytes.Buffer) Options {
	return Options{
		KiriData: "/home/u/Library/Application Support/Kiri",
		Out:      out,
		Err:      errw,
		Output: func(name string, args ...string) (string, error) {
			c := name + " " + strings.Join(args, " ")
			f.calls = append(f.calls, c)
			switch name {
			case "docker":
				return f.context + "\n", nil
			case "df":
				return "Filesystem Size Used Avail\n/dev/disk3 460Gi 12Gi 100Gi\n", nil
			case "du":
				return "2.9G\t" + args[len(args)-1] + "\n", nil
			}
			return "", nil
		},
		Run: func(w io.Writer, name string, args ...string) error {
			c := name + " " + strings.Join(args, " ")
			f.calls = append(f.calls, c)
			if f.failOn != "" && strings.Contains(c, f.failOn) {
				return errors.New("boom")
			}
			return nil
		},
		Exists: func(string) bool { return f.dataImg },
	}
}

func run(f *fake, mod func(*Options)) (int, string, string) {
	var o, e bytes.Buffer
	op := f.opts(&o, &e)
	if mod != nil {
		mod(&op)
	}
	return Cleanup(op), o.String(), e.String()
}

func TestSafePrunesOnly(t *testing.T) {
	f := &fake{context: "kiri", dataImg: true}
	code, out, _ := run(f, nil)
	calls := strings.Join(f.calls, "\n")
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	for _, want := range []string{
		"docker container prune -f",
		"docker image prune -f",
		"docker builder prune -af",
		"docker buildx prune --builder multiarch -af",
		"docker volume create kiri-fstrim-probe",
		"fstrim -v /v",
		"docker volume rm kiri-fstrim-probe",
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("missing %q in:\n%s", want, calls)
		}
	}
	for _, never := range []string{"system prune", "volume prune", "image prune -a", "mount /dev/vda", "orbctl"} {
		if strings.Contains(calls, never) {
			t.Errorf("ran forbidden %q:\n%s", never, calls)
		}
	}
	for _, c := range f.calls {
		if strings.Contains(c, "volume rm") && !strings.HasSuffix(c, "volume rm kiri-fstrim-probe") {
			t.Errorf("removed a volume that is not the probe: %s", c)
		}
	}
	for _, want := range []string{"== BEFORE ==", "== AFTER ==", "Application Support/Kiri", "data.img", "measured difference"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "auto-shrink") {
		t.Error("claims the disk auto-shrinks")
	}
}

func TestRefusesOtherContexts(t *testing.T) {
	for _, ctx := range []string{"orbstack", "colima-rosetta", "desktop-linux", ""} {
		f := &fake{context: ctx}
		code, _, errw := run(f, nil)
		if code == 0 || !strings.Contains(errw, "Refusing to prune") {
			t.Errorf("context %q: code %d stderr %q", ctx, code, errw)
		}
		for _, c := range f.calls {
			if strings.Contains(c, "prune") || strings.Contains(c, "volume") {
				t.Errorf("context %q ran %q", ctx, c)
			}
		}
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	f := &fake{context: "kiri"}
	code, out, _ := run(f, func(o *Options) { o.DryRun = true })
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "prune") || strings.Contains(c, "volume") || strings.Contains(c, "fstrim") {
			t.Errorf("dry run executed %q", c)
		}
	}
	if !strings.Contains(out, "would run: docker container prune -f") {
		t.Errorf("dry run does not list the commands:\n%s", out)
	}
}

func TestPruneFailureAborts(t *testing.T) {
	f := &fake{context: "kiri", failOn: "image prune"}
	code, _, _ := run(f, nil)
	if code == 0 {
		t.Error("a failed prune must fail the command")
	}
	if strings.Contains(strings.Join(f.calls, "\n"), "builder prune") {
		t.Error("kept going after a failed prune")
	}
}

func TestMissingBuilderAndFstrimAreNotFatal(t *testing.T) {
	f := &fake{context: "kiri", failOn: "buildx prune"}
	if code, out, _ := run(f, nil); code != 0 || !strings.Contains(out, "builder 'multiarch' not available") {
		t.Errorf("buildx: code %d out %s", code, out)
	}
	f = &fake{context: "kiri", failOn: "fstrim"}
	code, out, _ := run(f, nil)
	if code != 0 || !strings.Contains(out, "fstrim failed") || !strings.Contains(strings.Join(f.calls, "\n"), "volume rm kiri-fstrim-probe") {
		t.Errorf("fstrim: code %d out %s calls %v", code, out, f.calls)
	}
}

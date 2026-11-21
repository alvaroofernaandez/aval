package tmux

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func plain(s string) string { return regexp.MustCompile(`#\[[^\]]*\]`).ReplaceAllString(s, "") }

func linuxBar(t *testing.T, fixture string) (*harness, string) {
	t.Helper()
	h := newHarness()
	h.vars["STATUSBAR_OS"] = "Linux"
	h.vars["STATUSBAR_PROC_DIR"] = filepath.Join("testdata", fixture)
	h.vars["STATUSBAR_CACHE"] = filepath.Join(t.TempDir(), "cache")
	h.vars["STATUSBAR_CACHE_TTL"] = "0"
	assertEq(t, "rc", 0, h.run("statusbar"))
	return h, strings.TrimSuffix(h.out.String(), "\n")
}

func TestStatusbarLinux(t *testing.T) {
	t.Run("loadavg over cores, MemTotal-MemAvailable, no GPU", func(t *testing.T) {
		_, out := linuxBar(t, "linux-proc")
		want := "#[fg=colour235,bg=colour110,bold] CPU 50% #[default] " +
			"#[fg=colour235,bg=colour108,bold] RAM 7.8/15.6G #[default] " +
			"#[fg=colour235,bg=colour179,bold] 14:07 #[default] " +
			"#[fg=colour235,bg=colour174,bold] 5 Nov #[default]"
		assertEq(t, "row", want, out)
		assertEq(t, "no GPU label", false, strings.Contains(out, "GPU"))
		assertEq(t, "no GPU colour", false, strings.Contains(out, "colour140"))
	})
	t.Run("busy: CPU clamped to 100 and alert colour", func(t *testing.T) {
		_, out := linuxBar(t, "linux-proc-busy")
		assertEq(t, "cpu", true, strings.Contains(plain(out), "CPU 100%"))
		assertEq(t, "alert", true, strings.Contains(out, "bg=colour167"))
	})
	t.Run("missing /proc still renders without noise", func(t *testing.T) {
		h, out := linuxBar(t, "does-not-exist")
		assertEq(t, "cpu", true, strings.Contains(plain(out), "CPU 0%"))
		assertEq(t, "ram", true, strings.Contains(plain(out), "RAM 0.0/0.0G"))
		assertEq(t, "stderr", "", h.errb.String())
	})
	t.Run("single row, ASCII, within 60 columns, no dangling gap", func(t *testing.T) {
		h, out := linuxBar(t, "linux-proc")
		assertEq(t, "one line", 1, strings.Count(h.out.String(), "\n"))
		for _, r := range out {
			if r > 127 {
				t.Fatalf("non-ASCII rune %q", r)
			}
		}
		if n := len(plain(out)); n > 60 {
			t.Errorf("%d columns", n)
		}
		assertEq(t, "ends on the last reset", true, strings.HasSuffix(out, "#[default]"))
	})
}

const (
	darwinSysctl = "{ 2.00 1.50 1.00 }\n8\n17179869184\n"
	darwinVMStat = "Mach Virtual Memory Statistics: (page size of 16384 bytes)\n" +
		"Pages free:                               1000.\n" +
		"Pages active:                           100000.\n" +
		"Pages wired down:                        50000.\n" +
		"Pages occupied by compressor:            25000.\n"
	darwinIOReg = `    | |   "PerformanceStatistics" = {"Device Utilization %"=23,"Renderer Utilization %"=10}` + "\n"
)

func darwinBar(t *testing.T, mutate func(*harness)) (*harness, string) {
	t.Helper()
	h := newHarness()
	h.vars["STATUSBAR_CACHE"] = filepath.Join(t.TempDir(), "cache")
	h.vars["STATUSBAR_CACHE_TTL"] = "0"
	h.outputs["/usr/sbin/sysctl -n vm.loadavg hw.logicalcpu hw.memsize"] = darwinSysctl
	h.outputs["/usr/bin/vm_stat"] = darwinVMStat
	h.outputs["/usr/sbin/ioreg -r -d 1 -w 0 -c AGXAccelerator"] = darwinIOReg
	if mutate != nil {
		mutate(h)
	}
	assertEq(t, "rc", 0, h.run("statusbar"))
	return h, strings.TrimSuffix(h.out.String(), "\n")
}

func TestStatusbarDarwin(t *testing.T) {
	t.Run("full row", func(t *testing.T) {
		_, out := darwinBar(t, nil)
		want := "#[fg=colour235,bg=colour110,bold] CPU 25% #[default] " +
			"#[fg=colour235,bg=colour140,bold] GPU 23% #[default] " +
			"#[fg=colour235,bg=colour108,bold] RAM 2.7/16.0G #[default] " +
			"#[fg=colour235,bg=colour179,bold] 14:07 #[default] " +
			"#[fg=colour235,bg=colour174,bold] 5 Nov #[default]"
		assertEq(t, "row", want, out)
	})
	t.Run("GPU over the threshold turns alert red", func(t *testing.T) {
		_, out := darwinBar(t, func(h *harness) {
			h.outputs["/usr/sbin/ioreg -r -d 1 -w 0 -c AGXAccelerator"] = `"Device Utilization %"=90`
		})
		assertEq(t, "gpu alert", true, strings.Contains(out, "bg=colour167,bold] GPU 90%"))
	})
	t.Run("no AGX accelerator reports GPU 0", func(t *testing.T) {
		_, out := darwinBar(t, func(h *harness) {
			h.outputs["/usr/sbin/ioreg -r -d 1 -w 0 -c AGXAccelerator"] = ""
		})
		assertEq(t, "gpu", true, strings.Contains(plain(out), "GPU 0%"))
	})
	t.Run("failing tools still render a row", func(t *testing.T) {
		_, out := darwinBar(t, func(h *harness) {
			h.outputs = map[string]string{}
		})
		assertEq(t, "cpu", true, strings.Contains(plain(out), "CPU 0%"))
		assertEq(t, "ram", true, strings.Contains(plain(out), "RAM "))
	})
	t.Run("RAM never exceeds total", func(t *testing.T) {
		_, out := darwinBar(t, func(h *harness) {
			h.outputs["/usr/bin/vm_stat"] = strings.Replace(darwinVMStat, "100000.", "9000000.", 1)
		})
		assertEq(t, "capped", true, strings.Contains(plain(out), "RAM 16.0/16.0G"))
	})
	t.Run("fits the width budget", func(t *testing.T) {
		_, out := darwinBar(t, nil)
		if n := len(plain(out)); n > 60 {
			t.Errorf("%d columns", n)
		}
	})
	t.Run("months and decimals do not depend on locale", func(t *testing.T) {
		_, out := darwinBar(t, func(h *harness) {
			h.vars["LANG"] = "es_ES.UTF-8"
			h.vars["LC_ALL"] = "es_ES.UTF-8"
			h.now = time.Date(2026, 8, 31, 9, 5, 0, 0, time.UTC)
		})
		assertEq(t, "date", true, strings.HasSuffix(plain(out), "09:05   31 Aug "))
		assertEq(t, "decimal dot", true, strings.Contains(out, "RAM 2.7/16.0G"))
	})
}

func TestStatusbarCache(t *testing.T) {
	cachePath := func(t *testing.T) string { return filepath.Join(t.TempDir(), "cache") }

	t.Run("the measured half is written, the clock is not", func(t *testing.T) {
		p := cachePath(t)
		h, _ := darwinBar(t, func(h *harness) { h.vars["STATUSBAR_CACHE"] = p })
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		want := "#[fg=colour235,bg=colour110,bold] CPU 25% #[default] " +
			"#[fg=colour235,bg=colour140,bold] GPU 23% #[default] " +
			"#[fg=colour235,bg=colour108,bold] RAM 2.7/16.0G #[default] \n"
		assertEq(t, "cache", want, string(data))
		_ = h
	})
	t.Run("a fresh cache skips every measurement and keeps the clock live", func(t *testing.T) {
		p := cachePath(t)
		cached := "#[fg=colour235,bg=colour110,bold] CPU 1% #[default] "
		if err := os.WriteFile(p, []byte(cached+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		h := newHarness()
		h.vars["STATUSBAR_CACHE"] = p
		h.now = time.Now() // cache mtime is "now"
		assertEq(t, "rc", 0, h.run("statusbar"))
		assertEq(t, "nothing measured", 0, len(h.outCalls))
		want := cached + "#[fg=colour235,bg=colour179,bold] " + h.now.Format("15:04") +
			" #[default] #[fg=colour235,bg=colour174,bold] " + h.now.Format("2 Jan") + " #[default]\n"
		assertEq(t, "row", want, h.out.String())
	})
	t.Run("an expired cache is recomputed", func(t *testing.T) {
		p := cachePath(t)
		if err := os.WriteFile(p, []byte("CACHE ANTIGUO\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-time.Hour)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
		h, out := darwinBar(t, func(h *harness) {
			h.vars["STATUSBAR_CACHE"] = p
			h.vars["STATUSBAR_CACHE_TTL"] = "4"
			h.now = time.Now()
		})
		assertEq(t, "stale gone", false, strings.Contains(out, "CACHE ANTIGUO"))
		assertEq(t, "measured", true, len(h.outCalls) > 0)
	})
	t.Run("an empty cache is ignored", func(t *testing.T) {
		p := cachePath(t)
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		h, _ := darwinBar(t, func(h *harness) {
			h.vars["STATUSBAR_CACHE"] = p
			h.vars["STATUSBAR_CACHE_TTL"] = "4"
			h.now = time.Now()
		})
		assertEq(t, "measured", true, len(h.outCalls) > 0)
	})
	t.Run("default cache path is per-uid under TMPDIR", func(t *testing.T) {
		h := newHarness()
		dir := t.TempDir()
		h.vars["TMPDIR"] = dir
		h.vars["STATUSBAR_OS"] = "Linux"
		h.vars["STATUSBAR_PROC_DIR"] = filepath.Join("testdata", "linux-proc")
		h.run("statusbar")
		if _, err := os.Stat(filepath.Join(dir, "tmux-statusbar-501.cache")); err != nil {
			t.Errorf("cache not written to the default path: %v", err)
		}
	})
}

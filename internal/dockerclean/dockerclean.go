// Package dockerclean reclaims Docker space on Kiri safely: stopped
// containers, dangling images and build cache (daemon and the `multiarch`
// builder). It never touches volumes (except its own throwaway probe), tagged
// images or running containers, and refuses any context other than `kiri`.
package dockerclean

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const probeVolume = "kiri-fstrim-probe"

// Options are every side effect Cleanup has, injectable for tests.
type Options struct {
	KiriData string // ~/Library/Application Support/Kiri
	DryRun   bool   // print the mutating commands instead of running them
	Out, Err io.Writer
	Output   func(name string, args ...string) (string, error)    // stdout of a read-only tool
	Run      func(w io.Writer, name string, args ...string) error // streams the tool's output to w
	Exists   func(path string) bool
}

// OSOptions wires real processes.
func OSOptions(dryRun bool) Options {
	home, _ := os.UserHomeDir()
	return Options{
		KiriData: filepath.Join(home, "Library", "Application Support", "Kiri"),
		DryRun:   dryRun,
		Out:      os.Stdout,
		Err:      os.Stderr,
		Output: func(name string, args ...string) (string, error) {
			b, err := exec.Command(name, args...).Output()
			return string(b), err
		},
		Run: func(w io.Writer, name string, args ...string) error {
			c := exec.Command(name, args...)
			c.Stdout, c.Stderr = w, w
			return c.Run()
		},
		Exists: func(p string) bool { _, err := os.Stat(p); return err == nil },
	}
}

// Cleanup returns the exit code: 0 done, 1 refused or a prune failed.
func Cleanup(o Options) int {
	ctx, _ := o.Output("docker", "context", "show")
	if ctx = strings.TrimSpace(ctx); ctx != "kiri" {
		fmt.Fprintf(o.Err, "Refusing to prune: active docker context is '%s', expected 'kiri'.\n", ctx)
		fmt.Fprintln(o.Err, "Run: docker context use kiri (or load the kiri-health skill)")
		return 1
	}

	report := func(title string) {
		fmt.Fprintf(o.Out, "== %s ==\n", title)
		if df, _ := o.Output("df", "-h", "/"); df != "" {
			ls := strings.Split(strings.TrimRight(df, "\n"), "\n")
			fmt.Fprintln(o.Out, ls[len(ls)-1])
		}
		if du, err := o.Output("du", "-sh", o.KiriData); err == nil && du != "" {
			fmt.Fprint(o.Out, du)
		} else {
			fmt.Fprintf(o.Out, "Kiri data dir not found: %s\n", o.KiriData)
		}
		img := filepath.Join(o.KiriData, "vm", "data.img")
		if o.Exists(img) {
			if du, _ := o.Output("du", "-h", img); du != "" {
				fmt.Fprintf(o.Out, "%s real  vm/data.img\n", strings.Fields(du)[0])
			}
		} else {
			fmt.Fprintln(o.Out, "vm/data.img not found")
		}
		_ = o.Run(o.Out, "docker", "system", "df")
		fmt.Fprintln(o.Out)
	}

	// run executes one mutating docker command, or only announces it.
	run := func(args ...string) error {
		if o.DryRun {
			fmt.Fprintf(o.Out, "would run: docker %s\n", strings.Join(args, " "))
			return nil
		}
		return o.Run(o.Out, "docker", args...)
	}

	report("BEFORE")

	for _, args := range [][]string{
		{"container", "prune", "-f"},
		{"image", "prune", "-f"},
		{"builder", "prune", "-af"},
	} {
		if err := run(args...); err != nil {
			fmt.Fprintf(o.Err, "docker %s failed: %v\n", strings.Join(args, " "), err)
			return 1
		}
	}
	if err := run("buildx", "prune", "--builder", "multiarch", "-af"); err != nil {
		fmt.Fprintln(o.Out, "builder 'multiarch' not available, skipped (see kiri-health).")
	}
	trimVMDisk(o, run)
	fmt.Fprintln(o.Out)

	report("AFTER")
	fmt.Fprintln(o.Out, "Compare the Kiri data dir size above: that measured difference is what was reclaimed on disk.")
	return 0
}

// trimVMDisk: Kiri's vm/data.img grows on write and never hands freed blocks
// back to macOS by itself; fstrim on the VM's ext4 does. The filesystem is
// reached through a throwaway volume (it lives on /dev/vda): mounting the block
// device a second time could corrupt it, so that is never done. Verified
// 2026-11-19: 49.4 -> 25.2 GB.
func trimVMDisk(o Options, run func(args ...string) error) {
	_ = run("volume", "create", probeVolume)
	if err := run("run", "--rm", "--privileged", "-v", probeVolume+":/v", "alpine:3", "sh", "-c", "fstrim -v /v"); err != nil {
		fmt.Fprintln(o.Out, "fstrim failed; data.img keeps its freed blocks until the next run.")
	}
	_ = run("volume", "rm", probeVolume)
}

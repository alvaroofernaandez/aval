package tmux

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// DESIGN.md §2 — Kanagawa roles, one colour per metric.
const (
	ink      = 235 // sumiInk   #1F1F28 — text on every segment
	cCPU     = 110 // crystalBlue
	cGPU     = 140 // oniViolet
	cRAM     = 108 // springGreen
	cTime    = 179 // carpYellow
	cDate    = 174 // sakuraPink
	cAlert   = 167 // autumnRed — value over threshold, never chrome
	alertPct = 85
)

var gpuRe = regexp.MustCompile(`"Device Utilization %"=([0-9]+)`)

// statusbar renders tmux status-right: CPU, GPU, RAM, clock and date as
// coloured segments. See .agents/DESIGN.md for the colour roles.
//
// It runs on every status refresh, so it is built to be cheap: the macOS
// readings are three tools started together, and the measured half is cached
// briefly and shared by every session. Deliberately absent:
//   - `top -l 1`     208ms, samples the whole process table
//   - `powermetrics` requires superuser, unusable from a status hook
//
// CPU comes from the 1-minute load average rather than sampled utilisation:
// sampling needs two readings separated by a sleep.
//
// Presentation is pinned: Go's formatting is locale-free, so the row is
// identical wherever tmux was started from (no comma decimals, no localised
// month names of varying width).
func statusbar(env Env) int {
	now := env.Now()
	clock := now.Format("15:04")
	today := now.Format("2 Jan")

	// CPU, GPU and RAM are system-wide: every tmux session renders the SAME
	// numbers, so the measured part is cached briefly. The clock is NOT cached —
	// it is free to produce and must not lag behind.
	cache := env.Getenv("STATUSBAR_CACHE")
	if cache == "" {
		tmp := env.Getenv("TMPDIR")
		if tmp == "" {
			tmp = "/tmp"
		}
		cache = filepath.Join(tmp, fmt.Sprintf("tmux-statusbar-%d.cache", env.UID))
	}
	ttl := int64(4)
	if v := env.Getenv("STATUSBAR_CACHE_TTL"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			ttl = n
		}
	}

	if cached, ok := readCache(cache, now.Unix(), ttl); ok {
		fmt.Fprintf(env.Stdout, "%s%s%s\n", cached, seg(cTime, clock, 0), strings.TrimSuffix(seg(cDate, today, 0), " "))
		return 0
	}

	var m metrics
	if strings.HasPrefix(strings.ToLower(orDefault(env.Getenv("STATUSBAR_OS"), env.GOOS)), "linux") {
		m = readLinux(orDefault(env.Getenv("STATUSBAR_PROC_DIR"), "/proc"))
	} else {
		m = readDarwin(env)
	}

	row := m.render()
	writeCache(cache, row)
	fmt.Fprintf(env.Stdout, "%s%s%s\n", row, seg(cTime, clock, 0), strings.TrimSuffix(seg(cDate, today, 0), " "))
	return 0
}

type metrics struct {
	load  float64
	ncpu  float64
	total float64 // bytes
	used  float64 // bytes, -1: unknown
	gpu   int
	// showGPU is false on Linux: no unprivileged, vendor-neutral utilisation
	// counter exists, and printing a zero would be a lie (DESIGN.md §7).
	showGPU bool
}

func (m metrics) render() string {
	ncpu := m.ncpu
	if ncpu <= 0 {
		ncpu = 1
	}
	cpu := m.load / ncpu * 100
	cpu = min(max(cpu, 0), 100)

	const gb = 1073741824.0
	used := m.used / gb
	tot := m.total / gb
	if tot <= 0 {
		tot = used
	}
	if used > tot {
		used = tot
	}
	ramPct := 0.0
	if tot > 0 {
		ramPct = used / tot * 100
	}

	row := seg(cCPU, fmt.Sprintf("CPU %d%%", int(cpu+0.5)), cpu)
	if m.showGPU {
		row += seg(cGPU, fmt.Sprintf("GPU %d%%", m.gpu), float64(m.gpu))
	}
	return row + seg(cRAM, fmt.Sprintf("RAM %.1f/%.1fG", used, tot), ramPct)
}

// seg is one segment — DESIGN.md §4: one padding space inside, one plain space
// between segments. Colour never carries meaning alone: the label and value
// always accompany it.
func seg(colour int, text string, pct float64) string {
	bg := colour
	if pct >= alertPct {
		bg = cAlert
	}
	return fmt.Sprintf("#[fg=colour%d,bg=colour%d,bold] %s #[default] ", ink, bg, text)
}

func readCache(path string, nowUnix, ttl int64) (string, bool) {
	st, err := os.Stat(path)
	if err != nil || st.Size() == 0 {
		return "", false
	}
	if age := nowUnix - st.ModTime().Unix(); age < 0 || age >= ttl {
		return "", false
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	line, _ := bufio.NewReader(f).ReadString('\n')
	line = strings.TrimSuffix(line, "\n")
	return line, line != ""
}

// writeCache is atomic: several bars refresh at once and one must never read
// another's half-written line.
func writeCache(path, row string) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".statusbar-*")
	if err != nil {
		return
	}
	_, werr := tmp.WriteString(row + "\n")
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		os.Remove(tmp.Name())
	}
}

// readLinux reads /proc directly: nothing is forked.
func readLinux(proc string) metrics {
	m := metrics{ncpu: 1, used: 0}

	if b, err := os.ReadFile(filepath.Join(proc, "loadavg")); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 && !strings.ContainsAny(f[0], "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-+") {
			m.load, _ = strconv.ParseFloat(f[0], 64)
		}
	}
	if b, err := os.ReadFile(filepath.Join(proc, "cpuinfo")); err == nil {
		n := 0
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "processor") {
				n++
			}
		}
		if n > 0 {
			m.ncpu = float64(n)
		}
	}
	var memTotal, memAvail float64
	if f, err := os.Open(filepath.Join(proc, "meminfo")); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fs := strings.Fields(sc.Text())
			if len(fs) < 2 {
				continue
			}
			v, _ := strconv.ParseFloat(fs[1], 64)
			switch fs[0] {
			case "MemTotal:":
				memTotal = v
			case "MemAvailable:":
				memAvail = v
			}
			if fs[0] == "MemAvailable:" {
				break
			}
		}
	}
	m.total = memTotal * 1024
	m.used = (memTotal - memAvail) * 1024
	if m.used < 0 {
		m.used = 0
	}
	return m
}

// readDarwin asks sysctl, vm_stat and ioreg at once, by absolute path: a status
// hook runs with launchd's PATH, and the three are independent.
func readDarwin(env Env) metrics {
	var sysOut, vmOut, ioOut string
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		sysOut, _ = env.Output("/usr/sbin/sysctl", "-n", "vm.loadavg", "hw.logicalcpu", "hw.memsize")
	}()
	go func() { defer wg.Done(); vmOut, _ = env.Output("/usr/bin/vm_stat") }()
	go func() {
		defer wg.Done()
		// "Device Utilization %" is exposed unprivileged; absent on Intel Macs,
		// in which case the GPU segment reports 0 rather than breaking the row.
		ioOut, _ = env.Output("/usr/sbin/ioreg", "-r", "-d", "1", "-w", "0", "-c", "AGXAccelerator")
	}()
	wg.Wait()

	m := metrics{ncpu: 1, used: 0, showGPU: true}

	var numeric []float64
	for _, line := range strings.Split(sysOut, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "{"):
			if f := strings.Fields(strings.NewReplacer("{", " ", "}", " ").Replace(line)); len(f) > 0 {
				m.load, _ = strconv.ParseFloat(f[0], 64)
			}
		case line != "" && len(strings.Fields(line)) == 1:
			v, _ := strconv.ParseFloat(line, 64)
			numeric = append(numeric, v)
		}
	}
	if len(numeric) > 0 {
		m.ncpu = numeric[0]
	}
	if len(numeric) > 1 {
		m.total = numeric[1]
	}

	page := 4096.0
	var active, wired, comp float64
	for _, line := range strings.Split(vmOut, "\n") {
		fs := strings.Fields(line)
		if len(fs) == 0 {
			continue
		}
		last := func() float64 {
			v, _ := strconv.ParseFloat(strings.ReplaceAll(fs[len(fs)-1], ".", ""), 64)
			return v
		}
		switch {
		case strings.Contains(line, "page size of"):
			for i, f := range fs {
				if f == "of" && i+1 < len(fs) {
					page, _ = strconv.ParseFloat(fs[i+1], 64)
					break
				}
			}
		case strings.HasPrefix(line, "Pages active"):
			active = last()
		case strings.HasPrefix(line, "Pages wired down"):
			wired = last()
		case strings.HasPrefix(line, "Pages occupied by compressor"):
			comp = last()
		}
	}
	m.used = (active + wired + comp) * page

	if g := gpuRe.FindStringSubmatch(ioOut); g != nil {
		m.gpu, _ = strconv.Atoi(g[1])
	}
	return m
}

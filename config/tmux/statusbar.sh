#!/usr/bin/env bash
# tmux status-right: CPU, GPU, RAM, clock and date as coloured segments.
#
# See .agents/DESIGN.md for the colour roles and the rules this follows.
#
# Runs on every status refresh, so it is built to be cheap: three sysctl keys in
# one call, one vm_stat, one ioreg, one date, one awk. Deliberately absent:
#   - `top -l 1`      208ms, samples the whole process table
#   - a python start   29ms, just to divide some numbers
#   - `powermetrics`  requires superuser, unusable from a status hook
#
# CPU comes from the 1-minute load average rather than sampled utilisation:
# sampling needs two readings separated by a sleep, which is what makes the
# usual implementations cost seconds per refresh.
set -uo pipefail

# Pinned so the row is identical wherever tmux was started from. Under a Spanish
# locale awk formats decimals with a comma (RAM 8,5/16,0G) and date renders
# months in Spanish with a trailing dot, whose width varies (ago. / sept.) and
# breaks the fixed column budget. Presentation here must not depend on the
# environment that happened to launch the server.
export LC_ALL=C

# DESIGN.md §2 — Kanagawa roles, one colour per metric.
INK=235          # sumiInk   #1F1F28  — text on every segment
C_CPU=110        # crystalBlue
C_GPU=140        # oniViolet
C_RAM=108        # springGreen
C_TIME=179       # carpYellow
C_DATE=174       # sakuraPink
C_ALERT=167      # autumnRed — value over threshold, never chrome

ALERT_PCT=85

# CPU, GPU and RAM are system-wide: every tmux session renders the SAME numbers.
# With several sessions open, computing them once per bar is pure waste, so the
# measured part is cached briefly and shared. The clock is NOT cached — it is
# free to produce and must not lag behind.
CACHE="${STATUSBAR_CACHE:-${TMPDIR:-/tmp}/tmux-statusbar-$(id -u).cache}"
CACHE_TTL="${STATUSBAR_CACHE_TTL:-4}"

# One `date` call yields all three values it needs. bash 3.2 is what runs under
# tmux's minimal PATH, so printf's %()T and EPOCHSECONDS are not available, and
# each avoided process is ~3ms on a path that runs once per bar per refresh.
now="$(date '+%s|%H:%M|%-d %b')"
epoch="${now%%|*}"
rest="${now#*|}"
clock="${rest%%|*}"
today="${rest#*|}"

# Platform is read once from $OSTYPE (a bash variable: no process, so the macOS
# path pays nothing). STATUSBAR_OS and STATUSBAR_PROC_DIR are test seams that
# let the Linux branch run against fixture files on any host.
case "${STATUSBAR_OS:-$OSTYPE}" in
  [Ll]inux*) PLATFORM=linux ;;
  *)         PLATFORM=darwin ;;
esac
PROC_DIR="${STATUSBAR_PROC_DIR:-/proc}"

# BSD stat takes -f, GNU stat takes -c (and -f means "filesystem" there, with
# exit 0 and unrelated output), so each platform is asked in its own dialect.
if [ "$PLATFORM" = linux ]; then
  mtime="$(stat -c %Y "$CACHE" 2>/dev/null || echo 0)"
else
  mtime="$(stat -f %m "$CACHE" 2>/dev/null || echo 0)"
fi
case "$mtime" in ''|*[!0-9]*) mtime=0 ;; esac
age=$(( epoch - mtime ))

if [ -s "$CACHE" ] && [ "$age" -ge 0 ] && [ "$age" -lt "$CACHE_TTL" ]; then
  # `read` is a builtin: no `cat` process. The cached half already carries its
  # own colour markup; only clock and date are appended fresh, so the clock
  # never lags behind the cache.
  IFS= read -r cached < "$CACHE" || cached=""
  if [ -n "$cached" ]; then
    printf '%s#[fg=colour%d,bg=colour%d,bold] %s #[default] #[fg=colour%d,bg=colour%d,bold] %s #[default]\n' \
      "$cached" "$INK" "$C_TIME" "$clock" "$INK" "$C_DATE" "$today"
    exit 0
  fi
fi

used_override=-1   # >= 0: RAM used in bytes, already measured (Linux)
show_gpu=1

if [ "$PLATFORM" = linux ]; then
  # /proc is read with shell builtins only, so this branch forks nothing.
  # GPU is omitted, not faked: Linux has no unprivileged, vendor-neutral
  # utilisation counter (nvidia-smi, rocm-smi and intel_gpu_top are each a
  # separate tool, most cost tens of ms and some need privileges). Printing
  # a zero would be a lie, so the segment is dropped (DESIGN.md §7).
  show_gpu=0
  load1=0; ncpu=0; mem_total=0; mem_avail=0
  [ -r "$PROC_DIR/loadavg" ] && read -r load1 _ < "$PROC_DIR/loadavg"
  if [ -r "$PROC_DIR/cpuinfo" ]; then
    while IFS= read -r line; do
      case "$line" in processor*) ncpu=$((ncpu + 1)) ;; esac
    done < "$PROC_DIR/cpuinfo"
  fi
  if [ -r "$PROC_DIR/meminfo" ]; then
    while read -r key val _; do
      case "$key" in
        MemTotal:)     mem_total=$val ;;
        MemAvailable:) mem_avail=$val; break ;;
      esac
    done < "$PROC_DIR/meminfo"
  fi
  [ "$ncpu" -gt 0 ] || ncpu=1
  case "$load1" in ''|*[!0-9.]*) load1=0 ;; esac
  case "$mem_total" in ''|*[!0-9]*) mem_total=0 ;; esac
  case "$mem_avail" in ''|*[!0-9]*) mem_avail=0 ;; esac
  sysctl_out="{ $load1 0 0 }
$ncpu
$((mem_total * 1024))"
  vm_out=""
  used_override=$(( (mem_total - mem_avail) * 1024 ))
  gpu_util=0
else
  sysctl_out="$(sysctl -n vm.loadavg hw.logicalcpu hw.memsize 2>/dev/null)"
  vm_out="$(vm_stat 2>/dev/null)"
  # "Device Utilization %" is exposed unprivileged; absent on Intel Macs, in which
  # case the GPU segment reports 0 rather than breaking the row.
  gpu_util="$(ioreg -r -d 1 -w 0 -c AGXAccelerator 2>/dev/null \
    | rg -o '"Device Utilization %"=[0-9]+' | head -1 | rg -o '[0-9]+$')"
fi

printf '%s\n---\n%s\n' "$sysctl_out" "$vm_out" | awk \
  -v ink="$INK" -v c_cpu="$C_CPU" -v c_gpu="$C_GPU" -v c_ram="$C_RAM" \
  -v c_time="$C_TIME" -v c_date="$C_DATE" -v c_alert="$C_ALERT" \
  -v alert="$ALERT_PCT" -v cache_file="$CACHE" -v gpu="${gpu_util:-0}" -v show_gpu="$show_gpu" -v used_override="$used_override" -v clock="$clock" -v today="$today" '
  BEGIN { section = 0; page = 4096; load = 0; ncpu = 1; total = 0 }

  /^---$/ { section = 1; next }

  section == 0 && /^\{/ { gsub(/[{}]/, ""); load = $1 + 0; next }
  section == 0 && NF == 1 && ncpu_set == 1 && total == 0 { total = $1 + 0; next }
  section == 0 && NF == 1 && ncpu_set != 1 { ncpu = $1 + 0; ncpu_set = 1; next }

  section == 1 && /page size of/ {
    for (i = 1; i <= NF; i++) if ($i == "of") { page = $(i + 1) + 0; break }
    next
  }
  section == 1 && /^Pages active/                 { gsub(/\./, ""); active = $NF + 0 }
  section == 1 && /^Pages wired down/             { gsub(/\./, ""); wired  = $NF + 0 }
  section == 1 && /^Pages occupied by compressor/ { gsub(/\./, ""); comp   = $NF + 0 }

  END {
    if (ncpu <= 0) ncpu = 1
    cpu = load / ncpu * 100
    if (cpu > 100) cpu = 100
    if (cpu < 0) cpu = 0

    gb = 1073741824
    used = (active + wired + comp) * page / gb
    if (used_override >= 0) used = used_override / gb
    tot  = total / gb
    if (tot <= 0) tot = used
    if (used > tot) used = tot
    ram_pct = tot > 0 ? used / tot * 100 : 0

    metrics = seg(c_cpu, sprintf("CPU %d%%", cpu + 0.5), cpu) \
              (show_gpu ? seg(c_gpu, sprintf("GPU %d%%", gpu), gpu) : "") \
              seg(c_ram, sprintf("RAM %.1f/%.1fG", used, tot), ram_pct)
    print metrics > cache_file
    close(cache_file)

    row = metrics seg(c_time, clock, 0) seg(c_date, today, 0)
    sub(/ $/, "", row)   # the row ends at the last segment, no trailing gap
    printf "%s\n", row
  }

  # DESIGN.md §4 — one padding space inside, one plain space between segments.
  # Colour never carries meaning alone: the label and value always accompany it.
  function seg(colour, text, pct,   bg) {
    bg = (pct >= alert) ? c_alert : colour
    return sprintf("#[fg=colour%d,bg=colour%d,bold] %s #[default] ", ink, bg, text)
  }
'

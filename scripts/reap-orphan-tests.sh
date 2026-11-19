#!/usr/bin/env bash
# Ends TEST processes left orphaned (adopted by launchd, PPID 1) for longer than --min-age seconds (default 600).
#
# Why: a test run that is killed or panics can leave its workers behind (vitest/tinypool forks, jest workers, cargo test
# binaries, Playwright's headless Chromium, the fake sidecars of haga-harness' Rust tests). They pile up silently and on
# 2026-11-09 they pushed a 16 GB Mac into memory-compression freeze. Only orphans that match a test pattern are touched:
# servers, apps and anything with a live parent are never ended. Run every 5 minutes by
# ~/Library/LaunchAgents/dev.reap-orphan-tests.plist. Log: ~/Library/Logs/reap-orphan-tests.log.
set -u

MIN_AGE=600
DRY=0
while [ $# -gt 0 ]; do
  case "$1" in
    --min-age) MIN_AGE="$2"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    *) echo "usage: $0 [--min-age SECONDS] [--dry-run]" >&2; exit 2 ;;
  esac
done

# Test-process signatures (extended regex, matched against the full command line).
PATTERN='fake-[a-z0-9_-]+\.sh|/vitest/|vitest\.mjs|tinypool|jest-worker|/target/(debug|release)/deps/|ms-playwright/|chrome-headless-shell'

# [[dd-]hh:]mm:ss -> seconds
to_seconds() {
  local t="$1" d=0 h=0 m=0 s=0
  case "$t" in *-*) d="${t%%-*}"; t="${t#*-}" ;; esac
  IFS=: read -r a b c <<<"$t"
  if [ -n "${c:-}" ]; then h=$a; m=$b; s=$c; else m=$a; s=$b; fi
  echo $(( 10#$d * 86400 + 10#$h * 3600 + 10#$m * 60 + 10#$s ))
}

ended=0
while read -r pid ppid etime cmd; do
  [ "$ppid" = "1" ] || continue
  [ "$pid" = "$$" ] && continue
  [[ $cmd =~ $PATTERN ]] || continue
  [ "$(to_seconds "$etime")" -ge "$MIN_AGE" ] || continue
  if [ "$DRY" = 1 ]; then
    echo "would end $pid ($etime): ${cmd:0:160}"
    continue
  fi
  # Its own children first (a fake's `sleep`), then itself; KILL because these loops often ignore TERM.
  pkill -9 -P "$pid" 2>/dev/null
  kill -9 "$pid" 2>/dev/null && { ended=$((ended + 1)); echo "$(date '+%F %T') ended $pid ($etime): ${cmd:0:160}"; }
done < <(ps -axo pid=,ppid=,etime=,command=)

[ "$DRY" = 1 ] || [ "$ended" = 0 ] || echo "$(date '+%F %T') ended $ended orphaned test process(es)"
exit 0

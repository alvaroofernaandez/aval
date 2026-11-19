#!/usr/bin/env bash
# Test for scripts/reap-orphan-tests.sh: only ORPHANED (parent = launchd) TEST processes past the age limit are ended.
set -u
REAPER="$(cd "$(dirname "$0")/.." && pwd)/scripts/reap-orphan-tests.sh"
T="$(mktemp -d)"; trap 'pkill -9 -f "$T/" 2>/dev/null; rm -rf "$T"' EXIT
fail() { echo "FAIL: $*"; exit 1; }
printf '#!/bin/sh\ntrap "" TERM\nwhile :; do sleep 1; done\n' > "$T/fake-sidecar.sh"
printf '#!/bin/sh\nwhile :; do sleep 1; done\n' > "$T/my-server.sh"
chmod +x "$T"/*.sh
# orphans: started from a subshell that exits, so launchd (1) adopts them
( "$T/fake-sidecar.sh" & ) ; ( "$T/my-server.sh" & )
# a test process that still has its parent
"$T/fake-sidecar.sh" & KEEP=$!
sleep 1
orphan() { ps -axo pid=,ppid=,command= | awk -v p="$T/$1" '$2==1 && index($0,p) {print $1}'; }
[ -n "$(orphan fake-sidecar.sh)" ] || fail "setup: no orphaned fake"
/bin/bash "$REAPER" --min-age 0 >/dev/null || fail "reaper exited non-zero"
sleep 1
[ -z "$(orphan fake-sidecar.sh)" ] || fail "orphaned test process survived"
[ -n "$(orphan my-server.sh)" ] || fail "a non-test orphan was killed"
kill -0 "$KEEP" 2>/dev/null || fail "a test process with a live parent was killed"
kill -9 "$KEEP"
echo "PASS"

#!/usr/bin/env bash
# Builds the aval binary under test and exports AVAL.
#
# The tmux helpers are `aval tmux <name>` subcommands now, so these suites drive
# the real binary. Set AVAL_BIN to test a prebuilt one; otherwise the current
# source is built into a private temp directory (a few hundred ms when the build
# cache is warm).
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
if [ -n "${AVAL_BIN:-}" ] && [ -x "$AVAL_BIN" ]; then
  AVAL="$AVAL_BIN"
else
  AVAL="${TMPDIR:-/tmp}/aval-tmux-tests-$(id -u)/aval"
  mkdir -p "$(dirname "$AVAL")"
  (cd "$REPO_ROOT" && go build -o "$AVAL" ./cmd/aval) || { echo "go build failed" >&2; exit 1; }
fi
export AVAL

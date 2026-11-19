#!/usr/bin/env bash
# kiri-up: make Docker ready on Kiri, idempotently. Safe to run any time.
#
#   1. Docker answers (starts the dev.kiri.daemon launchd agent only if it does not)
#   2. the active context is `kiri`
#   3. the `multiarch` builder exists, with a bounded BuildKit cache
#   4. /var/run/docker.sock points at Kiri's socket (warns otherwise)
#
# It never deletes anything and never touches containers, images or volumes.
set -euo pipefail

KIRI_SOCK="${KIRI_UP_SOCK:-$HOME/.kiri/run/docker.sock}"
SYSTEM_SOCK="${KIRI_UP_SYSTEM_SOCK:-/var/run/docker.sock}"
TIMEOUT="${KIRI_UP_TIMEOUT:-60}"
INTERVAL="${KIRI_UP_INTERVAL:-1}"
BUILDER="multiarch"
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}" 2>/dev/null || echo "${BASH_SOURCE[0]}")")" && pwd)"
BUILDKIT_CFG="$SCRIPT_DIR/buildkitd.toml"

say() { printf 'kiri-up: %s\n' "$*"; }
warn() { printf 'kiri-up: WARN %s\n' "$*" >&2; }

if [ -n "${DOCKER_HOST:-}" ]; then
  warn "DOCKER_HOST=$DOCKER_HOST overrides the kiri context; unset it (the context is the single source of truth)."
fi

# 1. Docker answers --------------------------------------------------------------
if ! docker info >/dev/null 2>&1; then
  if [ "$(uname -s)" != "Darwin" ]; then
    warn "Docker does not answer and this is not macOS: start the Docker daemon."
    exit 1
  fi
  say "Docker does not answer; starting Kiri..."
  launchctl kickstart "gui/$(id -u)/dev.kiri.daemon" >/dev/null 2>&1 || open -ga Kiri >/dev/null 2>&1 || true
  waited=0
  until docker info >/dev/null 2>&1; do
    if [ "$waited" -ge "$TIMEOUT" ]; then
      warn "Kiri did not answer in ${TIMEOUT}s. Check: kiri diag logs -n 50 --errors"
      exit 1
    fi
    sleep "$INTERVAL"
    waited=$((waited + 1))
  done
fi

# 2. Context ----------------------------------------------------------------------
if [ "$(docker context show 2>/dev/null)" != "kiri" ]; then
  say "switching docker context to kiri"
  docker context use kiri >/dev/null
fi

# 3. Builder ----------------------------------------------------------------------
if ! docker buildx inspect "$BUILDER" >/dev/null 2>&1; then
  say "creating the $BUILDER builder (bounded cache: $BUILDKIT_CFG)"
  docker buildx create --name "$BUILDER" --driver docker-container \
    --driver-opt default-load=true --buildkitd-config "$BUILDKIT_CFG" --bootstrap kiri >/dev/null
fi

# 4. System socket ----------------------------------------------------------------
target="$(readlink "$SYSTEM_SOCK" 2>/dev/null || true)"
if [ "$target" != "$KIRI_SOCK" ]; then
  warn "$SYSTEM_SOCK -> ${target:-missing}, not Kiri ($KIRI_SOCK). Tools that ignore the context will fail."
  warn "If /Library/LaunchDaemons/com.docker.socket.plist exists, Docker Desktop's helper re-links it on every boot:"
  warn "remove com.docker.socket/com.docker.vmnetd (sudo) and run: sudo ln -sfn $KIRI_SOCK $SYSTEM_SOCK"
fi

say "ready (context kiri, builder $BUILDER)"

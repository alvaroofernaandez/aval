#!/usr/bin/env bash
# Tests for kiri-up, the idempotent "make Docker ready on Kiri" command.
#
# Agents and the user run it before any Docker work. It must start Kiri only when
# Docker does not answer, put the `kiri` context back, create the `multiarch`
# builder (with a bounded BuildKit cache: the disk sits near 90%) only when it is
# missing, and flag /var/run/docker.sock when it is not Kiri's (Docker Desktop's
# leftover helper re-links it on boot). docker/launchctl are stubbed on PATH.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKILL_DIR="$REPO/shared/skills/kiri-docker"
SCRIPT="$SKILL_DIR/scripts/kiri-up.sh"
BUILDKIT_CFG="$SKILL_DIR/scripts/buildkitd.toml"

pass=0
fail=0
STUB=""
cleanup() { [ -n "$STUB" ] && rm -rf "$STUB"; }
trap cleanup EXIT

ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }
assert_contains() { case "$3" in *"$2"*) ok "$1" ;; *) ko "$1" "text containing: $2" "$3" ;; esac; }
assert_not_contains() { case "$3" in *"$2"*) ko "$1" "no: $2" "$3" ;; *) ok "$1" ;; esac; }

echo "kiri-up"

assert_eq "kiri-up.sh exists" "yes" "$([ -f "$SCRIPT" ] && echo yes || echo no)"
assert_eq "kiri-up.sh is executable" "yes" "$([ -x "$SCRIPT" ] && echo yes || echo no)"
assert_eq "kiri-up.sh uses strict mode" "yes" "$(rg -q '^set -euo pipefail' "$SCRIPT" 2>/dev/null && echo yes || echo no)"
assert_eq "buildkitd.toml bounds the cache" "yes" "$(rg -q '^\s*maxUsedSpace\s*=' "$BUILDKIT_CFG" 2>/dev/null && echo yes || echo no)"
assert_eq "kiri-docker skill points to kiri-up" "yes" "$(rg -q 'kiri-up' "$SKILL_DIR/SKILL.md" 2>/dev/null && echo yes || echo no)"
assert_eq "manifest links kiri-up into ~/.local/bin" "yes" \
  "$(rg -q '^shared/skills/kiri-docker/scripts/kiri-up\.sh +\.local/bin/kiri-up$' "$REPO/install.manifest" 2>/dev/null && echo yes || echo no)"

STUB="$(mktemp -d)"
LOG="$STUB/calls.log"
# docker stub driven by env:
#   FAKE_DOWN_TRIES  number of `docker info` calls that fail before it answers
#   FAKE_CONTEXT     what `docker context show` prints
#   FAKE_BUILDER     "yes" if `docker buildx inspect multiarch` succeeds
cat >"$STUB/docker" <<EOF
#!/usr/bin/env bash
echo "docker \$*" >>"$LOG"
case "\$1 \${2:-}" in
  "info "*|"info")
    n=\$(cat "$STUB/info.count" 2>/dev/null || echo 0); n=\$((n + 1)); echo \$n >"$STUB/info.count"
    [ "\$n" -gt "\${FAKE_DOWN_TRIES:-0}" ] ;;
  "context show") echo "\${FAKE_CONTEXT:-kiri}" ;;
  "context use") exit 0 ;;
  "buildx inspect") [ "\${FAKE_BUILDER:-yes}" = yes ] ;;
  *) exit 0 ;;
esac
EOF
cat >"$STUB/launchctl" <<EOF
#!/usr/bin/env bash
echo "launchctl \$*" >>"$LOG"
EOF
cat >"$STUB/open" <<EOF
#!/usr/bin/env bash
echo "open \$*" >>"$LOG"
EOF
cat >"$STUB/uname" <<'EOF'
#!/usr/bin/env bash
echo Darwin
EOF
chmod +x "$STUB"/*
ln -s "$STUB/kiri.sock" "$STUB/good.sock.link" 2>/dev/null
ln -s "$STUB/dead/docker.sock" "$STUB/bad.sock.link" 2>/dev/null

run() {
  : >"$LOG"; rm -f "$STUB/info.count"
  env -u DOCKER_HOST PATH="$STUB:/usr/bin:/bin" HOME="$STUB" \
    KIRI_UP_SOCK="$STUB/kiri.sock" KIRI_UP_SYSTEM_SOCK="${SYS_SOCK:-$STUB/good.sock.link}" \
    KIRI_UP_TIMEOUT=3 KIRI_UP_INTERVAL=0 "$@" bash "$SCRIPT" 2>&1
  echo "exit=$?"
}

if [ -f "$SCRIPT" ]; then
  out="$(run)"; calls="$(cat "$LOG")"
  assert_contains "healthy: succeeds" "exit=0" "$out"
  assert_not_contains "healthy: does not kickstart Kiri" "launchctl" "$calls"
  assert_not_contains "healthy: does not create a builder" "buildx create" "$calls"
  assert_not_contains "healthy: does not switch context" "context use" "$calls"
  assert_not_contains "healthy: no socket warning" "WARN" "$out"

  out="$(run FAKE_DOWN_TRIES=1)"; calls="$(cat "$LOG")"
  assert_contains "down: kickstarts the Kiri daemon" "launchctl kickstart gui/" "$calls"
  assert_contains "down: targets dev.kiri.daemon" "dev.kiri.daemon" "$calls"
  assert_contains "down: recovers" "exit=0" "$out"

  out="$(run FAKE_DOWN_TRIES=999)"
  assert_not_contains "never answers: fails" "exit=0" "$out"
  assert_contains "never answers: points to the logs" "kiri diag logs" "$out"

  out="$(run FAKE_CONTEXT=desktop-linux)"; calls="$(cat "$LOG")"
  assert_contains "wrong context: switches to kiri" "docker context use kiri" "$calls"

  out="$(run FAKE_BUILDER=no)"; calls="$(cat "$LOG")"
  assert_contains "no builder: creates multiarch" "docker buildx create --name multiarch" "$calls"
  assert_contains "no builder: on the kiri endpoint" "--bootstrap kiri" "$calls"
  assert_contains "no builder: loads images by default" "default-load=true" "$calls"
  assert_contains "no builder: with the bounded BuildKit config" "--buildkitd-config" "$calls"

  out="$(SYS_SOCK="$STUB/bad.sock.link" run)"
  assert_contains "foreign system socket: warns" "WARN" "$out"
  assert_contains "foreign system socket: names the Docker Desktop helper" "com.docker.socket" "$out"
  assert_contains "foreign system socket: still succeeds" "exit=0" "$out"

  out="$(run DOCKER_HOST=tcp://x:2375)"
  assert_contains "DOCKER_HOST set: warns it overrides the context" "DOCKER_HOST" "$out"
fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

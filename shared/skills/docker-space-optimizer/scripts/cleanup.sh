#!/usr/bin/env bash
# Safe Docker space cleanup for Kiri: stopped containers, dangling images,
# build cache (daemon and the `multiarch` builder). Never touches volumes,
# tagged images or running containers. Refuses any context other than `kiri`.
set -euo pipefail

KIRI_DATA="$HOME/Library/Application Support/Kiri"

ctx="$(docker context show)"
if [ "$ctx" != "kiri" ]; then
  echo "Refusing to prune: active docker context is '$ctx', expected 'kiri'." >&2
  echo "Run: docker context use kiri (or load the kiri-health skill)" >&2
  exit 1
fi

report() {
  echo "== $1 =="
  df -h / | tail -1
  du -sh "$KIRI_DATA" 2>/dev/null || echo "Kiri data dir not found: $KIRI_DATA"
  if [ -f "$KIRI_DATA/vm/data.img" ]; then
    du -h "$KIRI_DATA/vm/data.img" | awk '{print $1 " real  vm/data.img"}'
  else
    echo "vm/data.img not found"
  fi
  docker system df
  echo
}

# Kiri's vm/data.img grows on write and never hands freed blocks back to macOS by
# itself. fstrim on the VM's ext4 does. The filesystem is reached through a
# throwaway volume (it lives on /dev/vda): mounting the block device a second
# time could corrupt it, so that is never done. Verified 2026-11-19: 49.4 -> 25.2 GB.
PROBE_VOLUME="kiri-fstrim-probe"
trim_vm_disk() {
  docker volume create "$PROBE_VOLUME" >/dev/null
  docker run --rm --privileged -v "$PROBE_VOLUME:/v" alpine:3 sh -c 'fstrim -v /v' \
    || echo "fstrim failed; data.img keeps its freed blocks until the next run."
  docker volume rm "$PROBE_VOLUME" >/dev/null || true
}

report BEFORE

docker container prune -f
docker image prune -f
docker builder prune -af
docker buildx prune --builder multiarch -af || echo "builder 'multiarch' not available, skipped (see kiri-health)."
trim_vm_disk
echo

report AFTER
echo "Compare the Kiri data dir size above: that measured difference is what was reclaimed on disk."

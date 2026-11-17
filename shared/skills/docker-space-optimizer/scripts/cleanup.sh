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
  docker system df
  echo
}

report BEFORE

docker container prune -f
docker image prune -f
docker builder prune -af
docker buildx prune --builder multiarch -af || echo "builder 'multiarch' not available, skipped (see kiri-health)."
echo

report AFTER
echo "Compare the Kiri data dir size above: that measured difference is what was reclaimed on disk."

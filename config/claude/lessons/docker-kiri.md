---
triggers:
  - "**/Dockerfile*"
  - "**/.dockerignore"
  - "**/{docker-,}compose*.{yml,yaml}"
commands:
  - '(^|[;&|(] *)docker( |$)'
  - '(?i)orbstack|colima|docker desktop'
---
Kiri is the only Docker runtime on macOS; never use or recommend OrbStack,
Colima or Docker Desktop. Use `docker`, `docker compose`, `docker buildx` on the
`kiri` context with `BUILDX_BUILDER=multiarch` (verify with
`docker image inspect -f '{{.Architecture}}'`). Show what will be removed and ask
before deleting images, volumes or cache. Prefer small multi-stage images,
`.dockerignore`, Buildx cache. No Kubernetes unless asked. Skills: `kiri-docker`,
`docker-space-optimizer`, `kiri-health`, `docker-build-optimized`.

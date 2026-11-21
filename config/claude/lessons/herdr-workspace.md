---
commands:
  - '(^|[;&|(] *)(hc|herdr-app|herdr-core)( |$)'
---
Herdr is the only workspace (Orca was removed 2026-11-19): `hc` runs terminals
and agents, `herdr-app` is the window. Delegate long, parallel or cross-agent
work and slow commands to `hc run --bg`; never add agent hooks (status is
native). Load the `herdr` skill before any `hc` or `herdr-app` command.

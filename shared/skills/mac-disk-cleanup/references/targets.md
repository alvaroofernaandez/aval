# Cleanup targets

## Tier 1 — regenerable, delete without asking

```bash
rm -rf ~/Library/Caches/*
fd -H -d 1 . ~/.cache -E dokploy-deploy -X rm -rf   # NEVER ~/.cache/dokploy-deploy: Dokploy env backups (rollback)
npm cache clean --force
rm -rf ~/.bun/install/cache
pnpm store prune            # ~/Library/pnpm/store, often 15-20 GB
go clean -modcache
brew cleanup -s --prune=all
```

## Measure (du -sh)

`~/Library/Caches ~/.cache ~/.npm ~/.bun/install/cache ~/Library/pnpm/store go/pkg/mod/cache
~/.cargo/registry ~/Library/Developer/Xcode/DerivedData ~/Library/Developer/CoreSimulator
~/.gradle/caches "~/Library/Application Support/Kiri" ~/.local/share/opencode
~/.Trash "~/Library/Application Support/Steam/steamapps/common" /opt/homebrew /nix`

## Container runtimes on this machine

- Kiri is the only Docker runtime (context `kiri`); data lives in `~/Library/Application Support/Kiri`, config in `~/.kiri`. data.img never shrinks by itself; fstrim through a throwaway volume does (49.4 → 25.2 GB on 2026-11-19), see docker-space-optimizer. OrbStack was retired on 2026-11-17.
- Colima, Lima and ColimaBar were removed on 2026-11-02. The history below predates that and its fstrim recipes no longer apply.
- Apple `container` CLI is still installed but has no data and is not used for Docker.

## Tier 2 — ask first

- Docker images: usually per-commit tags of work images (`ghcr.io/<org>/*`); keep `latest` + its commit tag.
- Docker volumes (Postgres/MinIO data of compose projects): never prune without listing them.
- `~/.local/share/opencode/opencode.db` (session history, ~5-7 GB). Rotated `log/2026-*.log` are safe.
- Steam Baldur's Gate 3 (~146 GB), WhatsApp group container (~9 GB).

## Past results

- 2026-08-28: fstrim alone 2.2 → 182 GB free.
- 2026-10-01: deleted container data + profiles + pnpm prune, 2.1 → 72 GB.
- 2026-10-09: Tier 1 + Colima build cache 18.9 GB + fstrim, 4.3 → 45 GB. Removing 15 old image tags freed ~0 B (shared layers).
- 2026-10-10: Tier 1 (pnpm/npm/caches/brew) 24 → 36 GiB; orphan `~/.colima/_lima/_disks/colima-rosetta` (15 GB real, no profile in `colima list`) deleted → 50 GiB. Check `_lima/_disks` for orphans when `colima list` is empty.
- 2026-10-10 (2nd pass): nix-collect-garbage +2 GB (only 1 generation; home-manager profile ~4.6 GB). `cargo clean` launcher+harness Tauri targets 49 GiB, all `.next` 19 GB, WhatsApp group container 10 GB, minecraft, Whisky, ~/.windsurf ~/.trae ~/.vscode, ~/.claudeops (3.5 GB WAL) → 123 GiB free. Biggest regenerable pool: ~/Proyectos build artifacts (node_modules 55 GB, target, .next, dist).
- 2026-10-10 (3rd pass): removed 32 merged+clean agent worktrees (tecnisan, cobbleverse `.claude/worktrees`, du ~23 GB) → only +9 GiB real (pnpm hardlinks). `pnpm store prune` after it freed 0: main checkouts still reference the same packages. 132 GiB free.
- 2026-10-11: Tier 1 11 → 18 GiB; orphan `_lima/_disks/colima/datadisk` (53 GB real, Colima 0.10 keeps it after `colima delete`) + all `~/Proyectos` node_modules (166 dirs) → 106 GiB. Rust `target/` (58 GB, mostly haga-harness-port worktrees) left by user choice.
- 2026-10-14: orphan `_lima/_disks/colima-rosetta` reappeared (84 GB, no profile) and was deleted, plus ~/Library/Caches/colima; `default` profile (6.8 GB) kept by user choice → 38 → 139 GiB free.
- 2026-10-21: Tier 1 28 → 34 GiB; orphan `_lima/_disks/colima-rosetta` (33 GB, 3rd recurrence) + `_lima/_disks/colima` (9.4 GB, `default` profile gone) + `~/.gradle/caches` (18 GB) → 76 GiB free. Steam BG3 kept.
- 2026-10-22: `default` disk grew 9.4 → 74 GB in one day (47.6 GB build cache). Tier 1 +1.2 GB; builder prune both VMs + rosetta fstrim → 24 GiB; `default` VM had stopped, so user chose to delete both profiles entirely (images + 14 volumes) + leftover `_lima/_disks/*` (81 GB) → 106 GiB free.
- 2026-10-28: `rm -rf ~/.cache/*` wiped ~/.cache/dokploy-deploy (prod env backups); Tier 1 now excludes it. 2.8 → 9.6 GiB after Tier 1.
- 2026-10-29: 834 MiB free. Tier 1 (~0.4 GB) + rosetta builder prune 4.4 GB + fstrim 12.8 GB → 14 GiB; then user approved all ~/Proyectos + ~/japan-2027 node_modules/.next/.turbo, Rust target/ (harness 11 GB, launcher 6.6 GB) and `default` profile + its disk → 120 GiB free. Steam BG3 kept.
- 2026-10-30: 82 GiB free. Tier 1 +5 GiB (pnpm/npm/caches) → 87 GiB. rosetta build cache already 0; fstrim 3.3 GiB, no visible change. No `latest` tags exist (only sha-*): kept newest per repo, removed 92 tags → images 14.7 → 4.4 GB, fstrim 16.4 GiB, colima-rosetta disk 20 → 16 GB, but only 88 GiB free (+1).
- 2026-10-31: 46 GiB free; colima-rosetta disk 16 → 71 GB in one day (59.6 GB build cache). Tier 1 + builder prune + fstrim → 96 GiB. Removed 104 old tags (kept newest per repo; pgvector/postgres refused, in use) → images 16.5 → 11.8 GB, disk 26 → 23 GB, 99 GiB free. Build cache regrew to 8.3 GB mid-pass (active buildx builders).
- 2026-11-02: migrated Colima → OrbStack (5 named volumes copied, checksums identical); deleted ~/.colima (67 GB) → 40 → 100 GiB free.
- 2026-11-02 (post-OrbStack): 103 GiB free. Tier 1 +6 GiB → 109; safe Docker prune 673 MB but an active build regrew cache (4.6 GB in use) → 106 GiB. Unused volumes docker_postgres_data (2.5 GB), mir_pgdata, mir_materials are the ONLY copy of data migrated from Colima today (~/.colima deleted) — never prune them; "unused" only means no container is up. Steam BG3 kept.
- 2026-11-05: 61 GiB free. Tier 1 +13 → 74; builder prune 9.2 GB → 81; ~/Proyectos node_modules (135) + .next (69) + .turbo (19) + harness Tauri target 6.5 GB + 17 old image tags (images 10.3 → 7.4 GB) → 126 GiB. Steam BG3, e2e images and migrated volumes kept.
- 2026-11-08: 7.2 GiB free. Tier 1 ~8 GB → 15; daemon build cache 14 GB + buildx builder `soho-multi` cache 10.7 GB (`docker buildx prune --builder soho-multi -af`, lives in volume buildx_buildkit_soho-multi0_state) → 37; node_modules (201) 18.7 GB + .turbo 4.3 GB + 14 old puertanueva tags → 46 GiB. haga-harness-port Rust target/ (65.6 GB, 14 agent worktrees) delegated to the `harness` session. It removed the 14 agent-worktree targets (~57 GB, work merged into p7/all), kept main target (8 GB) → 102 GiB free. Steam BG3, volumes, dist/ kept.
- 2026-11-09: 41 GiB free. Tier 1 +3 → 44; builder prune 16.8 GB (OrbStack data 32 → 7.8 GB, shrank within minutes); 36 old tags + 2 e2e images (images 19.3 → 5.6 GB); node_modules (44) 8.4 GB + .next/.turbo 3.5 GB + harness main target 9.8 GB → 84 GiB; removed 133 clean idle (>24h, not cwd of any process, commits kept in a branch/ref) worktrees with `git worktree remove` (branches kept) → 115 GiB free. Kept: 37 recent worktrees, 33 dirty, 2 locked. Steam BG3, volumes kept.
- 2026-11-14: 30 GiB free. Tier 1 +17 → 47; safe Docker prune (build cache 11 GB, 19 stopped containers) → 58; ~/Proyectos node_modules/.next/.turbo + Rust target/ (153 dirs, 64 GB) → 111 GiB free. Images (31 GB), volumes, Steam BG3 kept. Purpose: room for a 25.6 GB NX2Apple game build.
- 2026-11-19: 47 GiB free. Tier 1 +4 → 51; multiarch builder cache trimmed 32.6 → 19.8 GB (`docker buildx prune --builder multiarch --max-used-space 20gb`, then `buildx rm --keep-state` + kiri-up to apply the 20 GiB GC policy); Kiri data file stayed 51 GB (no shrink seen). 912 node_modules/.next/.turbo dirs outside live repos (soho-cumpleanos `.turbo` alone 22 GB) → 85 GiB; Kiri builder `kiri-build0` state (5.9 GB, unused) + 6 old turing tags removed (~0 B, shared layers). Kept: Steam BG3, DB volumes, the 3 repos with live dev processes, harness-port target/ (11 GB).
- 2026-11-19 (2nd): fstrim via throwaway volume on the Kiri VM ext4 trimmed 41 GB: data.img 49.4 → 25.2 GB real, 86 → 110 GiB free. Volumes/Postgres intact. Now automated at the end of docker-space-optimizer cleanup.sh.

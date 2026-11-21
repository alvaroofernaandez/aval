# Workflow automation and remaining fixes

## T1 — recurring failures
- [x] SSH: IdentitiesOnly + explicit key per VPS alias
- [x] Engram: stale runtime sessions / ambiguous project
- [x] gentle-ai receipt review: off (user-owned rule), no nested-repo failures
- [x] Headless browser: verify Lightpanda vs Chromium claims; fix Playwright drift
- [x] pandoc installed (Brewfile)

## T2 — Claude Code settings
- [-] permissions.blockWritesOutsideWorkingDirectories — rejected: also blocks
      writes to ~/.claude and other repos, which aval maintenance needs
- [x] CLAUDE_CODE_SUBAGENT_EFFORT, subagentPromptCacheTtl
- [x] session-report plugin, LSP plugins (pyright, gopls)
- [x] agnix lint in the suite

## T3 — screenshots auto-copied to the clipboard
- [x] LaunchAgent + scripts/macos/screenshot-to-clipboard

## T4 — research: Willow Voice; habits audit (shell, Claude Code, browser, OS)

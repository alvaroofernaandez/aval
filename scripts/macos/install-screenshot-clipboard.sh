#!/usr/bin/env bash
# Send every screenshot to ~/Pictures/Screenshots AND to the clipboard.
#
# Sets the macOS screenshot folder, then installs a LaunchAgent that runs
# screenshot-to-clipboard whenever that folder changes. Idempotent.
# SKIP_DEFAULTS=1 and SKIP_LAUNCHCTL=1 let the test suite run it in a sandbox.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHOTS="$HOME/Pictures/Screenshots"
LABEL="dev.aval.screenshot-clipboard"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"

mkdir -p "$SHOTS" "$(dirname "$PLIST")"

if [ "${SKIP_DEFAULTS:-0}" != 1 ]; then
  defaults write com.apple.screencapture location "$SHOTS"
  defaults write com.apple.screencapture type png
  killall SystemUIServer 2>/dev/null || true
fi

cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/zsh</string>
    <string>$REPO/scripts/macos/screenshot-to-clipboard</string>
    <string>$SHOTS</string>
  </array>
  <key>WatchPaths</key>
  <array><string>$SHOTS</string></array>
  <key>ThrottleInterval</key><integer>1</integer>
</dict>
</plist>
EOF

if [ "${SKIP_LAUNCHCTL:-0}" != 1 ]; then
  launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
  launchctl bootstrap "gui/$(id -u)" "$PLIST"
fi
echo "screenshots: $SHOTS (also copied to the clipboard)"

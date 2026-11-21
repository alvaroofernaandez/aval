#!/usr/bin/env bash
# macOS defaults chosen in the habits audit (2026-11-21). Idempotent.
# Key repeat takes effect after logging out and back in.
set -euo pipefail
defaults write NSGlobalDomain KeyRepeat -int 2            # fastest repeat the UI allows
defaults write NSGlobalDomain InitialKeyRepeat -int 15    # short delay before repeating
defaults write NSGlobalDomain AppleShowAllExtensions -bool true
defaults write com.apple.finder ShowPathbar -bool true
killall Finder 2>/dev/null || true
echo "macOS defaults applied (log out for key repeat)"

#!/usr/bin/env bash
# scripts/install.sh downloads a binary and then runs it, so it must verify the
# download against the release's SHA256SUMS BEFORE anything is installed or
# executed. Fixtures are served over file:// (curl handles it), so the suite
# needs no network and no real release.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$REPO/scripts/install.sh"

pass=0
fail=0
ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "bootstrap-verify"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

case "$(uname -s)" in Darwin) OS=darwin ;; *) OS=linux ;; esac
case "$(uname -m)" in x86_64 | amd64) ARCH=amd64 ;; *) ARCH=arm64 ;; esac
ASSET="aval-$OS-$ARCH"

sha256() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1"; else shasum -a 256 "$1"; fi; }

# A release directory: a "binary" that announces itself, plus its SHA256SUMS.
make_release() { # $1 dir
  mkdir -p "$1"
  printf '#!/bin/sh\necho FIXTURE-BINARY-RAN\n' >"$1/$ASSET"
  (cd "$1" && sha256 "$ASSET" >SHA256SUMS)
}

# Run the bootstrap against a release directory; sets OUT, CODE and DIR.
bootstrap() { # $1 release dir
  DIR="$TMP/aval-$RANDOM"
  OUT="$(HOME="$TMP/home" AVAL_DIR="$DIR" AVAL_REPO_URL="$REPO" \
    AVAL_RELEASE_BASE="file://$1" sh "$SCRIPT" 2>&1)"
  CODE=$?
}
mkdir -p "$TMP/home"

# 1. A genuine release installs and runs.
make_release "$TMP/good"
bootstrap "$TMP/good"
assert_eq "a binary matching SHA256SUMS is installed and run" "0:yes" \
  "$CODE:$(printf '%s' "$OUT" | rg -q 'FIXTURE-BINARY-RAN' && echo yes || echo no)"

# 2. A tampered binary is refused and never runs.
make_release "$TMP/tampered"
printf '#!/bin/sh\necho EVIL-RAN\n' >"$TMP/tampered/$ASSET"
bootstrap "$TMP/tampered"
assert_eq "a tampered binary fails the install" "yes" "$([ "$CODE" -ne 0 ] && echo yes || echo no)"
assert_eq "...says why" "yes" "$(printf '%s' "$OUT" | rg -qi 'checksum' && echo yes || echo no)"
assert_eq "...and never executes it" "no" "$(printf '%s' "$OUT" | rg -q 'EVIL-RAN' && echo yes || echo no)"
assert_eq "...and leaves no binary behind" "no" "$([ -e "$DIR/bin/aval" ] && echo yes || echo no)"
assert_eq "...and leaves no temp file behind" "no" "$([ -e "$DIR/bin/aval.tmp" ] && echo yes || echo no)"

# 3. A release without SHA256SUMS cannot be verified, so it is refused.
make_release "$TMP/nosums"
rm "$TMP/nosums/SHA256SUMS"
bootstrap "$TMP/nosums"
assert_eq "a release without SHA256SUMS is refused" "yes" "$([ "$CODE" -ne 0 ] && echo yes || echo no)"
assert_eq "...and nothing is installed" "no" "$([ -e "$DIR/bin/aval" ] && echo yes || echo no)"

# 4. SHA256SUMS that does not list this asset is refused too.
make_release "$TMP/missing"
printf '%s  aval-plan9-amd64\n' "$(printf x | { sha256sum 2>/dev/null || shasum -a 256; } | cut -d' ' -f1)" >"$TMP/missing/SHA256SUMS"
bootstrap "$TMP/missing"
assert_eq "SHA256SUMS without an entry for this asset is refused" "yes" "$([ "$CODE" -ne 0 ] && echo yes || echo no)"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

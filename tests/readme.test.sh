#!/usr/bin/env bash
# Guards the README and the images it is built from.
#
# Two things can go wrong with a public README, and neither shows up in a diff
# review: it leaks something (a token pasted into an example, a home path, an
# email), or its images drift from the generator so nobody can rebuild them.
# This suite runs the generator into a scratch dir and checks both.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
README="$REPO/README.md"
GUIDE="$REPO/docs/guide.md"

pass=0
fail=0

ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "readme"

out="$(mktemp -d)"
trap 'rm -rf "$out"' EXIT

# --- generator ---------------------------------------------------------------
gen_status=0
python3 "$REPO/docs/brand.py" --out "$out" >/dev/null 2>&1 || gen_status=$?
assert_eq "docs/brand.py runs" "0" "$gen_status"

for f in hero-light.svg hero-dark.svg stack-light.svg stack-dark.svg; do
  assert_eq "generator emits $f" "yes" "$([ -s "$out/$f" ] && echo yes || echo no)"
done

# Committed images must be exactly what the generator produces, or the README
# shows something no command can rebuild.
for f in hero-light.svg hero-dark.svg stack-light.svg stack-dark.svg; do
  same="$(cmp -s "$out/$f" "$REPO/docs/assets/$f" && echo yes || echo no)"
  assert_eq "committed $f matches the generator" "yes" "$same"
done

# Monochrome: every colour literal must be a pure grey (#rgb with r=g=b).
non_grey="$(rg -o --no-filename '#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b' "$out" 2>/dev/null \
  | tr 'A-F' 'a-f' \
  | awk '{h=substr($0,2); if (length(h)==3) {r=substr(h,1,1); g=substr(h,2,1); b=substr(h,3,1)} else {r=substr(h,1,2); g=substr(h,3,2); b=substr(h,5,2)} if (r!=g || g!=b) print $0}' \
  | sort -u | tr '\n' ' ')"
assert_eq "images use only black, white and greys" "" "$non_grey"

# One tile per tool, and each tile is labelled so the image is not a mystery
# to a screen reader.
tools="$(python3 "$REPO/docs/brand.py" --list 2>/dev/null)"
tool_count="$(printf '%s\n' "$tools" | rg -c .)"
assert_eq "generator lists at least 20 tools" "yes" "$([ "${tool_count:-0}" -ge 20 ] && echo yes || echo no)"
missing=""
while IFS= read -r t; do
  [ -n "$t" ] || continue
  rg -qF ">$t<" "$out/stack-light.svg" || missing="$missing $t"
done <<<"$tools"
assert_eq "every tool has a labelled tile" "" "$missing"

# --- README ------------------------------------------------------------------
for theme in light dark; do
  assert_eq "README shows hero-$theme" "yes" \
    "$(rg -qF "docs/assets/hero-$theme.svg" "$README" && echo yes || echo no)"
  assert_eq "README shows stack-$theme" "yes" \
    "$(rg -qF "docs/assets/stack-$theme.svg" "$README" && echo yes || echo no)"
done
assert_eq "README switches images with the GitHub theme" "yes" \
  "$(rg -qF 'prefers-color-scheme: dark' "$README" && echo yes || echo no)"
assert_eq "README links the full guide" "yes" \
  "$(rg -qF '(docs/guide.md' "$README" && echo yes || echo no)"
assert_eq "the full guide exists" "yes" "$([ -s "$GUIDE" ] && echo yes || echo no)"

# Every relative link and image in the README resolves to a real file.
broken=""
while IFS= read -r ref; do
  path="${ref%%#*}"
  [ -n "$path" ] || continue
  [ -e "$REPO/$path" ] || broken="$broken $path"
done < <(rg -o --no-filename '(?:\]\(|src="|srcset=")([^")#][^")]*)' -r '$1' "$README" | rg -v '^(https?:|mailto:)')
assert_eq "README has no broken relative links" "" "$broken"

# --- leaks -------------------------------------------------------------------
for doc in "$README" "$GUIDE"; do
  name="${doc#"$REPO"/}"
  [ -f "$doc" ] || continue
  gl=0
  gitleaks detect --no-git --no-banner --source "$doc" >/dev/null 2>&1 || gl=$?
  assert_eq "$name: gitleaks finds nothing" "0" "$gl"

  personal="$(rg -n -e '/Users/[a-z]' -e '/home/[a-z]' -e '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}' \
    -e '\b[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\b' "$doc" | tr '\n' ' ')"
  assert_eq "$name: no home paths, emails or IPs" "" "$personal"
done

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

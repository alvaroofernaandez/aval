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

IMAGES="hero-light.svg hero-dark.svg stack-light.svg stack-dark.svg
mark-light.svg mark-dark.svg wordmark-light.svg wordmark-dark.svg"

for f in $IMAGES; do
  assert_eq "generator emits $f" "yes" "$([ -s "$out/$f" ] && echo yes || echo no)"
done

# The brand is drawn, not typeset: a wordmark set in a font renders in whatever
# fallback the viewer has, so the logo would change from machine to machine.
assert_eq "wordmark is drawn with paths, not text" "0" \
  "$(rg -c '<text' "$out/wordmark-light.svg" || echo 0)"
assert_eq "hero carries the aval wordmark" "yes" \
  "$(rg -q 'aria-label="aval' "$out/hero-light.svg" && echo yes || echo no)"
assert_eq "hero is a certificate: guilloché rosette" "yes" \
  "$(rg -q 'data-part="guilloche"' "$out/hero-light.svg" && echo yes || echo no)"
assert_eq "hero seals the rosette with the isotype" "yes" \
  "$(rg -q 'data-part="seal"' "$out/hero-light.svg" && echo yes || echo no)"
assert_eq "hero defines the word" "yes" \
  "$(rg -q 'Garantía' "$out/hero-light.svg" && echo yes || echo no)"
assert_eq "hero drops the old suminagashi" "0" \
  "$(rg -c -i 'suminagashi|id="vig"' "$out/hero-light.svg" || echo 0)"
assert_eq "hero no longer says Dotfiles" "0" \
  "$(rg -c -i 'dotfiles' "$out/hero-light.svg" || echo 0)"
# Legible as a 16px favicon: the stroke must be at least 1/8 of the canvas.
stroke_ok="$(python3 - "$out/mark-light.svg" <<'PY'
import re, sys
s = open(sys.argv[1]).read()
w = float(re.search(r'viewBox="0 0 ([\d.]+)', s).group(1))
widths = [float(x) for x in re.findall(r'stroke-width="([\d.]+)"', s)]
print("yes" if widths and min(widths) >= w / 8 else "no")
PY
)"
assert_eq "mark strokes survive a 16px favicon" "yes" "$stroke_ok"

# Committed images must be exactly what the generator produces, or the README
# shows something no command can rebuild.
for f in $IMAGES; do
  same="$(cmp -s "$out/$f" "$REPO/docs/assets/$f" && echo yes || echo no)"
  assert_eq "committed $f matches the generator" "yes" "$same"
done

# The hero is monochrome: every colour literal must be a pure grey (r=g=b).
# The logo wall is not — it shows each tool's real, coloured mark.
non_grey="$(rg -o --no-filename '#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b' "$out"/hero-*.svg "$out"/mark-*.svg "$out"/wordmark-*.svg 2>/dev/null \
  | tr 'A-F' 'a-f' \
  | awk '{h=substr($0,2); if (length(h)==3) {r=substr(h,1,1); g=substr(h,2,1); b=substr(h,3,1)} else {r=substr(h,1,2); g=substr(h,3,2); b=substr(h,5,2)} if (r!=g || g!=b) print $0}' \
  | sort -u | tr '\n' ' ')"
assert_eq "hero uses only black, white and greys" "" "$non_grey"

# Real logos, embedded, for every tool that publishes one. Embedded because a
# README image cannot load external files: an <image href="docs/..."> inside an
# SVG renders as a blank square on GitHub.
nologo="$(python3 "$REPO/docs/brand.py" --no-logo 2>/dev/null | tr '\n' ' ')"
assert_eq "only tools with no published logo lack one" "ripgrep delta " "$nologo"
embedded="$(rg -c '<image ' "$out/stack-light.svg")"
expected=$(( $(python3 "$REPO/docs/brand.py" --list 2>/dev/null | rg -c .) - 2 ))
assert_eq "every other tool embeds its logo" "$expected" "${embedded:-0}"
assert_eq "logos are not loaded from outside the SVG" "0" \
  "$(rg -c 'href="(https?:|\.\.?/|docs/)' "$out/stack-light.svg" || echo 0)"
size="$(rg -o 'class="logo" width="([0-9]+)"' -r '$1' "$out/stack-light.svg" | sort -n | head -1)"
assert_eq "logos render at least 56px" "yes" "$([ "${size:-0}" -ge 56 ] && echo yes || echo no)"

# Every vendored logo says where it came from.
unsourced=""
for f in "$REPO"/docs/logos/*.*; do
  b="$(basename "$f")"
  [ "$b" = "SOURCES.md" ] && continue
  rg -qF "\`$b\`" "$REPO/docs/logos/SOURCES.md" 2>/dev/null || unsourced="$unsourced $b"
done
assert_eq "every logo lists its source" "" "$unsourced"

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

# The brand: the README introduces aval, says what the word means, and the
# old installer name is gone everywhere a reader would copy a command from.
assert_eq "README shows the aval mark" "yes" \
  "$(rg -qF 'docs/assets/mark-dark.svg' "$README" && rg -qF 'docs/assets/mark-light.svg' "$README" && echo yes || echo no)"
assert_eq "README explains what aval means" "yes" \
  "$(rg -qi 'garant' "$README" && echo yes || echo no)"
for doc in "$README" "$GUIDE" "$REPO/npm/cli/README.md"; do
  assert_eq "${doc#"$REPO"/}: no old installer name" "0" \
    "$(rg -c 'dotfiles-installer|\.dotfiles-backup|DOTFILES_' "$doc" || echo 0)"
done

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

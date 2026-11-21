#!/usr/bin/env bash
# Guards interactive zsh startup time without losing behaviour.
#
# Measured 2026-11-21 (M4): 113.7 ms ± 0.8 per `zsh -i -c exit` against 2.7 ms
# for a bare zsh. zprof and timing put the cost in oh-my-zsh, loaded for one
# plugin and running a compinit that zsh-autocomplete asks nobody else to run
# (~27 ms), and in five external `eval "$(tool init)"` calls on every start
# (~36 ms) whose output only changes when the tool is upgraded.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ZSHRC="$REPO/home/zshrc"
BUDGET_MS=80

pass=0
fail=0

ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

echo "shell-startup"

# --- structure -----------------------------------------------------------------
assert_eq "oh-my-zsh is not loaded" "0" \
  "$(rg -c 'oh-my-zsh\.sh' "$ZSHRC" || echo 0)"
assert_eq "no tool init is evaluated from a subprocess at startup" "" \
  "$(rg -n 'eval "\$\(|source <\(' "$ZSHRC" | tr '\n' ' ')"
assert_eq "no subprocess for facts zsh already knows (uname, go env)" "" \
  "$(rg -n '\$\(uname\)|\$\(go env' "$ZSHRC" | tr '\n' ' ')"
assert_eq "no hard-coded home directory" "" \
  "$(rg -n '/Users/[a-z]|/home/[a-z]' "$ZSHRC" | rg -v linuxbrew | tr '\n' ' ')"
assert_eq "fzf's directory widget command uses the right variable" "yes" \
  "$(rg -q '^export FZF_ALT_C_COMMAND=.*--exclude' "$ZSHRC" && echo yes || echo no)"
assert_eq "no hook calls a node manager that is not installed" "0" \
  "$(rg -c 'nvm use' "$ZSHRC" || echo 0)"

# --- behaviour that must survive -------------------------------------------------
probe="$(zsh -i -c '
  print "z=$(whence -w z | cut -d" " -f2)"
  print "atuin=$(bindkey "^R" | rg -o "atuin[-a-z]*" | head -1)"
  print "fzf=$(bindkey "^T" | rg -o "fzf[-a-z]*" | head -1)"
  print "hist=$HISTSIZE share=$([[ -o sharehistory ]] && print on || print off)"
  print "prompt=$(( ${+functions[prompt_powerlevel10k_setup]} || ${+functions[_p9k_precmd]} ))"
' 2>/dev/null)"
assert_eq "zoxide's z is defined" "z=function" "$(printf '%s\n' "$probe" | rg '^z=')"
assert_eq "atuin owns Ctrl-R" "yes" "$(printf '%s\n' "$probe" | rg -q '^atuin=atuin' && echo yes || echo no)"
assert_eq "fzf owns Ctrl-T" "yes" "$(printf '%s\n' "$probe" | rg -q '^fzf=fzf' && echo yes || echo no)"
assert_eq "history is large and shared" "yes" \
  "$(printf '%s\n' "$probe" | rg -q '^hist=(5[0-9]{4}|[6-9][0-9]{4}|[0-9]{6,}) share=on' && echo yes || echo no)"
assert_eq "powerlevel10k prompt is loaded" "prompt=1" "$(printf '%s\n' "$probe" | rg '^prompt=')"

# A cache older than its binary (what `brew upgrade` produces) is regenerated,
# so an upgrade is never silently masked by a stale init script.
cache="${XDG_CACHE_HOME:-$HOME/.cache}/zsh-init/zoxide.zsh"
if [ -f "$cache" ]; then
  touch -t 200001010000 "$cache"
  zsh -i -c exit >/dev/null 2>&1
  assert_eq "a stale init cache is regenerated after an upgrade" "yes" \
    "$([ "$cache" -nt "$(command -v zoxide)" ] && echo yes || echo no)"
fi

# --- budget --------------------------------------------------------------------
if command -v hyperfine >/dev/null 2>&1; then
  json="$(mktemp)"
  hyperfine -N -w 3 -r 20 --style none --export-json "$json" 'zsh -i -c exit' >/dev/null 2>&1
  # hyperfine 2.x nests timings under summary; 1.x kept median at the top level.
  ms="$(jq '(.results[0].summary.time_wall_clock.median // .results[0].median) * 1000 | floor' "$json" 2>/dev/null)"
  rm -f "$json"
  assert_eq "interactive startup under ${BUDGET_MS} ms (median ${ms} ms)" "yes" \
    "$([ "${ms:-999}" -lt "$BUDGET_MS" ] && echo yes || echo no)"
else
  echo "  SKIP startup budget (hyperfine not installed)"
fi

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

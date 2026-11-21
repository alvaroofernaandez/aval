# Shell helpers from the habits audit (2026-11-21). Sourced by home/zshrc.
# Each one replaces something typed hundreds of times; numbers are in
# tests/shell_helpers_test.go.

# Typos of clear/exit (~150 in history). Ctrl+L also clears.
alias -- cñear=clear ckear=clear clea=clear clar=clear exir=exit exirt=exit

# gp: pull (with submodules) and show where the branch stands.
gp() { git pull --recurse-submodules "$@" && git status -sb }

# rd: react-doctor on the current diff.
rd() { npx -y react-doctor@latest . --verbose --diff "$@" }

# up / down: start or stop the local stack, whatever this project uses.
# Order: Makefile target, compose file, npm stack scripts.
_stack_kind() {
  if [[ -f Makefile && $'\n'"$(<Makefile)" == *$'\n'up-build:* ]]; then print make
  elif [[ -f compose.yaml || -f compose.yml || -f docker-compose.yml || -f docker-compose.yaml ]]; then print compose
  elif [[ -f package.json && "$(<package.json)" == *'"dev:stack:up"'* ]]; then print npm
  else return 1; fi
}

up() {
  case $(_stack_kind) in
    make) make up-build "$@" ;;
    compose) docker compose up --build -d "$@" ;;
    npm) npm run dev:stack:up "$@" ;;
    *) print -u2 "up: no Makefile up-build, compose file or dev:stack:up script here"; return 1 ;;
  esac
}

# down: stops the stack. -v deletes volumes (the data), so it asks first.
down() {
  local kind; kind=$(_stack_kind) || { print -u2 "down: nothing to stop here"; return 1; }
  if [[ " $* " == *" -v "* || " $* " == *" --volumes "* ]]; then
    local reply
    print -n "down: -v deletes this project's volumes (databases included). Continue? [y/N] "
    read -r reply || reply=
    [[ $reply == [yYsS]* ]] || { print "down: cancelled"; return 1; }
  fi
  case $kind in
    make) make down "$@" ;;
    compose) docker compose down "$@" ;;
    npm) npm run dev:stack:down "$@" ;;
  esac
}

# waitfor '<condition>': poll until the condition succeeds, then notify.
# Replaces `sleep 600` guesses. WAITFOR_INTERVAL seconds (default 20).
waitfor() {
  (( $# )) || { print -u2 "usage: waitfor '<shell condition>'"; return 2; }
  until eval "$1"; do sleep ${WAITFOR_INTERVAL:-20}; done
  [[ -n ${WAITFOR_QUIET:-} ]] && return 0
  print "waitfor: done — $1"
  command -v osascript >/dev/null && osascript -e "display notification \"$1\" with title \"waitfor: done\"" 2>/dev/null
  return 0
}

# wt: one worktree per task, with the repo's ignored .env files and its own
# port, so two dev servers never fight over 3000.
#   wt <name> [base]   create (or reuse) .worktrees/<name> on branch <name>, cd into it
#   wt ls              list worktrees
#   wt rm <name>       remove it (refuses if dirty); deletes the branch if merged
wt() {
  local root; root=$(git rev-parse --show-toplevel 2>/dev/null) || { print -u2 "wt: not in a git repo"; return 1; }
  # From inside a worktree, operate on the main checkout.
  local common; common=$(git -C "$root" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)
  [[ $common == */.git ]] && root=${common:h}
  case ${1:-} in
    ''|-h|--help) print "usage: wt <name> [base] | wt ls | wt rm <name>"; return 0 ;;
    ls) git -C "$root" worktree list; return ;;
    rm)
      [[ -n ${2:-} ]] || { print -u2 "usage: wt rm <name>"; return 2; }
      git -C "$root" worktree remove "$root/.worktrees/$2" || return
      git -C "$root" branch -d "$2" 2>/dev/null || print "wt: branch $2 kept (not merged)"
      return 0 ;;
  esac
  local name=$1 base=${2:-HEAD} dir=$root/.worktrees/$1
  if [[ ! -d $dir ]]; then
    if git -C "$root" show-ref -q "refs/heads/$name"; then
      git -C "$root" worktree add -q "$dir" "$name" || return
    else
      git -C "$root" worktree add -q -b "$name" "$dir" "$base" || return
    fi
    local f
    for f in $root/.env*(N.); do
      [[ ${f:t} == .env.example ]] && continue
      git -C "$root" check-ignore -q "$f" && cp "$f" "$dir/"
    done
  fi
  # Stable per name: 3100-3599, never the default 3000.
  local port=$(( 3100 + $(print -n -- "$name" | cksum | cut -d' ' -f1) % 500 ))
  if [[ ! -f $dir/.env.local || ${(M)${(f)"$(<$dir/.env.local)"}:#PORT=*} == "" ]]; then
    print "PORT=$port" >> "$dir/.env.local"
  fi
  print "wt: $name → $dir (PORT=$port)"
  cd "$dir"
}

# forrepos '<command>': run a command in every git repo under
# ${FORREPOS_ROOT:-~/Proyectos} (3 levels deep), output prefixed with the repo.
#   forrepos 'git status -sb'     forrepos 'git pull --ff-only'
forrepos() {
  (( $# )) || { print -u2 "usage: forrepos '<command>'"; return 2; }
  local root=${FORREPOS_ROOT:-$HOME/Proyectos} g repo rel failed=()
  for g in $root/{*,*/*,*/*/*}/.git(N/); do
    repo=${g:h}; rel=${repo#$root/}
    ( cd "$repo" && eval "$1" ) 2>&1 | while IFS= read -r line; do printf '%-28s %s\n' "$rel" "$line"; done
    (( ${pipestatus[1]} )) && failed+=$rel
  done
  (( $#failed )) && { print "forrepos: failed in ${#failed}: ${failed[*]}"; return 1 }
  return 0
}

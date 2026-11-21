# Shell helpers from the habits audit (2026-11-21). Sourced by home/zshrc.
# Each one replaces something typed hundreds of times; numbers are in
# tests/shell-helpers.test.sh.

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

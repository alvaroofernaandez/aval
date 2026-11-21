# Shared by the suites that exercise the Go binary (`aval hook ...`, `aval design ...`).
# build_aval <dir> prints the path of a freshly built aval in <dir>, or nothing
# when there is no Go toolchain or the build fails.
build_aval() {
  command -v go >/dev/null 2>&1 || return 0
  local repo; repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
  (cd "$repo" && go build -o "$1/aval" ./cmd/aval) >/dev/null 2>&1 && printf '%s' "$1/aval"
}

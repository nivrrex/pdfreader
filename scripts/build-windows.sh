#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly SCRIPT_DIR
tmp_dir=''

err() {
  printf '%s\n' "$*" >&2
}

cleanup() {
  if [[ -n "${tmp_dir}" ]]; then
    rm -rf -- "${tmp_dir}"
  fi
}

main() {
  if ! command -v zig >/dev/null 2>&1; then
    err '缺少 Zig；请安装 Zig 0.17.1 或验证兼容的版本。'
    return 1
  fi

  trap cleanup EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  tmp_dir="$(mktemp -d)"
  cat >"${tmp_dir}/zig-win-cc" <<'EOF'
#!/bin/sh
exec zig cc -target x86_64-windows-gnu "$@"
EOF
  chmod +x "${tmp_dir}/zig-win-cc"

  (
    cd "${SCRIPT_DIR}"
    GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC="${tmp_dir}/zig-win-cc" \
      go build -mod=readonly -trimpath -buildvcs=false \
      -ldflags="-s -w -linkmode external -extldflags '-Wl,--subsystem,windows'" \
      -o "${SCRIPT_DIR}/pdfreader_windows_amd64.exe" .
  )
  printf '已生成 %s\n' "${SCRIPT_DIR}/pdfreader_windows_amd64.exe"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi

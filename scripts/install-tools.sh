#!/usr/bin/env bash
set -euo pipefail

WGCF_VERSION="${WGCF_VERSION:-v2.2.31}"
WIREPROXY_VERSION="${WIREPROXY_VERSION:-v1.1.2}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

if ! command -v go >/dev/null 2>&1; then
  echo "未找到 Go。请先安装 Go 1.23 或更高版本。" >&2
  exit 1
fi

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

GOBIN="$workdir" go install "github.com/ViRb3/wgcf@${WGCF_VERSION}"
GOBIN="$workdir" go install "github.com/windtf/wireproxy/cmd/wireproxy@${WIREPROXY_VERSION}"

install_cmd=(install -m 0755)
if [[ ! -w "$INSTALL_DIR" ]]; then
  if ! command -v sudo >/dev/null 2>&1; then
    echo "${INSTALL_DIR} 不可写且未安装 sudo。请用 root 执行，或设置 INSTALL_DIR。" >&2
    exit 1
  fi
  install_cmd=(sudo install -m 0755)
  sudo install -d "$INSTALL_DIR"
else
  install -d "$INSTALL_DIR"
fi

"${install_cmd[@]}" "$workdir/wgcf" "$INSTALL_DIR/wgcf"
"${install_cmd[@]}" "$workdir/wireproxy" "$INSTALL_DIR/wireproxy"

echo "已安装："
"$INSTALL_DIR/wgcf" --version 2>/dev/null || true
"$INSTALL_DIR/wireproxy" --version 2>/dev/null || true

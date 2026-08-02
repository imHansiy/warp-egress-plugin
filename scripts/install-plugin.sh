#!/usr/bin/env bash
set -euo pipefail

PLUGIN_DIR="${1:-}"
SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/bin/warp-egress.so"

if [[ -z "$PLUGIN_DIR" ]]; then
  echo "用法：$0 /path/to/CLIProxyAPI/plugins" >&2
  exit 1
fi
if [[ ! -f "$SOURCE" ]]; then
  echo "未找到 $SOURCE，请先执行 make build。" >&2
  exit 1
fi

mkdir -p "$PLUGIN_DIR"
install -m 0755 "$SOURCE" "$PLUGIN_DIR/warp-egress.so"
echo "已安装到 $PLUGIN_DIR/warp-egress.so"

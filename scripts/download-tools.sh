#!/usr/bin/env bash
set -euo pipefail

# 下载 wgcf / wireproxy 官方预编译二进制，供 go:embed 打进插件。
# 产物写入 cmd/warp-egress/embedded_tools/（不入库，见 .gitignore）。
#
# 用法：
#   ./scripts/download-tools.sh                 # 默认 linux/amd64
#   GOOS_ARCH=linux_arm64 ./scripts/download-tools.sh
#
# 覆盖版本：
#   WGCF_VERSION=v2.2.31 WIREPROXY_VERSION=v1.1.2 ./scripts/download-tools.sh

WGCF_VERSION="${WGCF_VERSION:-v2.2.31}"
WIREPROXY_VERSION="${WIREPROXY_VERSION:-v1.1.2}"
GOOS_ARCH="${GOOS_ARCH:-linux_amd64}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/cmd/warp-egress/embedded_tools}"

mkdir -p "$OUT_DIR"

# 下载并校验（校验和来自各仓库 Release 的 checksums.txt）
download_checked() {
  local repo="$1" version="$2" asset="$3" checksum_name="$4" target="$5"
  local base="https://github.com/${repo}/releases/download/${version}"
  echo "==> ${asset}"
  curl -fsSL -o "${target}.tmp" "${base}/${asset}"
  local expect got
  expect="$(curl -fsSL "${base}/checksums.txt" | awk -v n="${checksum_name}" '$2 == n {print $1}')"
  got="$(sha256sum "${target}.tmp" | awk '{print $1}')"
  if [[ -z "${expect}" ]]; then
    echo "!! 未在 checksums.txt 中找到 ${checksum_name}，跳过校验" >&2
  elif [[ "${expect}" != "${got}" ]]; then
    echo "!! 校验失败 ${asset}: expect ${expect} got ${got}" >&2
    rm -f "${target}.tmp"
    exit 1
  fi
  mv "${target}.tmp" "${target}"
  chmod 0755 "${target}"
}

# wgcf：release 资产即单个可执行文件
download_checked \
  "ViRb3/wgcf" "${WGCF_VERSION}" \
  "wgcf_${WGCF_VERSION#v}_${GOOS_ARCH}" \
  "wgcf_${WGCF_VERSION#v}_${GOOS_ARCH}" \
  "${OUT_DIR}/wgcf"

# wireproxy：release 资产是 tar.gz，解压出 wireproxy 可执行文件
tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT
download_checked \
  "windtf/wireproxy" "${WIREPROXY_VERSION}" \
  "wireproxy_${GOOS_ARCH}.tar.gz" \
  "wireproxy_${GOOS_ARCH}.tar.gz" \
  "${tmpdir}/wireproxy.tar.gz"
tar -xzf "${tmpdir}/wireproxy.tar.gz" -C "${tmpdir}"
if [[ ! -x "${tmpdir}/wireproxy" ]]; then
  echo "!! tar.gz 解压后未找到可执行文件 wireproxy" >&2
  ls -la "${tmpdir}" >&2
  exit 1
fi
install -m 0755 "${tmpdir}/wireproxy" "${OUT_DIR}/wireproxy"

echo "==> 已就绪:"
ls -la "${OUT_DIR}"

# warp-egress 插件运行环境镜像
# 在官方 CLIProxyAPI 镜像基础上安装插件运行所需的外部工具：
#   - wgcf:     注册并生成 WARP WireGuard 配置
#   - wireproxy:把每个 WireGuard 配置暴露为独立 SOCKS5 端口
# 插件通过 exec.LookPath 在 PATH 中查找这两个可执行文件，因此必须放入 /usr/local/bin。
# 两个工具均为纯 Go 项目，用 CGO_ENABLED=0 静态构建，可在任意基础镜像上运行。

# ---- 阶段 1：下载 wgcf / wireproxy 官方预编译二进制 ----
# 两个项目均有 GitHub Release 二进制；wgcf 源码要求 Go 1.26，直接下载省去构建
FROM debian:bookworm-slim AS tools

ARG WGCF_VERSION=v2.2.31
ARG WIREPROXY_VERSION=v1.1.2

RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
 && curl -fsSL -o /tmp/wgcf "https://github.com/ViRb3/wgcf/releases/download/${WGCF_VERSION}/wgcf_${WGCF_VERSION#v}_linux_amd64" \
 && chmod +x /tmp/wgcf \
 && curl -fsSL -o /tmp/wireproxy.tar.gz "https://github.com/windtf/wireproxy/releases/download/${WIREPROXY_VERSION}/wireproxy_linux_amd64.tar.gz" \
 && tar -xzf /tmp/wireproxy.tar.gz -C /tmp \
 && ls -la /tmp/wireproxy*

# ---- 阶段 2：官方镜像 + 工具 ----
FROM eceasy/cli-proxy-api:latest

COPY --from=tools /tmp/wgcf /usr/local/bin/wgcf
COPY --from=tools /tmp/wireproxy /usr/local/bin/wireproxy

# wgcf/wireproxy 的版本子命令各不相同，这里只验证二进制存在且可执行
RUN test -x /usr/local/bin/wgcf && test -x /usr/local/bin/wireproxy

# CLIProxyAPI WARP Egress Plugin

为 CLIProxyAPI 提供可视化 WARP 出口管理、全局出口热切换和认证文件级代理分流。

## 已实现功能

- 管理菜单面板：`/v0/resource/plugins/warp-egress/panel`
- 面板内注册新的托管 WARP 配置
- 接入已有的外部 SOCKS5 出口
- 全局 WARP 出口无重启切换
- 定时自动轮换、故障自动转移和不同 IP 约束
- 按认证文件名、邮箱、标签、服务商或类型执行正则分流
- 按认证类型/服务商分流，例如 `codex`、`claude`、`gemini-cli`
- 单个认证文件即时切换
- 每个物理 JSON 认证文件绑定不同出口
- 出口 IP、Cloudflare Colo、WARP 状态和延迟检测
- 重复出口 IP 检测
- WARP 配置启动、停止、重新注册和删除
- 规则持久化、批量应用与执行结果明细

规则优先级：

```text
单个认证文件 > 正则表达式 > 认证类型/服务商 > 全局出口
```


## v0.2.0 面板重构

新版管理面板改为分区工作台，不再把所有配置挤在同一页：

- 侧边栏导航：总览、WARP 配置、路由规则、认证文件、自动切换、执行记录
- 管理密钥连接弹窗、连接状态和错误恢复
- 出口拓扑、健康概览、快速切换和全量检测
- 配置卡片、搜索筛选、危险操作二次确认和加载状态
- 规则未保存提示、正则即时校验、保存与应用分离
- 认证文件搜索、类型/规则筛选、多选和批量出口绑定
- 自动轮换状态摘要、操作通知和当前标签页执行记录
- 白色与海蓝色视觉主题，响应式适配桌面和窄屏

## 工作方式

```text
CLIProxyAPI 全局 proxy-url
        │
        ▼
127.0.0.1:40000（插件固定 SOCKS5 中继）
        │
        └── 当前选中的 WARP 配置

认证文件 A proxy_url ──> WARP 配置 1 的独立 SOCKS5 端口
认证文件 B proxy_url ──> WARP 配置 2 的独立 SOCKS5 端口
认证文件 C 无 proxy_url ──> 继承 CLIProxyAPI 全局中继
```

全局切换只改变固定中继的后端，不需要重启 CLIProxyAPI。认证文件级规则通过 CLIProxyAPI 的 `host.auth.get` 和 `host.auth.save` 回调，只修改认证 JSON 的 `proxy_url` 字段。

## 依赖

- 支持动态插件的 CLIProxyAPI 版本
- Linux x86-64：可直接使用 `bin/warp-egress.so`
- `wgcf`：注册并生成 WARP WireGuard 配置
- `wireproxy`：把每个 WireGuard 配置暴露为独立 SOCKS5 端口
- 管理 API 必须配置 `remote-management.secret-key`

`wgcf` 和 `wireproxy` 均为第三方开源项目，不是 Cloudflare 官方 WARP 客户端。本项目不会打包或分发这两个二进制文件。

## 快速安装

### 1. 安装 wgcf 和 wireproxy

服务器已经安装 Go 时：

```bash
cd warp-egress-plugin
./scripts/install-tools.sh
```

默认安装版本：

- wgcf `v2.2.31`
- wireproxy `v1.1.2`

也可以覆盖：

```bash
WGCF_VERSION=v2.2.31 \
WIREPROXY_VERSION=v1.1.2 \
INSTALL_DIR=$HOME/.local/bin \
./scripts/install-tools.sh
```

### 2. 安装插件

把预编译文件复制到 CLIProxyAPI 的插件目录：

```bash
mkdir -p /path/to/CLIProxyAPI/plugins
cp bin/warp-egress.so /path/to/CLIProxyAPI/plugins/warp-egress.so
chmod 755 /path/to/CLIProxyAPI/plugins/warp-egress.so
```

或：

```bash
./scripts/install-plugin.sh /path/to/CLIProxyAPI/plugins
```

### 3. 修改 CLIProxyAPI 配置

把 `config.example.yaml` 中的配置合并到现有 `config.yaml`。核心配置如下：

```yaml
proxy-url: "socks5://127.0.0.1:40000"

remote-management:
  allow-remote: false
  secret-key: "替换为高强度管理密钥"

plugins:
  enabled: true
  dir: "plugins"
  configs:
    warp-egress:
      enabled: true
      priority: 10
      data-dir: "./warp-egress-data"
      wgcf-path: "wgcf"
      wireproxy-path: "wireproxy"
      listen-host: "127.0.0.1"
      global-port: 40000
      profile-port-start: 41000
      profile-port-end: 41999
      auto-start: true
      health-check-interval: "60s"
      ip-check-url: "https://www.cloudflare.com/cdn-cgi/trace"
      allow-remote-listen: false
```

`proxy-url` 必须和插件的 `listen-host + global-port` 一致，否则全局出口切换只会改变插件中继，不会影响 CLIProxyAPI 请求。

### 4. 重启并打开面板

```text
http://你的CLIProxyAPI地址:8317/v0/resource/plugins/warp-egress/panel
```

资源面板本身不携带管理权限。打开后输入 `remote-management.secret-key`，面板将密钥仅保存到当前标签页的 `sessionStorage`，随后调用受保护的 `/v0/management/warp-egress/*` 接口。

## 面板使用顺序

1. 进入“WARP 配置”，点击“新增配置”，创建至少一个托管 WARP 配置。
2. 等待出口检测显示 `warp=on` 或 `warp=plus`。
3. 把一个配置设为全局出口。
4. 按需启用定时轮换或故障自动切换。
5. 添加类型规则和正则规则。
6. 在认证文件表中，为特殊账号选择独立出口。
7. 在“路由规则”页面点击“保存并应用”。

## 正则目标字段

正则规则可以匹配：

- `name`：认证文件名
- `email`：认证邮箱
- `label`：认证标签
- `provider`：服务商
- `type`：认证类型
- `all`：以上字段合并匹配

示例：

```text
目标：email
表达式：@example\.com$
出口：warp-singapore-02
```

## 管理 API

```text
GET  /v0/management/warp-egress/status
GET  /v0/management/warp-egress/profiles
POST /v0/management/warp-egress/profiles/create
POST /v0/management/warp-egress/profiles/action
POST /v0/management/warp-egress/profiles/delete
POST /v0/management/warp-egress/global/switch
GET  /v0/management/warp-egress/auth-files
POST /v0/management/warp-egress/auth-files/assign
GET  /v0/management/warp-egress/rules
POST /v0/management/warp-egress/rules/save
POST /v0/management/warp-egress/rules/apply
GET  /v0/management/warp-egress/auto
POST /v0/management/warp-egress/auto/save
POST /v0/management/warp-egress/auto/run
```

认证方式：

```http
Authorization: Bearer <remote-management.secret-key>
```

## 从源码构建

```bash
make test
make build
```

输出：

```text
bin/warp-egress.so
bin/warp-egress.h
```

交叉构建 C 共享库需要目标平台的 C 编译器。仓库附带的二进制为 Linux x86-64、glibc 动态链接版本。


## Plugin Store registry

仓库内提供三份 CLIProxyAPI Plugin Store schema v1 文件：

- `registry.json`：中英双语默认版本
- `registry.zh-CN.json`：纯中文版本
- `registry.en.json`：纯英文版本

CLIProxyAPI 当前 registry schema 没有单独的多语言字段，因此官方商店提交时通常只提交一个条目。发布前必须把三份文件中的：

```text
https://github.com/OWNER/warp-egress
```

替换为真实公开仓库地址。校验命令：

```bash
make registry-check
```

构建官方格式的 Linux AMD64 Release 资产：

```bash
make release-linux-amd64
```

输出：

```text
dist/warp-egress_0.2.0_linux_amd64.zip
dist/checksums.txt
```

Release 标签应使用 `v0.2.0`，压缩包根目录直接包含 `warp-egress.so`。详见 `REGISTRY.md`。

## 数据目录

```text
warp-egress-data/
├── state.json
└── profiles/
    └── warp-xxxxxxxxxxxx/
        ├── wgcf-account.toml
        ├── wgcf-profile.conf
        ├── wireproxy.conf
        └── wireproxy.log
```

注册资料包含私钥，目录权限默认为 `0700`，状态和配置文件默认为 `0600`。不要把该目录提交到 Git。

## 限制

- WARP 配置数量不等于唯一出口 IP 数量。多个配置可能得到同一个共享出口，面板会标记重复 IP。
- 重新注册配置后，出口可能变化，也可能仍然相同。
- 单文件分流仅适用于有物理 `.json` 文件的认证；运行时虚拟认证会被跳过。
- 配置在主 YAML 中的静态 API Key 无独立认证文件，只能使用全局中继。
- 当前外部出口只支持无认证的 `socks5://` 和 `socks5h://`。
- 插件不会按每次请求重新注册 WARP；这样可避免中断流式请求和高频创建设备。
- 本项目尚未在所有 CLIProxyAPI 发行版、Linux 发行版和 CPU 架构上做集成测试。源码单元测试和 ABI 加载测试通过，但首次部署应先备份认证目录。

## 安全建议

- 保持 `listen-host: 127.0.0.1`。
- 保持 `remote-management.allow-remote: false`，或在反向代理层增加额外认证。
- 先备份 CLIProxyAPI 的 `auth-dir`，再首次执行“保存并应用”。
- 不要对外开放 `40000` 和 `41000-41999` 端口。
- 使用系统服务管理 CLIProxyAPI，避免插件被强制终止时留下不完整操作。

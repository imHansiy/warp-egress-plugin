# CLIProxyAPI WARP Egress Plugin

为 CLIProxyAPI 提供可视化 WARP 出口管理、全局出口热切换和认证文件级代理分流。

## 已实现功能

- 管理菜单面板：`/v0/resource/plugins/warp-egress/panel`
- 面板内注册新的托管 WARP 配置
- 接入已有的外部 SOCKS5 出口
- 通过已有出口注册新 WARP（`register_via`），绕开本机 IP 的注册限流
- 全局 WARP 出口无重启切换
- 定时自动轮换、故障自动转移和不同 IP 约束
- 按认证文件名、邮箱、标签、服务商或类型执行正则分流（含常用正则模板）
- 按认证类型/服务商分流，类型以下拉选择（仅列 CPA 支持的认证类型）
- 单文件出口三态：继承全局 / 不设置代理 / 自定义代理（与 CPA 认证文件 `proxy_url` 字段实时联动）
- 单个认证文件即时切换
- 每个物理 JSON 认证文件绑定不同出口
- 出口 IP、Cloudflare Colo、WARP 状态和延迟检测
- 重复出口 IP 检测
- WARP 配置启动、停止、重新注册和删除
- 规则持久化、批量应用与执行结果明细
- 注册限流（429）友好提示，附等待与替代方案

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
认证文件 D "不设置代理" ──> 清除 proxy_url，跟随全局，不被其他规则接管
认证文件 E 自定义代理 ──> 使用 CPA 认证文件中已有的代理地址（与 CPA 面板联动）
```

全局切换只改变固定中继的后端，不需要重启 CLIProxyAPI。认证文件级规则通过 CLIProxyAPI 的 `host.auth.get` 和 `host.auth.save` 回调，只修改认证 JSON 的 `proxy_url` 字段——与 CPA 自带的“认证文件代理”配置是同一个字段，两处完全同步。

## 依赖

- 支持动态插件的 CLIProxyAPI 版本
- Linux x86-64：可直接使用 `bin/warp-egress.so`
- `wgcf` 和 `wireproxy` 已内置进插件，无需单独安装；找不到时自动解压到 data-dir/bin
- 管理 API 必须配置 `remote-management.secret-key`

`wgcf` 和 `wireproxy` 均为第三方开源项目，不是 Cloudflare 官方 WARP 客户端。本项目不会直接分发这两个二进制文件，构建时从各自 GitHub Release 下载并内嵌。

## 快速安装

### 1. 安装插件

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

插件已内置 wgcf 和 wireproxy，无需在服务器上单独安装；首次启动自动解压到 data-dir/bin。如需使用系统安装的版本，可在插件配置中指定 `wgcf-path` / `wireproxy-path`。

### 2. 修改 CLIProxyAPI 配置

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
      listen-host: "127.0.0.1"
      global-port: 40000
      profile-port-start: 41000
      profile-port-end: 41999
      auto-start: true
      health-check-interval: "60s"
      ip-check-url: "https://www.cloudflare.com/cdn-cgi/trace"
      allow-remote-listen: false
```

`wgcf-path` / `wireproxy-path` 已省略：插件内置这两个工具，找不到时自动解压到 data-dir/bin。如需覆盖（如使用系统安装的特定版本），可添加对应字段。

`proxy-url` 必须和插件的 `listen-host + global-port` 一致，否则全局出口切换只会改变插件中继，不会影响 CLIProxyAPI 请求。

### 3. 重启并打开面板

```text
http://你的CLIProxyAPI地址:8317/v0/resource/plugins/warp-egress/panel
```

资源面板本身不携带管理权限。打开后输入 `remote-management.secret-key`，面板将密钥仅保存到当前标签页的 `sessionStorage`，随后调用受保护的 `/v0/management/warp-egress/*` 接口。

## 面板使用顺序

1. 进入“WARP 配置”，点击“新增配置”，创建至少一个托管 WARP 配置。
2. 等待出口检测显示 `warp=on` 或 `warp=plus`。
3. 把一个配置设为全局出口。
4. 按需启用定时轮换或故障自动切换。
5. 添加类型规则（认证类型从下拉选择）和正则规则（可从常用模板选择）。
6. 在认证文件表中，为特殊账号选择独立出口（继承全局 / 不设置代理 / 自定义代理 / 具体出口）。
7. 在“路由规则”页面点击“保存并应用”。

## 注册限流（429）与规避

Cloudflare 对 WARP 注册接口（`api.cloudflareclient.com`）按来源 IP 临时限流（`429 Too Many Requests`，窗口约 15 分钟）。数据中心共享 IP（云开发环境、VPS 机房）最容易触发。插件已内置中文 429 提示，告知等待时间与替代方案。

绕开限流的方式：

1. **通过已有出口注册**：面板“新增配置”中选择“通过已有出口注册”，wgcf 注册请求会经该出口的 SOCKS5 发出（内置 HTTP CONNECT 桥，纯标准库实现）。注意：WARP 出口 IP 也是共享的，Cloudflare 同样可能限流；使用**干净独立 IP 的 SOCKS5**（如家宽/住宅代理）效果最佳。API 方式：`register_via` 传出口 ID 或 `socks5://` 地址。
2. **导入已有配置**：在**不受限流的网络环境**（家宽、手机流量、住宅代理）生成 `wgcf-profile.conf`，再导入插件，完全不触发注册。完整步骤见下文「如何用 register 生成配置文件并导入」。
3. **等待窗口**：429 为临时限流，间隔 15-30 分钟重试通常可成功。

## 如何用 register 生成配置文件并导入

适用场景：服务器/云环境注册 WARP 被 429 限流时，在干净网络环境手动注册一次，把生成的配置导入插件使用。

### 第 1 步：在干净网络环境注册

任意一台**不受限流的机器**（自己电脑、手机、家宽网络）上：

1. 下载 wgcf（Windows/macOS/Linux 均有预编译二进制）：

   ```bash
   # Linux x86-64 示例，其他平台见 https://github.com/ViRb3/wgcf/releases
   wget https://github.com/ViRb3/wgcf/releases/download/v2.2.31/wgcf_2.2.31_linux_amd64 -O wgcf
   chmod +x wgcf
   ```

2. 注册 WARP 账号并生成配置：

   ```bash
   ./wgcf register --accept-tos   # 生成 wgcf-account.toml（含私钥，勿泄露）
   ./wgcf generate                # 生成 wgcf-profile.conf
   ```

   也可以使用浏览器一键生成器（warpper.me、itsyebekhe.github.io/warp 等），下载的 `warp.conf` 就是同一个格式。

3. 查看生成的配置文件内容：

   ```bash
   cat wgcf-profile.conf
   ```

   内容形如：

   ```text
   [Interface]
   PrivateKey = 6Dk6Z4...=
   Address = 172.16.0.2/32
   DNS = 1.1.1.1, 1.0.0.1
   MTU = 1280

   [Peer]
   PublicKey = bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=
   AllowedIPs = 0.0.0.0/0, ::/0
   Endpoint = engage.cloudflareclient.com:2408
   ```

### 第 2 步：导入插件

**方式 A：管理 API（推荐，可脚本化）**

把 `wgcf-profile.conf` 的内容作为 JSON 字符串传入：

```bash
CONF=$(cat wgcf-profile.conf)
curl -X POST http://你的CLIProxyAPI地址:8317/v0/management/warp-egress/profiles/import \
  -H "Authorization: Bearer <remote-management.secret-key>" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"家用网络注册的出口\",\"wgcf_profile\":\"$CONF\"}"
```

导入成功后自动分配独立 SOCKS5 端口并启动，面板“WARP 配置”中即可看到、设为全局出口。

**方式 B：手动放置文件**

如果无法调用 API，可先随便创建一个托管出口（会失败但会生成目录），或直接创建目录：

```bash
mkdir -p /path/to/CLIProxyAPI/warp-egress-data/profiles/<出口ID>
cp wgcf-profile.conf /path/to/CLIProxyAPI/warp-egress-data/profiles/<出口ID>/
chmod 600 /path/to/CLIProxyAPI/warp-egress-data/profiles/<出口ID>/wgcf-profile.conf
```

之后在面板对该出口执行「启动」，插件检测到目录中已有 `wgcf-profile.conf` 会跳过注册直接使用（需先在面板创建同名出口以得到目录与端口）。

> 注意：导入的配置属于独立 WARP 账号，与插件面板注册的账号互不影响；私钥仅存在于该配置文件中，妥善保管。

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
POST /v0/management/warp-egress/profiles/import
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

关键参数：

- `POST /profiles/create`：托管模式可传 `register_via`（已有托管出口 ID 或 `socks5://` 地址），注册请求将经该出口发出，用于绕开 429 限流。
- `POST /profiles/import`：`{name, wgcf_profile}`，直接导入 wgcf-profile.conf 内容创建出口，不触发注册。
- `POST /auth-files/assign`：`{auth_index, profile_id | proxy_url, apply_now}`。`profile_id` 传空字符串清除出口；传 `direct` 表示“不设置代理”（锁定清除，不被其他规则接管）；传 `proxy_url` 字段可写入任意自定义代理（与 CPA 认证文件 `proxy_url` 同一字段）。

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
- 免费版 WARP 的 IPv4 出口是全局共享的（如 `104.28.222.43`），区分出口以 IPv6 为准。
- 注册接口按来源 IP 限流（429）：共享出口（数据中心 IP、WARP 出口 IP）均可能触发，使用干净独立 IP 注册最稳定。
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

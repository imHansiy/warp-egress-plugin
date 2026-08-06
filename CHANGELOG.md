# Changelog

## 0.5.0

- 新增 **系统代理**（设置 → 系统代理，独立开关）：把当前全局出口应用到系统，
  系统其他进程（AI 服务等）经本地 HTTP 桥（默认 40001）→ 插件中继 → 当前全局出口。
- 平台接入：Linux GNOME 桌面（gsettings 系统设置面板同款，即时生效）、
  macOS（networksetup）、Windows（注册表 + InternetSetOption 广播，
  已运行应用立即感知）；无桌面服务器回退写入 /etc/profile.d 环境变量。
- 关闭即清理：停止桥 + 移除系统设置/环境文件；重启自动恢复。
- 修复 Manager.Shutdown 双重 Unlock 崩溃（插件卸载 panic）。

## 0.4.0

- 插件配置随 CPA 配置体系持久化：config.yaml 插件段新增 `state-json` 权威源
  （配合 CPA 外部数据库/Postgres、对象存储、Git 等集中管理，多节点统一）。
- 面板保存全自动同步：设置/拓展功能/路由规则/单文件分配的保存自动 PATCH 到
  CPA 管理 API（使用连接时输入的管理密钥）→ 写回配置文件并入库 → 自动重载生效。
- 面板自动读取 CPA 管理面板「记住密码」的管理密钥，无需手动输入。
- 新增「导出配置段」按钮（手动同步场景）。

## 0.3.0

- 新增 **xAI 降智守护**（拓展功能）：仅针对 xAI / Grok 输出降智检测。
  - 被动 TPS 检测（usage 事件，仅统计 xAI 请求）+ 流式补偿（chunk 拦截器，覆盖 CPA 流式 usage 盲区）。
  - 连续高输出 TPS 给出口打降智标记：分流自动跳过、当前全局出口自动切换。
  - 开启时所有 XAI 认证文件自动绑定健康托管出口参与检测，关闭自动解绑。
  - 自动补充出口（经健康出口注册防限流、429 退避、首次开启立即判断）。
  - 自动清理降智代理（全部清理，被规则引用保留）。
  - 可选主动探测（复用 CPA 内 xAI 账号实测 TPS，401 自动换账号）。
- 新增 **设置** 入口（分类标签页，可扩展）：自动切换 + 通用「自动清理不健康出口」
  （连通失败且未被规则引用的托管出口自动删除，含新出口保护期与持续异常时长配置）。
- 全局出口支持「不使用代理」；自动切换与降智检测均尊重该选择。
- 中继无已选出口时回退直连，不再阻断 CPA 管理请求（配额查询等）。
- 面板：出口质量列、自动补充状态条、设置/拓展功能标签页弹窗。
- 出口表格列调整：名称、状态、质量、公网 IP、国家、延迟、本地代理、操作；节点代码（colo）
  显示为可读的国家/地区名（SIN→新加坡等）。
- 出口 IP 双栈显示：健康检查同时检测 IPv4 与 IPv6 出口地址，当前出口卡片与表格优先展示 IPv4。
- 采用全新单页出口管理 UI：当前出口、出口表格和分流摘要；类型/正则规则与认证文件绑定
  收进按需打开的侧边抽屉；单文件出口选择后立即写入并应用。

## 0.2.0

- Rebuilt the management panel as a sidebar-based workspace.
- Added overview metrics, traffic topology, health summaries, and quick actions.
- Added profile cards with search, filtering, loading feedback, and destructive-action confirmation.
- Added unsaved-rule detection, regex validation, and separate save/apply actions.
- Added auth-file search, provider/rule filters, multi-selection, and bulk egress assignment.
- Added connection management, progress indicators, toast notifications, and in-tab activity history.
- Added responsive desktop/mobile layouts using the white and ocean-blue visual system.
- Added bilingual, Chinese, and English CLIProxyAPI registry files.
- Added registry validation and official Linux AMD64 release packaging targets.

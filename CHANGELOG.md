# Changelog

## 0.8.0

- 补齐 xAI 出口守护状态机：新增 `hard_tps` 单次立即隔离、`consecutive_errors` 探测错误累计，以及 `min_healthy` 隔离抑制保护。
- 统一 xAI Token 口径：completion/output 已包含 reasoning 时不重复相加，仅有 reasoning 明细时仍可用于 TPS 证据；`total_tokens` 不进入分子。
- 面板升级到质量策略 schema 4，显示软/硬阈值、探测错误次数与 `suppressed` / 硬隔离状态；旧策略自动补齐默认值。
- 账号过期、额度耗尽与暂无可调度探针账号继续只记诊断，不会累计为出口错误。
- 修复流式 xAI 请求的误报：usage 未提供 thinking 字段时按“未知”处理；结束帧中的 reasoning token 可以纠正早到但字段不完整的结论，不再被请求去重丢弃。
- 交叉验证改为有界任务：等待探测槽位也计入总超时，超时后退出 `verifying`、保留当前出口，并允许完整流式证据取消无效复核。
- 插件重载时会清理上次遗留的 `verifying` 状态，避免中断的交叉验证让出口长期卡在待确认状态。

## 0.7.0

- 将 xAI 降智守护重构为默认关闭、可独立移除的「xAI 出口守护扩展」，核心 SOCKS 中继只保留通用目标路由接口，不再绑定 xAI / Grok 业务判断。
- 新增 xAI 独立出口模式：普通全局代理关闭时，非 xAI 请求保持直连，xAI 目标仍只走健康代理；没有健康出口时拒绝连接，避免静默泄漏到服务器直连。
- xAI 请求不再批量改写成千上万个认证文件的 `proxy_url`；插件在固定本地中继内按目标域名选路，普通全局出口与 xAI 活动出口互不覆盖。
- 增强降智识别与复核：支持 thinking / reasoning 字段、TPS 与 thinking 分离计数、同出口去重交叉验证、探针账号短缓存以及账号额度/认证异常与出口网络异常分离。
- 探测遇到超时、EOF、连接重置或 TLS/HTTP2 中断时立即隔离对应 xAI 出口；401/403、额度耗尽和 429 则切换探针账号，不误伤出口。

## 0.6.0

- 重构「拓展功能 → xAI 降智守护」的出口维护逻辑，其他模块保持原有行为：
  - 移除 xAI 守护对全部认证文件的自动 `proxy_url` 绑定/解绑；xAI 账号统一继承 CPA 全局中继。
  - 主动探测只读取 CPA 标记为可用、未禁用、未冷却的少量 xAI 账号，不保存或改写认证文件；账号列表缓存 5 分钟，避免每个出口重复扫描大账号目录。
  - 备用出口按最久未测顺序错峰探测，每轮一个、全局串行，并可配置同一出口最短复检间隔。
  - 当前全局出口降智时，只切换到近期 xAI 实测健康的备用出口；过期候选先复检再接管。
  - 账号侧 401/403/402/429 自动尝试下一个健康账号，不把账号或配额问题误判为出口质量问题。

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

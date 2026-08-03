# Changelog

## 0.3.1

- 出口 IP 双栈显示：健康检查同时检测 IPv4 与 IPv6 出口地址。
- 当前出口卡片与出口表格优先展示 IPv4，IPv6 作为补充行展示。
- 业务逻辑不变。

## 0.3.0

- 采用全新单页出口管理 UI（GPT 协作设计）：当前出口、出口表格和分流摘要。
- 类型规则、正则规则与认证文件绑定收进按需打开的侧边抽屉。
- 自动切换收进独立弹窗，减少主页面常驻配置。
- 单文件出口选择后立即写入并应用。
- 优化嵌入 CLIProxyAPI 管理后台时的宽度、留白和视觉层级。
- 业务逻辑保持 0.2.7 不变（含导入预生成 WARP 配置能力）。
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

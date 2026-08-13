package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"warp-egress-plugin/cmd/warp-egress/version"
)

const (
	pluginID      = "warp-egress"
	schemaVersion = 1
	abiVersion    = 1
)

// pluginVersion 为插件版本号，构建时经 ldflags -X 注入
// （注入目标是子包 version.Version，见 Makefile / CI）；未注入时显示 dev。
var pluginVersion = version.Version

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type lifecycleRequest struct {
	ConfigYAML string `json:"config_yaml"`
}

type configField struct {
	Name        string   `json:"Name"`
	Type        string   `json:"Type"`
	EnumValues  []string `json:"EnumValues,omitempty"`
	Description string   `json:"Description"`
}

type pluginMetadata struct {
	Name             string        `json:"Name"`
	Version          string        `json:"Version"`
	Author           string        `json:"Author"`
	GitHubRepository string        `json:"GitHubRepository"`
	Logo             string        `json:"Logo"`
	ConfigFields     []configField `json:"ConfigFields"`
}

type registrationCapabilities struct {
	ManagementAPI             bool `json:"management_api"`
	UsagePlugin               bool `json:"usage_plugin"`
	RequestInterceptor        bool `json:"request_interceptor"`
	ResponseStreamInterceptor bool `json:"response_stream_interceptor"`
}

type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      pluginMetadata           `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}

type managementRoute struct {
	Method      string `json:"Method"`
	Path        string `json:"Path"`
	Menu        string `json:"Menu,omitempty"`
	Description string `json:"Description,omitempty"`
}

type resourceRoute struct {
	Path        string `json:"Path"`
	Menu        string `json:"Menu,omitempty"`
	Description string `json:"Description,omitempty"`
}

type managementRegistration struct {
	Routes    []managementRoute `json:"routes,omitempty"`
	Resources []resourceRoute   `json:"resources,omitempty"`
}

type managementRequest struct {
	Method         string      `json:"Method"`
	Path           string      `json:"Path"`
	Headers        http.Header `json:"Headers"`
	Query          url.Values  `json:"Query"`
	Body           []byte      `json:"Body"`
	HostCallbackID string      `json:"host_callback_id,omitempty"`
}

type managementResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

type hostAuthFileEntry struct {
	ID             string            `json:"id"`
	AuthIndex      string            `json:"auth_index"`
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	Provider       string            `json:"provider"`
	Label          string            `json:"label"`
	Email          string            `json:"email"`
	Status         string            `json:"status"`
	StatusMessage  string            `json:"status_message"`
	Disabled       bool              `json:"disabled"`
	Unavailable    bool              `json:"unavailable"`
	RuntimeOnly    bool              `json:"runtime_only"`
	NextRetryAfter time.Time         `json:"next_retry_after,omitempty"`
	ProxyURL       string            `json:"proxy_url"`
	Attributes     map[string]string `json:"attributes,omitempty"`
}

type hostAuthListResponse struct {
	Files []hostAuthFileEntry `json:"files"`
}

type hostAuthGetRequest struct {
	AuthIndex string `json:"auth_index"`
}

type hostAuthGetResponse struct {
	Name      string          `json:"name"`
	AuthIndex string          `json:"auth_index"`
	JSON      json.RawMessage `json:"json"`
}

type hostAuthSaveRequest struct {
	Name string          `json:"name"`
	JSON json.RawMessage `json:"json"`
}

type hostAuthSaveResponse struct {
	Status string `json:"status"`
	Name   string `json:"name,omitempty"`
}

type ProfileMode string

const (
	ProfileModeManaged  ProfileMode = "managed"
	ProfileModeExternal ProfileMode = "external"
)

type Profile struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Mode        ProfileMode `json:"mode"`
	ProxyURL    string      `json:"proxy_url"`
	ListenHost  string      `json:"listen_host,omitempty"`
	ListenPort  int         `json:"listen_port,omitempty"`
	Directory   string      `json:"directory,omitempty"`
	PID         int         `json:"pid,omitempty"`
	Running     bool        `json:"running"`
	Healthy     bool        `json:"healthy"`
	ExitIP      string      `json:"exit_ip,omitempty"`
	ExitIPV4    string      `json:"exit_ip_v4,omitempty"`
	ExitIPV6    string      `json:"exit_ip_v6,omitempty"`
	Colo        string      `json:"colo,omitempty"`
	WarpMode    string      `json:"warp_mode,omitempty"`
	LatencyMS   int64       `json:"latency_ms,omitempty"`
	LastChecked time.Time   `json:"last_checked,omitempty"`
	LastError   string      `json:"last_error,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	// xAI 降智守护：出口被观测/探测到输出 TPS 异常（降智）时标记，
	// 路由分流与自动切换会跳过被标记的出口。
	Degraded       bool      `json:"degraded,omitempty"`
	DegradedReason string    `json:"degraded_reason,omitempty"`
	DegradedAt     time.Time `json:"degraded_at,omitempty"`
	QualityTPS     float64   `json:"quality_tps,omitempty"`
	QualityStrikes int       `json:"quality_strikes,omitempty"`
	// QualityErrorStrikes 只累计能够归因到出口或探测链路的失败；账号过期、
	// 额度耗尽和暂无可调度账号只展示错误，不消耗这个计数。
	QualityErrorStrikes int `json:"quality_error_strikes,omitempty"`
	// QualityThinkingStrikes 单独累计“有足够输出但缺少 thinking”样本，
	// 避免与 TPS 异常计数混在一起后由不同质量信号误触发隔离。
	QualityThinkingStrikes int       `json:"quality_thinking_strikes,omitempty"`
	QualityRecovery        int       `json:"quality_recovery,omitempty"`
	QualityCheckedAt       time.Time `json:"quality_checked_at,omitempty"`
	// QualityClassification / QualitySource 记录最近一次 xAI 质量结论及来源，
	// 让全局降智切换只选择近期主动探测或真实请求确认健康的备用出口。
	QualityClassification string `json:"quality_classification,omitempty"`
	QualitySource         string `json:"quality_source,omitempty"`
	QualityError          string `json:"quality_error,omitempty"`
	// Origin 出口来源：空为手动创建，"auto" 为自动补充创建。
	Origin string `json:"origin,omitempty"`
}

type TypeRule struct {
	Key       string `json:"key"`
	ProfileID string `json:"profile_id"`
	Enabled   bool   `json:"enabled"`
}

type RegexRule struct {
	ID        string `json:"id"`
	Pattern   string `json:"pattern"`
	Target    string `json:"target"`
	ProfileID string `json:"profile_id"`
	Enabled   bool   `json:"enabled"`
}

type Rules struct {
	GlobalProfileID string            `json:"global_profile_id"`
	TypeRules       []TypeRule        `json:"type_rules"`
	RegexRules      []RegexRule       `json:"regex_rules"`
	ExactRules      map[string]string `json:"exact_rules"`
}

type AutoSwitchConfig struct {
	Enabled               bool      `json:"enabled"`
	FailoverEnabled       bool      `json:"failover_enabled"`
	RotateIntervalSeconds int       `json:"rotate_interval_seconds"`
	RequireDifferentIP    bool      `json:"require_different_ip"`
	LastSwitchAt          time.Time `json:"last_switch_at,omitempty"`
	LastProfileID         string    `json:"last_profile_id,omitempty"`
	LastReason            string    `json:"last_reason,omitempty"`
}

// QualityProbeConfig 主动质量探测：新出口/恢复出口先经该出口，复用 CPA 内
// 的 xAI 账号向 xAI 端点发流式请求，实测输出 TPS。检测对象与降智特征一致
// （xAI 输出 TPS 异常飙升=共享出口被打穿），无需额外配置 API Key。
type QualityProbeConfig struct {
	Enabled         bool   `json:"enabled"`
	Model           string `json:"model"`
	MaxTokens       int    `json:"max_tokens"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	IntervalMinutes int    `json:"interval_minutes"`
}

const (
	XAIRouteModeIndependent  = "independent"
	XAIRouteModeFollowGlobal = "follow_global"
	XAIRouteModeDirect       = "direct"
	qualityThinkingSchema    = 2
	qualityProbeModelSchema  = 3
	qualityPolicySchema      = 4
)

// XAIRouteConfig 只决定 xAI 请求通过插件本地中继后的出口。
// independent 与普通 GlobalProfileID 完全分离；无可用出口时拒绝连接，
// 以保证“xAI 只走代理”不会在异常时静默退回服务器直连。
type XAIRouteConfig struct {
	Mode            string   `json:"mode"`
	ActiveProfileID string   `json:"active_profile_id,omitempty"`
	Hosts           []string `json:"hosts,omitempty"`
}

// QualityConfig xAI 降智守护策略（仅针对 xAI / Grok 输出降智）：
// 被动观测 CPA usage 事件中的输出 token 数与耗时，估算输出 TPS；
// TPS 异常高说明出口共享 IP 被打穿（对 AI 生成表现为"降智"），
// 连续多次则给该出口打降智标记，路由分流自动跳过。
type QualityConfig struct {
	PolicySchema int `json:"policy_schema,omitempty"`
	// Enabled 是整个 xAI 出口守护扩展的总开关，不只是检测开关。
	// false 时域名路由、主动探测、自动补充/清理和独立切换均不接管核心行为。
	Enabled                    bool               `json:"enabled"`
	SoftTPS                    float64            `json:"soft_tps"`
	HardTPS                    float64            `json:"hard_tps"`
	ConsecutiveDegraded        int                `json:"consecutive_degraded"`
	ConsecutiveErrors          int                `json:"consecutive_errors"`
	ThinkingGuard              bool               `json:"thinking_guard"`
	ConsecutiveMissingThinking int                `json:"consecutive_missing_thinking"`
	ThinkingCrossVerify        bool               `json:"thinking_cross_verify"`
	SoftCrossVerify            bool               `json:"soft_cross_verify"`
	RecoveryObservations       int                `json:"recovery_observations"`
	MinGenerationMs            int64              `json:"min_generation_ms"`
	MinOutputTokens            int64              `json:"min_output_tokens"`
	AutoProvision              bool               `json:"auto_provision"`
	AutoPrune                  bool               `json:"auto_prune"`
	MinHealthy                 int                `json:"min_healthy"`
	MaxProfiles                int                `json:"max_profiles"`
	ProvisionCooldownMin       int                `json:"provision_cooldown_minutes"`
	Probe                      QualityProbeConfig `json:"probe"`
	Route                      XAIRouteConfig     `json:"route"`
}

func defaultQualityConfig() QualityConfig {
	return QualityConfig{
		PolicySchema: qualityPolicySchema,
		// xAI 守护是可选拓展，新安装默认关闭；开启后才注册目标接管、
		// 主动探测和独立出口切换行为。
		Enabled:                    false,
		SoftTPS:                    500,
		HardTPS:                    1000,
		ConsecutiveDegraded:        3,
		ConsecutiveErrors:          3,
		ThinkingGuard:              true,
		ConsecutiveMissingThinking: 1,
		ThinkingCrossVerify:        true,
		SoftCrossVerify:            true,
		RecoveryObservations:       2,
		MinGenerationMs:            1000,
		MinOutputTokens:            32,
		AutoProvision:              true,
		AutoPrune:                  true,
		MinHealthy:                 2,
		MaxProfiles:                8,
		ProvisionCooldownMin:       15,
		Probe: QualityProbeConfig{
			Enabled:         true,
			Model:           "grok-4.6",
			MaxTokens:       128,
			TimeoutSeconds:  60,
			IntervalMinutes: 15,
		},
		Route: XAIRouteConfig{
			Mode:  XAIRouteModeIndependent,
			Hosts: []string{"cli-chat-proxy.grok.com", "api.x.ai"},
		},
	}
}

// SettingsConfig 通用设置（与 xAI 降智守护无关，任何场景都生效）。
type SettingsConfig struct {
	// 自动清理不健康出口：连通失败（Healthy=false）且未被规则引用的
	// 托管出口自动删除，防止异常出口堆积占满出口池。
	CleanupUnhealthy        bool `json:"cleanup_unhealthy_enabled"`
	CleanupUnhealthyMinutes int  `json:"cleanup_unhealthy_minutes"`
	// SystemProxy 系统代理：把当前全局出口应用到系统（写入
	// /etc/profile.d/warp-egress-proxy.sh 环境变量），系统其他进程
	// 的网络经 HTTP 桥 → 插件中继 → 当前全局出口。
	SystemProxy SystemProxyConfig `json:"system_proxy,omitempty"`
}

// SystemProxyConfig 系统代理配置（独立开关：应用到系统 / 不应用）。
type SystemProxyConfig struct {
	Enabled bool `json:"enabled"`
	// Port HTTP 桥监听端口（默认 40001）。
	Port int `json:"port"`
	// File 系统环境文件（默认 /etc/profile.d/warp-egress-proxy.sh）。
	File string `json:"file,omitempty"`
}

func defaultSettingsConfig() SettingsConfig {
	return SettingsConfig{CleanupUnhealthy: false, CleanupUnhealthyMinutes: 10}
}

type PersistedState struct {
	Version  int              `json:"version"`
	Profiles []*Profile       `json:"profiles"`
	Rules    Rules            `json:"rules"`
	Auto     AutoSwitchConfig `json:"auto_switch"`
	Quality  QualityConfig    `json:"quality,omitempty"`
	Settings SettingsConfig   `json:"settings,omitempty"`
}

type EffectiveRoute struct {
	ProfileID string `json:"profile_id,omitempty"`
	RuleType  string `json:"rule_type"`
	RuleKey   string `json:"rule_key,omitempty"`
	ProxyURL  string `json:"proxy_url,omitempty"`
}

type AuthFileView struct {
	hostAuthFileEntry
	Effective EffectiveRoute `json:"effective"`
}

type ApplyItemResult struct {
	AuthIndex string `json:"auth_index"`
	Name      string `json:"name"`
	RuleType  string `json:"rule_type,omitempty"`
	ProfileID string `json:"profile_id,omitempty"`
	ProxyURL  string `json:"proxy_url,omitempty"`
	Changed   bool   `json:"changed"`
	Skipped   bool   `json:"skipped"`
	Error     string `json:"error,omitempty"`
}

type ApplyRulesResult struct {
	Total   int               `json:"total"`
	Changed int               `json:"changed"`
	Skipped int               `json:"skipped"`
	Failed  int               `json:"failed"`
	Items   []ApplyItemResult `json:"items"`
}

type createProfileRequest struct {
	Name        string `json:"name"`
	Mode        string `json:"mode"`
	ProxyURL    string `json:"proxy_url,omitempty"`
	AutoStart   bool   `json:"auto_start"`
	RegisterVia string `json:"register_via,omitempty"`
	Origin      string `json:"origin,omitempty"`
}

type importProfileRequest struct {
	Name        string `json:"name"`
	WGCFProfile string `json:"wgcf_profile"`
}

type profileActionRequest struct {
	ID     string `json:"id"`
	Action string `json:"action"`
}

type profileDeleteRequest struct {
	ID string `json:"id"`
}

type globalSwitchRequest struct {
	ProfileID string `json:"profile_id"`
}

type exactAssignRequest struct {
	AuthIndex string `json:"auth_index"`
	ProfileID string `json:"profile_id"`
	ProxyURL  string `json:"proxy_url"`
	ApplyNow  bool   `json:"apply_now"`
}

// exactCustomPrefix 标记单文件出口规则引用的是自定义代理地址（CPA 认证文件 proxy_url 字段直接写值），
// 而不是插件管理的出口。值为 "custom:<代理地址>"。
const exactCustomPrefix = "custom:"

// exactDirect 标记单文件出口规则为"不设置代理"：清除认证文件的 proxy_url 且不被其他规则接管。
const exactDirect = "direct"

type statusResponse struct {
	PluginID             string               `json:"plugin_id"`
	Version              string               `json:"version"`
	GlobalRelayURL       string               `json:"global_relay_url"`
	GlobalRelayRunning   bool                 `json:"global_relay_running"`
	GlobalProfileID      string               `json:"global_profile_id,omitempty"`
	GlobalProfile        *Profile             `json:"global_profile,omitempty"`
	Profiles             []*Profile           `json:"profiles"`
	DuplicateExitIPs     map[string][]string  `json:"duplicate_exit_ips,omitempty"`
	DataDir              string               `json:"data_dir"`
	LastError            string               `json:"last_error,omitempty"`
	RequiredHostProxyURL string               `json:"required_host_proxy_url"`
	AutoSwitch           AutoSwitchConfig     `json:"auto_switch"`
	AutoProvision        *autoProvisionStatus `json:"auto_provision,omitempty"`
	UsageDiagnostics     UsageDiagnostics     `json:"usage_diagnostics,omitempty"`
}

// autoProvisionStatus 自动补充出口的当前状态（供面板展示）。
type autoProvisionStatus struct {
	Enabled              bool      `json:"enabled"`
	MinHealthy           int       `json:"min_healthy"`
	HealthyManaged       int       `json:"healthy_managed"`
	MaxProfiles          int       `json:"max_profiles"`
	LastAttemptAt        time.Time `json:"last_attempt_at,omitempty"`
	LastError            string    `json:"last_error,omitempty"`
	NextAttemptInSeconds int64     `json:"next_attempt_in_seconds,omitempty"`
}

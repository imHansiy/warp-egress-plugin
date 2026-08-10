package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// xAI 降智守护（Quality Guard）：仅针对 xAI / Grok 输出降智。
//
// 原理：共享出口 IP 被打穿时，AI 生成的输出 Token/s 会异常飙升（表现为"模型变笨"）。
// 插件被动观测 CPA usage 事件中的 xAI 输出 token 数与耗时，
// 估算输出 TPS；TPS 超过阈值且连续多次则给对应出口打上降智标记（Degraded）。
// xAI 认证文件不写 proxy_url；CPA 请求先进入插件本地中继，再由目标域名
// 路由到独立 xAI 活动出口。后台只复用少量可用账号错峰实测备用出口。

// 注意口径：本模块对外称为「xAI 降智守护」，仅针对 xAI / Grok 模型输出的
// 降智检测（usage/流式观测都只统计 xAI 类请求），只影响 xAI 全局出口的
// 质量判定和 xAI 独立活动出口切换；与其他 provider 的认证文件配置、
// 普通全局出口和手工分流规则无关。
// 自动清理 = 清理降智代理（删除所有降智标记的托管出口，全部清理）；
// max_profiles 仅作为自动补充的停止线。

// computeTPS 计算输出 TPS（token/s）。短回复首 token 时间接近总时长会虚高 TPS，
// 因此要求一个可配置的最小生成窗口；窗口不足时退化为用总时长计算。
func computeTPS(outputTokens, durationMs, firstTokenMs, minGenerationMs int64) float64 {
	if outputTokens <= 0 || durationMs <= 0 {
		return 0
	}
	denom := durationMs - firstTokenMs
	if minGenerationMs <= 0 {
		minGenerationMs = 1000
	}
	if denom < minGenerationMs {
		denom = durationMs
	}
	if denom < minGenerationMs {
		return 0
	}
	return float64(outputTokens) / (float64(denom) / 1000.0)
}

func classifyQualityTPS(tps float64, outputTokens int64, q QualityConfig) string {
	return classifyQualitySignal(tps, outputTokens, true, q)
}

// classifyQualitySignal 先要求足够样本，再用 thinking 与 TPS 两种互不混算
// 的信号判定。thinking 存在时仍沿用插件原有 TPS 逻辑。
func classifyQualitySignal(tps float64, outputTokens int64, hasThinking bool, q QualityConfig) string {
	if outputTokens <= 0 || tps <= 0 {
		return "unknown"
	}
	if q.MinOutputTokens > 0 && outputTokens < q.MinOutputTokens {
		return "ignored"
	}
	if q.ThinkingGuard && !hasThinking {
		return "degraded"
	}
	if q.SoftTPS > 0 && tps >= q.SoftTPS {
		return "degraded"
	}
	return "healthy"
}

func usageTokens(detail map[string]any) int64 {
	if detail == nil {
		return 0
	}
	best := int64(0)
	for _, key := range []string{"OutputTokens", "output_tokens", "outputTokens", "CompletionTokens", "completion_tokens", "completionTokens", "ReasoningTokens", "reasoning_tokens", "reasoningTokens", "TotalTokens", "total_tokens"} {
		if v := anyInt64(detail[key]); v > best {
			best = v
		}
	}
	return best
}

func anyInt64(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	case json.Number:
		n, _ := t.Int64()
		return n
	default:
		return 0
	}
}

// nsOrMsToMs 兼容 time.Duration（JSON 反序列化为纳秒 float64）与毫秒两种单位。
func nsOrMsToMs(v float64) int64 {
	if v > 1e6 {
		return int64(v / 1e6)
	}
	return int64(v)
}

func recordFloat(payload map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := payload[k]; ok {
			switch t := v.(type) {
			case float64:
				return t
			case int64:
				return float64(t)
			case int:
				return float64(t)
			case json.Number:
				f, _ := t.Float64()
				return f
			}
		}
	}
	return 0
}

func recordString(payload map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := payload[k].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// authProxyCache 缓存 auth 标识 → proxy_url 的映射（60s TTL），
// 让 usage 事件反查出口节点不需要逐条 host.auth.get。
var (
	authProxyMu    sync.Mutex
	authProxyCache map[string]string
	authProxyAt    time.Time
)

const authProxyCacheTTL = 60 * time.Second

func authProxyMap() map[string]string {
	authProxyMu.Lock()
	if authProxyCache != nil && time.Since(authProxyAt) < authProxyCacheTTL {
		out := authProxyCache
		authProxyMu.Unlock()
		return out
	}
	authProxyMu.Unlock()

	data := map[string]string{}
	if entries, err := callHostAuthList(); err == nil {
		for _, e := range entries.Files {
			if e.ProxyURL == "" {
				continue
			}
			if e.AuthIndex != "" {
				data[e.AuthIndex] = e.ProxyURL
			}
			if e.ID != "" {
				data[e.ID] = e.ProxyURL
			}
			if e.Name != "" {
				data[e.Name] = e.ProxyURL
			}
			if e.Email != "" {
				data[e.Email] = e.ProxyURL
			}
		}
	}
	authProxyMu.Lock()
	authProxyCache = data
	authProxyAt = time.Now()
	authProxyMu.Unlock()
	return data
}

// authProxyResolver 可替换，供单元测试注入 auth→proxy 映射；
// 生产环境总是调用 authProxyMap（走 host.auth.list）。
var authProxyResolver = func() map[string]string { return authProxyMap() }

// profileByAuthProxy 通过 usage 事件里的 auth 标识反查绑定的出口 profile。
func (m *Manager) profileByAuthProxy(authKeys ...string) *Profile {
	data := authProxyResolver()
	var proxy string
	for _, k := range authKeys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if p, ok := data[k]; ok && p != "" {
			proxy = p
			break
		}
	}
	if proxy == "" {
		return nil
	}
	for _, p := range m.stateStore().Profiles() {
		if p.ProxyURL == proxy {
			return p
		}
	}
	return nil
}

// HandleUsage 被动质量观测：CPA 每个请求完成后调用 usage.handle。
// 记录为供应商无关的 UsageRecord（Provider/AuthID/AuthIndex/Detail/Latency/TTFT/Failed）。
// 注意：输出 TPS 降智监测基于 xAI/Grok 类模型输出特征（共享出口被打穿时输出 token/s 异常飙升），
// 因此只统计 xAI / Grok 请求；其他 provider 的请求不参与，避免误标。
func (m *Manager) HandleUsage(record map[string]any) error {
	m.mu.Lock()
	m.usageEvents++
	if record != nil {
		m.lastUsageProvider = recordString(record, "Provider", "provider")
		m.lastUsageModel = recordString(record, "Model", "model")
		m.lastUsageAuth = recordString(record, "AuthID", "auth_id", "authId")
		if detail, ok := record["Detail"].(map[string]any); ok {
			m.lastUsageTokens = usageTokens(detail)
		}
		if v, ok := record["Latency"]; ok {
			m.lastUsageLatency = fmt.Sprintf("%v", v)
		}
	}
	m.mu.Unlock()
	store := m.stateStore()
	if store == nil {
		return nil
	}
	q := store.Quality()
	if !q.Enabled {
		return nil
	}
	if record == nil {
		return nil
	}
	provider := strings.ToLower(recordString(record, "Provider", "provider"))
	if !strings.Contains(provider, "xai") && !strings.Contains(provider, "grok") {
		return nil
	}
	if record["Failed"] == true || record["failed"] == true {
		// 失败请求不是出口降智的证据（可能只是账号/上游错误），不参与质量统计。
		return nil
	}
	var outTokens int64
	if detail, ok := record["Detail"].(map[string]any); ok {
		outTokens = usageTokens(detail)
	}
	if outTokens == 0 {
		if detail, ok := record["detail"].(map[string]any); ok {
			outTokens = usageTokens(detail)
		}
	}
	if outTokens == 0 {
		return nil
	}
	latencyNS := recordFloat(record, "Latency", "latency")
	ttftNS := recordFloat(record, "TTFT", "ttft")
	durationMs := nsOrMsToMs(latencyNS)
	firstTokenMs := nsOrMsToMs(ttftNS)
	if durationMs <= 0 {
		return nil
	}
	profile := m.profileByAuthProxy(recordString(record, "AuthID", "auth_id", "authId"),
		recordString(record, "AuthIndex", "auth_index", "authIndex"))
	if profile == nil {
		// 账号未显式绑定出口时，按 xAI 路由模式归到真正承载请求的
		// 独立出口或普通全局出口；direct 模式不做出口质量归因。
		if profileID := m.xaiQualityProfileID(); profileID != "" {
			profile = store.Profile(profileID)
		}
	}
	if profile == nil {
		return nil
	}
	tps := computeTPS(outTokens, durationMs, firstTokenMs, q.MinGenerationMs)
	hasThinking := recordHasThinking(record)
	class := classifyQualitySignal(tps, outTokens, hasThinking, q)
	return m.applyQualitySignal(profile.ID, tps, outTokens, hasThinking, class, "passive")
}

// applyQualityObservation 合并一次质量观测到 profile：
// degraded 连续达到阈值 → 打降智标记（立即落盘 + 评估故障转移）；
// healthy 连续恢复 → 清除标记。常规统计走防抖落盘，避免高频写盘。
func (m *Manager) applyQualityObservation(profileID string, tps float64, outTokens int64, class, source string) error {
	return m.applyQualitySignal(profileID, tps, outTokens, true, class, source)
}

func (m *Manager) applyQualitySignal(profileID string, tps float64, outTokens int64, hasThinking bool, class, source string) error {
	store := m.stateStore()
	if !store.Quality().Enabled {
		return nil
	}
	profile := store.Profile(profileID)
	if profile == nil {
		return errors.New("profile not found")
	}
	q := store.Quality()
	profile.QualityTPS = tps
	profile.QualityCheckedAt = time.Now()
	profile.QualityClassification = class
	profile.QualitySource = source
	profile.QualityError = ""
	stateChanged := false
	crossVerify := false
	switch class {
	case "degraded":
		profile.QualityRecovery = 0
		missingThinking := q.ThinkingGuard && !hasThinking
		if missingThinking {
			// thinking 缺失与 TPS 异常分别累计；交叉验证可能使用另一个
			// 探针账号，但只验证同一个出口，不把账号结论混成新样本。
			profile.QualityThinkingStrikes++
			profile.QualityStrikes = 0
			threshold := q.ConsecutiveMissingThinking
			if threshold <= 0 {
				threshold = 1
			}
			if !profile.Degraded && profile.QualityThinkingStrikes >= threshold {
				if q.ThinkingCrossVerify && source != "probe" {
					crossVerify = true
					profile.QualityClassification = "verifying"
					profile.QualityError = "响应缺少 thinking，等待主动交叉验证"
				} else {
					profile.Degraded = true
					profile.DegradedAt = time.Now()
					profile.DegradedReason = fmt.Sprintf("连续 %d 次响应缺少 thinking_content（%s）", profile.QualityThinkingStrikes, source)
					stateChanged = true
				}
			}
		} else {
			profile.QualityThinkingStrikes = 0
			profile.QualityStrikes++
			if !profile.Degraded && profile.QualityStrikes >= q.ConsecutiveDegraded {
				if q.SoftCrossVerify && source != "probe" {
					crossVerify = true
					profile.QualityClassification = "verifying"
					profile.QualityError = "TPS 异常，等待主动交叉验证"
				} else {
					profile.Degraded = true
					profile.DegradedAt = time.Now()
					profile.DegradedReason = fmt.Sprintf("连续 %d 次高输出 TPS（%.1f token/s，%s）", profile.QualityStrikes, tps, source)
					stateChanged = true
				}
			}
		}
	case "healthy":
		profile.QualityStrikes = 0
		profile.QualityThinkingStrikes = 0
		if profile.Degraded {
			profile.QualityRecovery++
			if profile.QualityRecovery >= q.RecoveryObservations {
				profile.Degraded = false
				profile.DegradedAt = time.Time{}
				profile.DegradedReason = ""
				profile.QualityRecovery = 0
				stateChanged = true
			}
		} else {
			profile.QualityRecovery = 0
		}
	}
	if stateChanged {
		_ = store.UpdateProfile(profile)
		m.evaluateDegradedFailover(profile)
		return nil
	}
	_ = store.UpdateProfileQuiet(profile)
	m.scheduleQualitySave()
	if crossVerify {
		m.scheduleQualityCrossVerify(profile.ID)
	}
	return nil
}

// scheduleQualityCrossVerify 同一出口最多挂一个复测任务，避免连续真实请求
// 同时触发多次主动模型探测和额外 Token 消耗。
func (m *Manager) scheduleQualityCrossVerify(profileID string) {
	m.qualityCrossVerifyMu.Lock()
	if m.qualityCrossVerify == nil {
		m.qualityCrossVerify = map[string]bool{}
	}
	if m.qualityCrossVerify[profileID] {
		m.qualityCrossVerifyMu.Unlock()
		return
	}
	m.qualityCrossVerify[profileID] = true
	m.qualityCrossVerifyMu.Unlock()
	go func() {
		defer func() {
			m.qualityCrossVerifyMu.Lock()
			delete(m.qualityCrossVerify, profileID)
			m.qualityCrossVerifyMu.Unlock()
		}()
		_, _ = m.ProbeProfile(profileID)
	}()
}

func (m *Manager) scheduleQualitySave() {
	m.mu.Lock()
	if m.qualitySavePending {
		m.mu.Unlock()
		return
	}
	m.qualitySavePending = true
	m.mu.Unlock()
	time.AfterFunc(2*time.Second, func() {
		m.mu.Lock()
		m.qualitySavePending = false
		m.mu.Unlock()
		_ = m.stateStore().Save()
	})
}

// evaluateDegradedFailover 当前全局出口被打上降智标记时，
// 异步确认并切换到近期 xAI 实测健康的备用出口，不阻塞 usage 热路径。
// 降智检测与降智切换一体：xAI 降智守护开启即生效，无需额外开关。
func (m *Manager) evaluateDegradedFailover(profile *Profile) {
	if profile == nil || !profile.Degraded {
		return
	}
	if !m.stateStore().Quality().Enabled {
		return
	}
	quality := m.stateStore().Quality()
	switch quality.Route.Mode {
	case XAIRouteModeIndependent:
		if quality.Route.ActiveProfileID != profile.ID {
			return
		}
		go func() { _, _ = m.EvaluateXAISwitch(false) }()
	case XAIRouteModeFollowGlobal:
		if m.stateStore().Rules().GlobalProfileID != profile.ID {
			return
		}
		go func() { _, _ = m.EvaluateAutoSwitch(false) }()
	}
}

func mapHasThinkingContent(data map[string]any) bool {
	if data == nil {
		return false
	}
	for _, key := range []string{
		"thinking_content", "ThinkingContent", "thinkingContent",
		"reasoning_content", "ReasoningContent", "reasoningContent",
		"thinking", "Thinking",
	} {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

// recordHasThinking 兼容 CPA usage 事件只带 usage 数字、只带响应嵌套字段，
// 或显式 has_thinking 标记的不同版本。
func recordHasThinking(record map[string]any) bool {
	if mapHasThinkingContent(record) {
		return true
	}
	for _, key := range []string{"Detail", "detail", "Message", "message", "Response", "response", "Delta", "delta", "Usage", "usage"} {
		nested, _ := record[key].(map[string]any)
		if mapHasThinkingContent(nested) || anyInt64(nested["reasoning_tokens"]) > 0 || anyInt64(nested["reasoningTokens"]) > 0 {
			return true
		}
	}
	for _, key := range []string{"has_thinking", "HasThinking", "hasThinking", "has_reasoning", "HasReasoning"} {
		switch value := record[key].(type) {
		case bool:
			if value {
				return true
			}
		case string:
			if strings.EqualFold(strings.TrimSpace(value), "true") || strings.TrimSpace(value) == "1" {
				return true
			}
		}
	}
	return anyInt64(record["reasoning_tokens"]) > 0 || anyInt64(record["reasoningTokens"]) > 0
}

func qualityObservationCurrent(profile *Profile, q QualityConfig, now time.Time) bool {
	if profile == nil || profile.QualityCheckedAt.IsZero() {
		return false
	}
	maxAge := time.Duration(q.Probe.IntervalMinutes) * time.Minute
	if maxAge <= 0 {
		maxAge = 15 * time.Minute
	}
	return !now.After(profile.QualityCheckedAt.Add(maxAge))
}

func qualityObservationFresh(profile *Profile, q QualityConfig, now time.Time) bool {
	return profile != nil && profile.QualityClassification == "healthy" &&
		qualityObservationCurrent(profile, q, now)
}

// xaiQualityProfileID 返回真实承载 xAI 请求的出口。被动 usage 和流式补偿
// 必须使用同一口径，否则独立模式会把降智样本记到普通全局出口。
func (m *Manager) xaiQualityProfileID() string {
	store := m.stateStore()
	if store == nil {
		return ""
	}
	quality := store.Quality()
	switch quality.Route.Mode {
	case XAIRouteModeIndependent:
		return quality.Route.ActiveProfileID
	case XAIRouteModeFollowGlobal:
		return store.Rules().GlobalProfileID
	default:
		return ""
	}
}

// ensureXAIActiveProfile 为独立路由补齐活动出口。普通全局为空也不会影响
// 这里的选择；首次启动允许先选网络健康出口，后续主动探测维护质量结论。
func (m *Manager) ensureXAIActiveProfile(force bool) (*Profile, error) {
	store := m.stateStore()
	if store == nil {
		return nil, errors.New("plugin is not configured")
	}
	quality := store.Quality()
	if !quality.Enabled || quality.Route.Mode != XAIRouteModeIndependent {
		return nil, nil
	}
	current := store.Profile(quality.Route.ActiveProfileID)
	if !force && profileUsableForXAIRoute(current) {
		return current, nil
	}
	return m.switchXAIToVerifiedQualityCandidate(current, current != nil && current.Degraded)
}

// EvaluateXAISwitch 只更新 xAI 独立活动出口，绝不写普通 GlobalProfileID。
func (m *Manager) EvaluateXAISwitch(force bool) (*Profile, error) {
	store := m.stateStore()
	if store == nil {
		return nil, errors.New("plugin is not configured")
	}
	quality := store.Quality()
	if !quality.Enabled || quality.Route.Mode != XAIRouteModeIndependent {
		return nil, nil
	}
	current := store.Profile(quality.Route.ActiveProfileID)
	if !force && profileUsableForXAIRoute(current) {
		return nil, nil
	}
	return m.switchXAIToVerifiedQualityCandidate(current, current != nil && current.Degraded)
}

func (m *Manager) switchXAIToVerifiedQualityCandidate(current *Profile, requireQualityProof bool) (*Profile, error) {
	store := m.stateStore()
	quality := store.Quality()
	profiles := store.Profiles()
	start := -1
	for i, profile := range profiles {
		if current != nil && profile.ID == current.ID {
			start = i
			break
		}
	}
	now := time.Now()
	auto := store.AutoSwitch()
	for offset := 1; offset <= len(profiles); offset++ {
		index := (start + offset + len(profiles)) % len(profiles)
		candidate := profiles[index]
		if !profileUsableForXAIRoute(candidate) || (current != nil && candidate.ID == current.ID) {
			continue
		}
		if auto.RequireDifferentIP && current != nil && current.ExitIP != "" && candidate.ExitIP == current.ExitIP {
			continue
		}
		verified := qualityObservationFresh(candidate, quality, now)
		if requireQualityProof && quality.Probe.Enabled && !verified {
			// 本轮已经得到失败/降智结论的候选直接跳过，防止每次健康
			// tick 都重复消耗同一批 xAI 探针账号额度。
			if qualityObservationCurrent(candidate, quality, now) {
				continue
			}
			result, _ := m.ProbeProfile(candidate.ID)
			verified = result.Classification == "healthy"
		}
		if requireQualityProof && quality.Probe.Enabled && !verified {
			continue
		}
		if err := store.SetXAIActiveProfile(candidate.ID); err != nil {
			return nil, err
		}
		return store.Profile(candidate.ID), nil
	}
	return nil, errors.New("no eligible healthy xAI egress; independent route remains blocked")
}

// switchToVerifiedQualityCandidate 仅服务于 xAI 降智故障转移。
// 近期实测健康的备用出口可直接切换；结论过期的出口先做一次主动探测，
// 确认健康后才接管全局流量。通用定时轮换与网络故障转移仍走原有逻辑。
func (m *Manager) switchToVerifiedQualityCandidate(current *Profile, profiles []*Profile, auto AutoSwitchConfig) (*Profile, error) {
	store := m.stateStore()
	q := store.Quality()
	start := -1
	for index, profile := range profiles {
		if current != nil && profile.ID == current.ID {
			start = index
			break
		}
	}
	now := time.Now()
	for offset := 1; offset <= len(profiles); offset++ {
		index := (start + offset + len(profiles)) % len(profiles)
		candidate := profiles[index]
		if candidate == nil || !candidate.Healthy || candidate.Degraded || candidate.ProxyURL == "" ||
			(candidate.Mode == ProfileModeManaged && !candidate.Running) ||
			(current != nil && candidate.ID == current.ID) {
			continue
		}
		if auto.RequireDifferentIP && current != nil && current.ExitIP != "" && candidate.ExitIP == current.ExitIP {
			continue
		}
		verified := qualityObservationFresh(candidate, q, now)
		if !verified {
			// 刚确认失败或降智的候选在复检间隔内直接跳过，避免全局出口
			// 持续降智时每个健康检查周期都重复消耗 xAI 探测用量。
			if qualityObservationCurrent(candidate, q, now) {
				continue
			}
			result, _ := m.ProbeProfile(candidate.ID)
			verified = result.Classification == "healthy"
		}
		if !verified {
			continue
		}
		if err := store.SetGlobalProfile(candidate.ID); err != nil {
			return nil, err
		}
		if err := store.RecordSwitch(candidate.ID, "degraded"); err != nil {
			return nil, err
		}
		return store.Profile(candidate.ID), nil
	}
	return nil, errors.New("no recently verified healthy xAI egress for automatic switch")
}

// probeQualityResult 主动探测结果（暴露给 UI 与管理 API）。
type probeQualityResult struct {
	ProfileID      string  `json:"profile_id"`
	Classification string  `json:"classification"` // healthy / degraded / unknown / error
	TPS            float64 `json:"tps"`
	OutputTokens   int64   `json:"output_tokens"`
	DurationMs     int64   `json:"duration_ms"`
	FirstTokenMs   int64   `json:"first_token_ms"`
	HasThinking    bool    `json:"has_thinking"`
	ErrorKind      string  `json:"error_kind,omitempty"`
	Error          string  `json:"error,omitempty"`
}

func isAccountQuotaExhausted(status int, body string) bool {
	lower := strings.ToLower(body)
	for _, marker := range []string{"free-usage-exhausted", "free_usage_exhausted", "subscription:free-usage", "included free usage"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return (status == http.StatusTooManyRequests &&
		(strings.Contains(lower, "quota") || strings.Contains(lower, "usage") || strings.Contains(lower, "rate"))) ||
		(strings.Contains(lower, "quota") && strings.Contains(lower, "exhaust"))
}

func shouldRetryProbeAccount(hasNext bool, status int, body string) bool {
	if !hasNext {
		return false
	}
	if isAccountQuotaExhausted(status, body) {
		return true
	}
	switch status {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusPaymentRequired,
		http.StatusForbidden, http.StatusNotFound, http.StatusConflict,
		http.StatusUnprocessableEntity, http.StatusTooManyRequests:
		return true
	}
	lower := strings.ToLower(body)
	for _, marker := range []string{"invalid token", "expired", "no auth", "permission denied", "x_xai_token_auth=none"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func isProbeUnstableErr(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	for _, marker := range []string{
		"timeout", "timed out", "deadline exceeded", "unexpected eof", "eof",
		"connection reset", "connection refused", "broken pipe", "stream error",
		"http2: stream", "server closed idle connection", "tls:", "tls handshake",
		"use of closed network connection",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func httpClientThroughProfile(proxyURL string, timeout time.Duration) (*http.Client, error) {
	parsed, err := url.Parse(proxyURL)
	if err != nil || parsed.Host == "" {
		return nil, errors.New("invalid profile proxy URL")
	}
	proxyAddr := parsed.Host
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialSOCKS5(ctx, proxyAddr, address)
		},
		TLSHandshakeTimeout: 15 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}

// xaiAccount 主动探测复用的 CPA xAI 账号（从 host.auth.list/get 获取）。
type xaiAccount struct {
	AuthIndex   string
	AccessToken string
	BaseURL     string
	Headers     map[string]string
	Expired     bool
}

// 主动探测只读取 CPA 认证状态和认证内容，不保存认证文件，也不修改 proxy_url。
// 注入点让测试可以验证大规模账号目录下只读取少量健康账号。
var (
	authListForProbe = func() (hostAuthListResponse, error) { return callHostAuthList() }
	authGetForProbe  = func(authIndex string) (hostAuthGetResponse, error) { return callHostAuthGet(authIndex) }
)

func isXAIEntry(entry hostAuthFileEntry) bool {
	provider := strings.ToLower(entry.Provider)
	authType := strings.ToLower(entry.Type)
	return strings.Contains(provider, "xai") || strings.Contains(provider, "grok") ||
		strings.Contains(authType, "xai") || strings.Contains(authType, "grok")
}

// isHealthyXAIProbeEntry 只允许 CPA 当前可调度的 xAI 认证参与探测。
// 空 status 兼容尚未返回运行状态的旧版 CLIProxyAPI；其他非 active 状态均跳过。
func isHealthyXAIProbeEntry(entry hostAuthFileEntry, now time.Time) bool {
	if !isXAIEntry(entry) || entry.AuthIndex == "" || entry.RuntimeOnly || entry.Disabled || entry.Unavailable {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(entry.Status))
	if status != "" && status != "active" {
		return false
	}
	return entry.NextRetryAfter.IsZero() || !now.Before(entry.NextRetryAfter)
}

// listXAIAccounts 从 CPA auth 目录筛选 xai/grok 类账号，
// 供主动质量探测经被测出口发送真实 xAI 请求。
func listXAIAccounts(limit int) []xaiAccount {
	if limit <= 0 {
		limit = 8
	}
	entries, err := authListForProbe()
	if err != nil {
		return nil
	}
	out := make([]xaiAccount, 0, limit)
	now := time.Now()
	for _, entry := range entries.Files {
		if !isHealthyXAIProbeEntry(entry, now) {
			continue
		}
		got, errGet := authGetForProbe(entry.AuthIndex)
		if errGet != nil {
			continue
		}
		var raw map[string]any
		if json.Unmarshal(got.JSON, &raw) != nil {
			continue
		}
		token, _ := raw["access_token"].(string)
		if strings.TrimSpace(token) == "" {
			continue
		}
		baseURL, _ := raw["base_url"].(string)
		if strings.TrimSpace(baseURL) == "" {
			baseURL = "https://cli-chat-proxy.grok.com/v1"
		}
		account := xaiAccount{
			AuthIndex:   entry.AuthIndex,
			AccessToken: token,
			BaseURL:     strings.TrimRight(baseURL, "/"),
			Headers:     map[string]string{},
		}
		if headers, ok := raw["headers"].(map[string]any); ok {
			for key, value := range headers {
				if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
					account.Headers[key] = s
				}
			}
		}
		if expired, ok := raw["expired"].(string); ok {
			for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
				if t, err := time.Parse(layout, expired); err == nil {
					account.Expired = time.Now().After(t)
					break
				}
			}
		}
		if account.Expired {
			continue
		}
		out = append(out, account)
		if len(out) >= limit {
			break
		}
	}
	return out
}

const xaiProbeAccountCacheTTL = 5 * time.Minute

// xaiAccountsForProbe 在多个备用出口之间短时复用同一小组健康探针账号。
// host.auth.list 在数千账号时返回体较大，缓存可把目录扫描限制为每 5 分钟一次；
// 账号出现认证/配额错误时会立即失效，下次探测重新从 CPA 健康状态筛选。
func (m *Manager) xaiAccountsForProbe(limit int) []xaiAccount {
	if limit <= 0 {
		limit = 8
	}
	if len(m.qualityProbeAuths) > 0 && time.Since(m.qualityProbeAuthAt) < xaiProbeAccountCacheTTL {
		if limit > len(m.qualityProbeAuths) {
			limit = len(m.qualityProbeAuths)
		}
		return append([]xaiAccount(nil), m.qualityProbeAuths[:limit]...)
	}
	accounts := listXAIAccounts(limit)
	m.qualityProbeAuths = append([]xaiAccount(nil), accounts...)
	m.qualityProbeAuthAt = time.Now()
	return accounts
}

func (m *Manager) invalidateXAIProbeAccounts() {
	m.qualityProbeAuths = nil
	m.qualityProbeAuthAt = time.Time{}
}

func applyGrokHeaders(req *http.Request, account xaiAccount) {
	// xAI 官方代理要求这些客户端头，缺失会返回 401 "no auth context"。
	req.Header.Set("X-XAI-Token-Auth", "xai-grok-cli")
	req.Header.Set("x-grok-client-version", "0.2.93")
	req.Header.Set("x-grok-client-identifier", "grok-shell")
	for key, value := range account.Headers {
		req.Header.Set(key, value)
	}
	if req.Header.Get("X-XAI-Token-Auth") == "" {
		req.Header.Set("X-XAI-Token-Auth", "xai-grok-cli")
	}
	if req.Header.Get("x-grok-client-version") == "" {
		req.Header.Set("x-grok-client-version", "0.2.93")
	}
	if req.Header.Get("x-grok-client-identifier") == "" {
		req.Header.Set("x-grok-client-identifier", "grok-shell")
	}
}

// ---------------------------------------------------------------------------
// 流式补偿：CPA 对 xAI 流式请求不发布 usage 事件（cli-chat-proxy 流式响应
// 不含 usage 字段，ParseCodexUsage 无法识别），导致被动 TPS 观测只覆盖
// 非流式请求。这里通过 request.intercept_before + response.intercept_stream_chunk
// 自行统计流式输出的字符量与耗时，补偿被动检测的流式盲区。
// 归因与 usage 事件一致：账号未绑定出口时归到独立 xAI 活动出口；
// follow_global 才归普通全局，direct 模式不做出口观测。
// ---------------------------------------------------------------------------

type streamTrack struct {
	startAt      time.Time
	firstTokenAt time.Time
	chars        int
	hasThinking  bool
}

const streamTrackTTL = 5 * time.Minute

func (m *Manager) HandleRequestBefore(record map[string]any) error {
	q := m.stateStore().Quality()
	if !q.Enabled {
		return nil
	}
	if record == nil {
		return nil
	}
	if record["Stream"] != true && record["stream"] != true {
		return nil
	}
	requestID := recordString(record, "RequestID", "request_id", "requestId")
	if requestID == "" {
		return nil
	}
	// 与被动 usage 一致：只统计 xAI / Grok 模型输出（OpenAI 兼容接口的
	// grok-4.5 等模型名即 grok-*，可覆盖；其他模型不参与，避免误标）。
	model := strings.ToLower(recordString(record, "Model", "model"))
	if !strings.Contains(model, "grok") && !strings.Contains(model, "xai") {
		return nil
	}
	m.mu.Lock()
	m.streamBeforeEvents++
	m.mu.Unlock()
	m.streamMu.Lock()
	if m.streamTracks == nil {
		m.streamTracks = map[string]*streamTrack{}
	}
	m.streamTracks[requestID] = &streamTrack{startAt: time.Now()}
	m.streamMu.Unlock()
	return nil
}

// HandleStreamChunk 逐 chunk 统计流式输出；[DONE] 帧到达时结算一次观测。
func (m *Manager) HandleStreamChunk(record map[string]any) error {
	q := m.stateStore().Quality()
	if !q.Enabled {
		return nil
	}
	if record == nil {
		return nil
	}
	requestID := recordString(record, "RequestID", "request_id", "requestId")
	if requestID == "" {
		return nil
	}
	body, _ := record["Body"].([]byte)
	if body == nil {
		if s, ok := record["Body"].(string); ok {
			body = []byte(s)
		}
	}
	if body == nil {
		if s, ok := record["body"].(string); ok {
			body = []byte(s)
		}
	}
	if len(body) == 0 {
		return nil
	}
	m.mu.Lock()
	m.streamChunkEvents++
	m.mu.Unlock()
	chunkIndex := int(anyInt64(record["ChunkIndex"]))
	m.mu.Lock()
	m.streamChunkIndexes = append(m.streamChunkIndexes, chunkIndex)
	if len(m.streamChunkIndexes) > 5 {
		m.streamChunkIndexes = m.streamChunkIndexes[len(m.streamChunkIndexes)-5:]
	}
	m.mu.Unlock()
	m.streamMu.Lock()
	track := m.streamTracks[requestID]
	if track == nil {
		if chunkIndex == streamChunkHeaderInit {
			m.streamMu.Unlock()
			return nil
		}
		track = &streamTrack{startAt: time.Now()}
		if m.streamTracks == nil {
			m.streamTracks = map[string]*streamTrack{}
		}
		m.streamTracks[requestID] = track
	}
	if chunkIndex == streamChunkHeaderInit && track.startAt.IsZero() {
		track.startAt = time.Now()
	}
	done, chars, hasThinking := extractStreamChunkMetrics(body)
	if hasThinking {
		track.hasThinking = true
	}
	if chars > 0 {
		if track.firstTokenAt.IsZero() {
			track.firstTokenAt = time.Now()
		}
		track.chars += chars
	}
	m.mu.Lock()
	m.streamTrackChars = track.chars
	m.streamTrackDone = done
	if len(body) > 0 {
		sample := string(body)
		if len(sample) > 300 {
			sample = sample[:300]
		}
		m.streamSample = sample
	}
	m.mu.Unlock()
	m.streamMu.Unlock()
	if done {
		m.finishStreamTrack(requestID)
	}
	return nil
}

// extractStreamChunkMetrics 从拦截器 chunk 中提取输出字符数与是否结束。
// rpc 传输时 Body（[]byte）序列化为 base64；解码后可能是 SSE 行
// （"data: {...}"）或纯 JSON（CPA 已剥掉前缀）。
func extractStreamChunkMetrics(body []byte) (done bool, chars int, hasThinking bool) {
	raw := body
	if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body))); err == nil && len(decoded) > 0 {
		raw = decoded
	}
	text := string(raw)
	trimmed := strings.TrimSpace(text)
	if trimmed == "[DONE]" || strings.HasSuffix(trimmed, "data: [DONE]") {
		return true, 0, false
	}
	handleJSON := func(data string) {
		var chunk map[string]any
		if json.Unmarshal([]byte(data), &chunk) != nil {
			return
		}
		if choices, ok := chunk["choices"].([]any); ok {
			for _, c := range choices {
				choice, _ := c.(map[string]any)
				delta, _ := choice["delta"].(map[string]any)
				if delta == nil {
					delta, _ = choice["message"].(map[string]any)
				}
				if delta == nil {
					continue
				}
				if mapHasThinkingContent(delta) {
					hasThinking = true
				}
				if content, _ := delta["content"].(string); content != "" {
					chars += len([]rune(content))
				}
				for _, key := range []string{"thinking_content", "reasoning_content", "thinking"} {
					if reasoning, _ := delta[key].(string); reasoning != "" {
						chars += len([]rune(reasoning))
					}
				}
			}
			// xAI/OpenAI 流的结束帧带 finish_reason（拦截器链上可能
			// 没有 [DONE] 帧，用 finish_reason 判定流结束）。
			for _, c := range choices {
				choice, _ := c.(map[string]any)
				if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
					done = true
				}
			}
		}
		// xAI 原生 Responses 事件（未翻译时）：delta 为字符串。
		if t, ok := chunk["type"].(string); ok && (t == "response.output_text.delta" || t == "response.output_reasoning_text.delta") {
			if textPart, ok := chunk["delta"].(string); ok && textPart != "" {
				chars += len([]rune(textPart))
				if t == "response.output_reasoning_text.delta" {
					hasThinking = true
				}
			}
		}
	}
	// 纯 JSON（无 SSE 前缀）。
	if strings.HasPrefix(trimmed, "{") {
		handleJSON(trimmed)
		return done, chars, hasThinking
	}
	// SSE 行。
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			if data == "[DONE]" {
				done = true
			}
			continue
		}
		handleJSON(data)
	}
	return done, chars, hasThinking
}

const streamChunkHeaderInit = -1

// finishStreamTrack 结算一个流式请求的 TPS 观测并清理状态。
func (m *Manager) finishStreamTrack(requestID string) {
	m.streamMu.Lock()
	track := m.streamTracks[requestID]
	if track != nil {
		delete(m.streamTracks, requestID)
	}
	m.streamMu.Unlock()
	if track == nil || track.chars <= 0 {
		return
	}
	q := m.stateStore().Quality()
	outTokens := int64(track.chars / 4)
	if outTokens == 0 {
		outTokens = 1
	}
	duration := time.Since(track.startAt).Milliseconds()
	firstTokenMs := int64(0)
	if !track.firstTokenAt.IsZero() {
		firstTokenMs = track.firstTokenAt.Sub(track.startAt).Milliseconds()
	}
	tps := computeTPS(outTokens, duration, firstTokenMs, q.MinGenerationMs)
	class := classifyQualitySignal(tps, outTokens, track.hasThinking, q)
	if class == "unknown" || class == "ignored" {
		return
	}
	// 账号不绑定代理时，按独立/follow_global 的真实活动出口归因。
	profileID := m.xaiQualityProfileID()
	if profileID == "" {
		return
	}
	profile := m.stateStore().Profile(profileID)
	if profile == nil {
		return
	}
	_ = m.applyQualitySignal(profile.ID, tps, outTokens, track.hasThinking, class, "stream")
}

// startStreamTrackTTLCleanup 周期清理异常断开的流式轨道（兜底结算）。
func (m *Manager) startStreamTrackTTLCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				m.streamMu.Lock()
				var expired []string
				for id, t := range m.streamTracks {
					if t != nil && now.Sub(t.startAt) > streamTrackTTL {
						expired = append(expired, id)
					}
				}
				m.streamMu.Unlock()
				for _, id := range expired {
					m.finishStreamTrack(id)
				}
			}
		}
	}()
}

// ProbeProfile 主动质量探测：经该出口，复用 CPA 内 xAI 账号向 xAI 端点
// 发一个流式请求，实测输出 TPS 并应用观测。账号类失败（401/403/429）换下一个
// 账号再试，不把账号问题误判为出口降智。
func (m *Manager) ProbeProfile(profileID string) (probeQualityResult, error) {
	store := m.stateStore()
	profile := store.Profile(profileID)
	if profile == nil {
		return probeQualityResult{}, errors.New("profile not found")
	}
	q := store.Quality()
	res := probeQualityResult{ProfileID: profileID}
	probe := q.Probe
	if !probe.Enabled {
		return res, errors.New("quality probe is disabled")
	}
	if strings.TrimSpace(probe.Model) == "" {
		return res, errors.New("probe model is required（xAI 模型名，如 grok-4）")
	}
	// 主动探测会消耗少量真实 xAI 用量；串行化可避免定时、手工与故障转移
	// 同时触发，确保任一时刻最多只有一个出口在探测。
	m.qualityProbeMu.Lock()
	defer m.qualityProbeMu.Unlock()
	accounts := m.xaiAccountsForProbe(8)
	if len(accounts) == 0 {
		err := errors.New("没有可用的 CPA xAI 账号，无法主动探测（仅使用未禁用、未冷却的健康账号）")
		m.recordQualityProbeError(profileID, err.Error())
		return res, err
	}
	client, err := httpClientThroughProfile(profile.ProxyURL, time.Duration(probe.TimeoutSeconds)*time.Second)
	if err != nil {
		res.Classification = "error"
		res.Error = err.Error()
		m.recordQualityProbeError(profileID, res.Error)
		return res, err
	}
	maxTokens := probe.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 128
	}
	payload, _ := json.Marshal(map[string]any{
		"model": probe.Model,
		"messages": []map[string]string{
			{"role": "user", "content": "Write a detailed technical explanation of how TCP slow start works, at least 12 sentences, plain text only."},
		},
		"stream":      true,
		"max_tokens":  maxTokens,
		"temperature": 0.7,
	})
	var lastErr string
	for accountIndex, account := range accounts {
		if account.Expired {
			lastErr = "xAI 账号已过期"
			continue
		}
		request, errReq := http.NewRequest(http.MethodPost, account.BaseURL+"/chat/completions", bytes.NewReader(payload))
		if errReq != nil {
			res.Classification = "error"
			res.Error = "无法创建探测请求"
			return res, errReq
		}
		request.Header.Set("Authorization", "Bearer "+account.AccessToken)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "text/event-stream")
		applyGrokHeaders(request, account)

		start := time.Now()
		response, errDo := client.Do(request)
		if errDo != nil {
			lastErr = "probe request failed: " + truncateString(errDo.Error(), 120)
			res.DurationMs = time.Since(start).Milliseconds()
			if isProbeUnstableErr(errDo) {
				res.Classification = "degraded"
				res.ErrorKind = "probe_unstable"
				res.Error = "出口探测超时或链路不稳定: " + truncateString(errDo.Error(), 120)
				m.markQualityUnstable(profileID, res.Error)
				return res, nil
			}
			continue
		}
		if response.StatusCode >= 400 {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
			_ = response.Body.Close()
			bodyText := string(body)
			lastErr = fmt.Sprintf("probe upstream HTTP %d: %s", response.StatusCode, truncateString(bodyText, 160))
			res.DurationMs = time.Since(start).Milliseconds()
			// 账号、免费额度与限流属于当前探针账号；只有仍有候选时才换号，
			// 不把这些错误累计成出口降智。
			if shouldRetryProbeAccount(accountIndex+1 < len(accounts), response.StatusCode, bodyText) {
				m.invalidateXAIProbeAccounts()
				continue
			}
			res.Classification = "error"
			res.Error = lastErr
			m.recordQualityProbeError(profileID, res.Error)
			return res, nil
		}

		var (
			firstTokenAt time.Time
			contentLen   int
			usageOut     int64
			hasThinking  bool
		)
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" || data == "[DONE]" {
				if data == "[DONE]" {
					break
				}
				continue
			}
			var chunk map[string]any
			if json.Unmarshal([]byte(data), &chunk) != nil {
				continue
			}
			if usage, ok := chunk["usage"].(map[string]any); ok {
				if v := usageTokens(usage); v > usageOut {
					usageOut = v
				}
				if anyInt64(usage["reasoning_tokens"]) > 0 || anyInt64(usage["reasoningTokens"]) > 0 {
					hasThinking = true
				}
			}
			if choices, ok := chunk["choices"].([]any); ok {
				for _, c := range choices {
					choice, _ := c.(map[string]any)
					delta, _ := choice["delta"].(map[string]any)
					if delta == nil {
						delta, _ = choice["message"].(map[string]any)
					}
					if delta == nil {
						continue
					}
					if mapHasThinkingContent(delta) {
						hasThinking = true
					}
					content, _ := delta["content"].(string)
					if content != "" {
						if firstTokenAt.IsZero() {
							firstTokenAt = time.Now()
						}
						contentLen += len([]rune(content))
					}
					for _, key := range []string{"thinking_content", "reasoning_content", "thinking"} {
						if reasoning, _ := delta[key].(string); reasoning != "" {
							if firstTokenAt.IsZero() {
								firstTokenAt = time.Now()
							}
							contentLen += len([]rune(reasoning))
						}
					}
				}
			}
		}
		_ = response.Body.Close()
		if scanErr := scanner.Err(); scanErr != nil {
			lastErr = "probe stream failed: " + truncateString(scanErr.Error(), 120)
			res.DurationMs = time.Since(start).Milliseconds()
			if isProbeUnstableErr(scanErr) {
				res.Classification = "degraded"
				res.ErrorKind = "probe_unstable"
				res.Error = "出口探测流中断: " + truncateString(scanErr.Error(), 120)
				m.markQualityUnstable(profileID, res.Error)
				return res, nil
			}
			continue
		}
		duration := time.Since(start)
		res.DurationMs = duration.Milliseconds()
		if !firstTokenAt.IsZero() {
			res.FirstTokenMs = firstTokenAt.Sub(start).Milliseconds()
		}
		outTokens := usageOut
		if outTokens <= 0 {
			outTokens = int64(contentLen / 4)
			if outTokens == 0 && contentLen > 0 {
				outTokens = 1
			}
		}
		res.OutputTokens = outTokens
		res.HasThinking = hasThinking
		res.TPS = computeTPS(outTokens, res.DurationMs, res.FirstTokenMs, q.MinGenerationMs)
		res.Classification = classifyQualitySignal(res.TPS, outTokens, hasThinking, q)
		if res.Classification == "unknown" {
			lastErr = "探测无输出"
			continue
		}
		if res.Classification == "ignored" {
			res.Classification = "unknown"
		}
		if res.Classification == "degraded" && q.ThinkingGuard && !hasThinking {
			res.Error = "响应缺少 thinking_content（降智）"
		}
		_ = m.applyQualitySignal(profileID, res.TPS, outTokens, hasThinking, res.Classification, "probe")
		return res, nil
	}
	res.Classification = "error"
	if lastErr == "" {
		lastErr = "所有 xAI 账号探测失败"
	}
	res.Error = lastErr
	m.recordQualityProbeError(profileID, res.Error)
	return res, nil
}

func (m *Manager) recordQualityProbeError(profileID, message string) {
	store := m.stateStore()
	profile := store.Profile(profileID)
	if profile == nil {
		return
	}
	profile.QualityTPS = 0
	profile.QualityClassification = "error"
	profile.QualitySource = "probe"
	profile.QualityError = truncateString(message, 240)
	profile.QualityCheckedAt = time.Now()
	_ = store.UpdateProfileQuiet(profile)
	m.scheduleQualitySave()
}

// markQualityUnstable 处理真实探测中的超时/断流。它不代表模型一定降智，
// 但该出口无法稳定承载流式 xAI 请求，因此立即退出活动池而不是换探针账号。
func (m *Manager) markQualityUnstable(profileID, message string) {
	store := m.stateStore()
	profile := store.Profile(profileID)
	if profile == nil {
		return
	}
	profile.Degraded = true
	profile.DegradedAt = time.Now()
	profile.DegradedReason = truncateString(message, 240)
	profile.QualityClassification = "degraded"
	profile.QualitySource = "probe"
	profile.QualityError = truncateString(message, 240)
	profile.QualityCheckedAt = time.Now()
	_ = store.UpdateProfile(profile)
	m.evaluateDegradedFailover(profile)
}

func qualityProbeInterval(q QualityConfig) time.Duration {
	interval := time.Duration(q.Probe.IntervalMinutes) * time.Minute
	if interval <= 0 {
		return 15 * time.Minute
	}
	return interval
}

// selectQualityProbeCandidate 从连通正常的出口中选择最久未测的一个。
// 每轮只返回一个出口，账号和出口数量再大也不会形成并发探测风暴。
func selectQualityProbeCandidate(profiles []*Profile, interval time.Duration, now time.Time) *Profile {
	var selected *Profile
	for _, profile := range profiles {
		if profile == nil || !profile.Healthy || profile.ProxyURL == "" ||
			(profile.Mode == ProfileModeManaged && !profile.Running) {
			continue
		}
		if !profile.QualityCheckedAt.IsZero() && now.Before(profile.QualityCheckedAt.Add(interval)) {
			continue
		}
		if selected == nil || profile.QualityCheckedAt.Before(selected.QualityCheckedAt) {
			selected = profile
		}
	}
	return selected
}

// probeNextQualityProfile 在健康检查周期中错峰探测一个出口，维护少量近期实测
// 健康的备用出口；不会扫描写入 xAI 认证文件，也不会为账号建立持久代理绑定。
func (m *Manager) probeNextQualityProfile(q QualityConfig) {
	if !q.Probe.Enabled || strings.TrimSpace(q.Probe.Model) == "" {
		return
	}
	candidate := selectQualityProbeCandidate(m.stateStore().Profiles(), qualityProbeInterval(q), time.Now())
	if candidate != nil {
		_, _ = m.ProbeProfile(candidate.ID)
	}
}

// evaluateQualityTasks 由健康检查循环周期调用：自动清理、自动补充，并错峰
// 主动探测一个备用出口。清理先于补充执行，释放名额后补充判断更准确。
func (m *Manager) evaluateQualityTasks() {
	q := m.stateStore().Quality()
	if !q.Enabled {
		return
	}
	// 先把独立 xAI 活动出口从异常/降智节点迁走，再清理旧节点。
	_, _ = m.ensureXAIActiveProfile(false)
	if q.AutoPrune {
		m.autoPrune(q)
	}
	if q.AutoProvision {
		_ = m.autoProvision(q)
	}
	m.probeNextQualityProfile(q)
}

// autoProvision 健康（连通 + 未降智）出口不足时自动补充一个托管 WARP 出口，
// 通过现有健康出口注册以规避 Cloudflare 注册限流；带冷却时间防止 429 风暴。
func (m *Manager) autoProvision(q QualityConfig) error {
	profiles := m.stateStore().Profiles()
	if len(profiles) >= q.MaxProfiles {
		return nil
	}
	healthyManaged := 0
	var viaID string
	now := time.Now()
	for _, p := range profiles {
		if p.Mode != ProfileModeManaged {
			continue
		}
		if p.Healthy && !p.Degraded {
			if viaID == "" {
				viaID = p.ID
			}
			if !q.Probe.Enabled || qualityObservationFresh(p, q, now) {
				healthyManaged++
			}
		}
	}
	if healthyManaged >= q.MinHealthy {
		return nil
	}
	m.mu.Lock()
	cooldown := time.Duration(q.ProvisionCooldownMin) * time.Minute
	if cooldown <= 0 {
		cooldown = 15 * time.Minute
	}
	if !m.lastProvisionAt.IsZero() && time.Since(m.lastProvisionAt) < cooldown {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	count := 1
	for _, p := range profiles {
		if strings.HasPrefix(p.Name, "自动补充") {
			count++
		}
	}
	profile, err := m.CreateProfile(createProfileRequest{
		Name:        fmt.Sprintf("自动补充 %02d", count),
		Mode:        "managed",
		AutoStart:   true,
		RegisterVia: viaID,
		Origin:      "auto",
	})
	// 无论成败都记录尝试时间：注册限流（429）期间反复重试只会加重封禁。
	m.mu.Lock()
	m.lastProvisionAt = time.Now()
	if err != nil {
		m.provisionError = err.Error()
	}
	m.mu.Unlock()
	if err != nil {
		m.setLastError("自动补充出口失败: " + err.Error())
		return err
	}
	m.mu.Lock()
	m.provisionError = ""
	m.mu.Unlock()
	_ = m.CheckProfile(profile.ID)
	return nil
}

// autoPrune 自动清理降智代理：删除所有被打降智标记的托管出口
// （全部清理，不受数量限制），配合自动补充形成「降智即清、清后即补」。
// 被规则引用的出口不自动删除；降智是可恢复状态，但 xAI 降智守护的
// 语义是降智代理不保留，直接清掉由自动补充重建。
func (m *Manager) autoPrune(q QualityConfig) {
	if !q.AutoPrune {
		return
	}
	store := m.stateStore()
	profiles := store.Profiles()
	if len(profiles) == 0 {
		return
	}
	referenced := map[string]bool{}
	rules := store.Rules()
	if rules.GlobalProfileID != "" {
		referenced[rules.GlobalProfileID] = true
	}
	for _, rule := range rules.TypeRules {
		if rule.Enabled {
			referenced[rule.ProfileID] = true
		}
	}
	for _, rule := range rules.RegexRules {
		if rule.Enabled {
			referenced[rule.ProfileID] = true
		}
	}
	for _, id := range rules.ExactRules {
		if id != "" && id != exactDirect {
			if _, custom := customExactProxy(id); !custom {
				referenced[id] = true
			}
		}
	}
	for _, p := range profiles {
		if p == nil || p.Mode != ProfileModeManaged || !p.Degraded || referenced[p.ID] {
			continue
		}
		if err := m.DeleteProfile(p.ID); err == nil {
			m.setLastError("自动清理降智代理: " + p.Name)
		}
	}
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

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
	"sort"
	"strings"
	"sync"
	"time"
)

// 质量守护（Quality Guard）：通用、与供应商无关。
//
// 原理：共享出口 IP 被打穿时，AI 生成的输出 Token/s 会异常飙升（表现为"模型变笨"）。
// 插件被动观测 CPA usage 事件（任意 provider 都产生）的输出 token 数与耗时，
// 估算输出 TPS；TPS 超过阈值且连续多次则给对应出口打上降智标记（Degraded）。
// 路由分流与自动切换会跳过被标记的出口；标记可通过连续健康观测或主动探测恢复。
// 可选主动探测：新出口创建后先经该出口向任意 OpenAI 兼容端点发流式请求实测质量，
// 降智的出口打记号不投入使用，避免轮换时用上没有质量保证的 IP。

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
	if outputTokens <= 0 || tps <= 0 {
		return "unknown"
	}
	if q.MinOutputTokens > 0 && outputTokens < q.MinOutputTokens {
		return "ignored"
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
		// 账号未显式绑定出口（全局出口模式：认证文件无 proxy_url，
		// 流量经 CPA 全局中继）：归到当前全局出口统计。
		rules := store.Rules()
		if rules.GlobalProfileID != "" {
			profile = store.Profile(rules.GlobalProfileID)
		}
	}
	if profile == nil {
		return nil
	}
	tps := computeTPS(outTokens, durationMs, firstTokenMs, q.MinGenerationMs)
	class := classifyQualityTPS(tps, outTokens, q)
	return m.applyQualityObservation(profile.ID, tps, outTokens, class, "passive")
}

// applyQualityObservation 合并一次质量观测到 profile：
// degraded 连续达到阈值 → 打降智标记（立即落盘 + 评估故障转移）；
// healthy 连续恢复 → 清除标记。常规统计走防抖落盘，避免高频写盘。
func (m *Manager) applyQualityObservation(profileID string, tps float64, outTokens int64, class, source string) error {
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
	stateChanged := false
	switch class {
	case "degraded":
		profile.QualityStrikes++
		profile.QualityRecovery = 0
		if !profile.Degraded && profile.QualityStrikes >= q.ConsecutiveDegraded {
			profile.Degraded = true
			profile.DegradedAt = time.Now()
			profile.DegradedReason = fmt.Sprintf("连续 %d 次高输出 TPS（%.1f token/s，%s）", profile.QualityStrikes, tps, source)
			stateChanged = true
		}
	case "healthy":
		profile.QualityStrikes = 0
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
	return nil
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
// 立即切换到其他健康出口（异步，不阻塞 usage 热路径）。
// 降智检测与降智切换一体：质量守护开启即生效，无需额外开关。
func (m *Manager) evaluateDegradedFailover(profile *Profile) {
	if profile == nil || !profile.Degraded {
		return
	}
	if !m.stateStore().Quality().Enabled {
		return
	}
	rules := m.stateStore().Rules()
	if rules.GlobalProfileID != profile.ID {
		return
	}
	go func() {
		_, _ = m.EvaluateAutoSwitch(false)
	}()
}

// probeQualityResult 主动探测结果（暴露给 UI 与管理 API）。
type probeQualityResult struct {
	ProfileID      string  `json:"profile_id"`
	Classification string  `json:"classification"` // healthy / degraded / unknown / error
	TPS            float64 `json:"tps"`
	OutputTokens   int64   `json:"output_tokens"`
	DurationMs     int64   `json:"duration_ms"`
	FirstTokenMs   int64   `json:"first_token_ms"`
	Error          string  `json:"error,omitempty"`
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
	AccessToken string
	BaseURL     string
	Headers     map[string]string
	Expired     bool
}

// listXAIAccounts 从 CPA auth 目录筛选 xai/grok 类账号，
// 供主动质量探测经被测出口发送真实 xAI 请求。
func listXAIAccounts(limit int) []xaiAccount {
	if limit <= 0 {
		limit = 8
	}
	entries, err := callHostAuthList()
	if err != nil {
		return nil
	}
	out := make([]xaiAccount, 0, limit)
	for _, entry := range entries.Files {
		provider := strings.ToLower(entry.Provider)
		authType := strings.ToLower(entry.Type)
		if !strings.Contains(provider, "xai") && !strings.Contains(provider, "grok") &&
			!strings.Contains(authType, "xai") && !strings.Contains(authType, "grok") {
			continue
		}
		if entry.AuthIndex == "" {
			continue
		}
		got, errGet := callHostAuthGet(entry.AuthIndex)
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
		out = append(out, account)
		if len(out) >= limit {
			break
		}
	}
	return out
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
// 归因与 usage 事件一致：账号未绑定出口时归到当前全局出口；
// "不使用代理"（全局出口为空）时不做观测，尊重用户选择。
// ---------------------------------------------------------------------------

type streamTrack struct {
	startAt      time.Time
	firstTokenAt time.Time
	chars        int
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
	done, chars := extractStreamChunkMetrics(body)
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
func extractStreamChunkMetrics(body []byte) (done bool, chars int) {
	raw := body
	if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body))); err == nil && len(decoded) > 0 {
		raw = decoded
	}
	text := string(raw)
	trimmed := strings.TrimSpace(text)
	if trimmed == "[DONE]" || strings.HasSuffix(trimmed, "data: [DONE]") {
		return true, 0
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
					continue
				}
				if content, _ := delta["content"].(string); content != "" {
					chars += len([]rune(content))
				}
				if reasoning, _ := delta["reasoning_content"].(string); reasoning != "" {
					chars += len([]rune(reasoning))
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
			}
		}
	}
	// 纯 JSON（无 SSE 前缀）。
	if strings.HasPrefix(trimmed, "{") {
		handleJSON(trimmed)
		return done, chars
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
	return done, chars
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
	class := classifyQualityTPS(tps, outTokens, q)
	if class == "unknown" || class == "ignored" {
		return
	}
	// 归因：全局出口模式（与被动 usage 的 fallback 一致）；
	// 全局"不使用代理"时，若质量守护自动绑定了 XAI 认证文件，
	// 归到自动绑定出口（所有自动绑定指向同一健康托管出口）。
	profileID := m.stateStore().Rules().GlobalProfileID
	if profileID == "" {
		for _, proxy := range m.stateStore().AutoBoundAuths() {
			for _, p := range m.stateStore().Profiles() {
				if p.ProxyURL == proxy && !p.Degraded {
					profileID = p.ID
					break
				}
			}
			if profileID != "" {
				break
			}
		}
	}
	if profileID == "" {
		return
	}
	profile := m.stateStore().Profile(profileID)
	if profile == nil {
		return
	}
	_ = m.applyQualityObservation(profile.ID, tps, outTokens, class, "stream")
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
// 发一个流式请求，实测输出 TPS 并应用观测。账号类失败（401/403）换下一个
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
	accounts := listXAIAccounts(8)
	if len(accounts) == 0 {
		return res, errors.New("没有可用的 CPA xAI 账号，无法主动探测（探测复用 xAI 账号）")
	}
	client, err := httpClientThroughProfile(profile.ProxyURL, time.Duration(probe.TimeoutSeconds)*time.Second)
	if err != nil {
		res.Classification = "error"
		res.Error = err.Error()
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
	for _, account := range accounts {
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
			continue
		}
		if response.StatusCode >= 400 {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
			_ = response.Body.Close()
			lastErr = fmt.Sprintf("probe upstream HTTP %d: %s", response.StatusCode, truncateString(string(body), 160))
			res.DurationMs = time.Since(start).Milliseconds()
			// 账号/配额类错误属于账号而非出口，换下一个账号再试。
			if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
				continue
			}
			res.Classification = "error"
			res.Error = lastErr
			return res, nil
		}

		var (
			firstTokenAt time.Time
			contentLen   int
			usageOut     int64
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
			}
			if choices, ok := chunk["choices"].([]any); ok {
				for _, c := range choices {
					choice, _ := c.(map[string]any)
					delta, _ := choice["delta"].(map[string]any)
					if delta == nil {
						continue
					}
					content, _ := delta["content"].(string)
					if content != "" {
						if firstTokenAt.IsZero() {
							firstTokenAt = time.Now()
						}
						contentLen += len([]rune(content))
					}
					if reasoning, _ := delta["reasoning_content"].(string); reasoning != "" {
						if firstTokenAt.IsZero() {
							firstTokenAt = time.Now()
						}
						contentLen += len([]rune(reasoning))
					}
				}
			}
		}
		_ = response.Body.Close()
		if scanErr := scanner.Err(); scanErr != nil {
			lastErr = "probe stream failed: " + truncateString(scanErr.Error(), 120)
			res.DurationMs = time.Since(start).Milliseconds()
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
		res.TPS = computeTPS(outTokens, res.DurationMs, res.FirstTokenMs, q.MinGenerationMs)
		res.Classification = classifyQualityTPS(res.TPS, outTokens, q)
		if res.Classification == "unknown" {
			lastErr = "探测无输出"
			continue
		}
		if res.Classification == "ignored" {
			res.Classification = "unknown"
		}
		_ = m.applyQualityObservation(profileID, res.TPS, outTokens, res.Classification, "probe")
		return res, nil
	}
	res.Classification = "error"
	if lastErr == "" {
		lastErr = "所有 xAI 账号探测失败"
	}
	res.Error = lastErr
	return res, nil
}

// evaluateQualityTasks 由健康检查循环周期调用：自动补充健康出口 + 清理历史出口。
func (m *Manager) evaluateQualityTasks() {
	q := m.stateStore().Quality()
	if !q.Enabled {
		return
	}
	if q.AutoProvision {
		_ = m.autoProvision(q)
	}
	if q.AutoPrune {
		m.autoPrune(q)
	}
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
	for _, p := range profiles {
		if p.Mode != ProfileModeManaged {
			continue
		}
		if p.Healthy && !p.Degraded {
			healthyManaged++
			if viaID == "" {
				viaID = p.ID
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
	if q.Probe.Enabled {
		go func(id string) { _, _ = m.ProbeProfile(id) }(profile.ID)
	}
	return nil
}

// autoPrune 清理历史出口：
//  1. 超过 max_profiles 时删除最旧且不健康的托管出口（跳过全局与规则引用）；
//  2. prune_unhealthy_minutes > 0 时，删除持续不健康超过该时长的闲置出口。
func (m *Manager) autoPrune(q QualityConfig) {
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
	sorted := append([]*Profile(nil), profiles...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })

	deletable := func(p *Profile) bool {
		return p != nil && p.Mode == ProfileModeManaged && !referenced[p.ID] && !p.Healthy
	}
	// 超过上限：优先删除最旧的不健康托管出口。
	if len(sorted) > q.MaxProfiles {
		for _, p := range sorted {
			if deletable(p) {
				if err := m.DeleteProfile(p.ID); err == nil {
					m.setLastError("自动清理出口: " + p.Name)
				}
				return
			}
		}
	}
	// 持续不健康的闲置出口按时间清理。
	if q.PruneUnhealthyMinutes > 0 {
		cutoff := time.Now().Add(-time.Duration(q.PruneUnhealthyMinutes) * time.Minute)
		for _, p := range sorted {
			if !deletable(p) {
				continue
			}
			if !p.LastChecked.IsZero() && p.LastChecked.Before(cutoff) && time.Since(p.CreatedAt) > 10*time.Minute {
				if err := m.DeleteProfile(p.ID); err == nil {
					m.setLastError("自动清理长期异常出口: " + p.Name)
				}
				return
			}
		}
	}
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

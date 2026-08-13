package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestComputeTPS(t *testing.T) {
	if got := computeTPS(100, 20000, 0, 1000); got != 5 {
		t.Fatalf("expected 5 tps, got %v", got)
	}
	// 短窗口（首 token 接近总时长）退化为总时长计算，防止虚高。
	short := computeTPS(100, 1200, 1100, 1000)
	if short <= 0 {
		t.Fatalf("short window should still produce a tps, got %v", short)
	}
	if got := computeTPS(0, 1000, 0, 1000); got != 0 {
		t.Fatalf("zero tokens must yield zero tps, got %v", got)
	}
	if got := computeTPS(100, 500, 0, 1000); got != 0 {
		t.Fatalf("generation shorter than min window must yield zero tps, got %v", got)
	}
}

func TestUsageTokensUsesAuthoritativeOutputWithoutDoubleCountingReasoning(t *testing.T) {
	usage := map[string]any{
		"output_tokens":     float64(180),
		"completion_tokens": float64(180),
		"total_tokens":      float64(230),
		"completion_tokens_details": map[string]any{
			"reasoning_tokens": float64(75),
		},
	}
	if got := usageTokens(usage); got != 180 {
		t.Fatalf("reasoning is already included in output and must not be added twice: got %d", got)
	}
	if got := usageTokens(map[string]any{"reasoning_tokens": float64(792), "total_tokens": float64(985)}); got != 792 {
		t.Fatalf("reasoning-only usage must remain usable as output evidence: got %d", got)
	}
	if got := usageTokens(map[string]any{"total_tokens": float64(985)}); got != 0 {
		t.Fatalf("total_tokens includes input and cannot be used as output: got %d", got)
	}
}

func TestClassifyQualityTPS(t *testing.T) {
	q := defaultQualityConfig()
	if class := classifyQualityTPS(600, 200, q); class != "degraded" {
		t.Fatalf("expected degraded, got %s", class)
	}
	if class := classifyQualityTPS(100, 200, q); class != "healthy" {
		t.Fatalf("expected healthy, got %s", class)
	}
	if class := classifyQualityTPS(600, 20, q); class != "ignored" {
		t.Fatalf("small output must be ignored, got %s", class)
	}
	if class := classifyQualityTPS(0, 200, q); class != "unknown" {
		t.Fatalf("zero tps must be unknown, got %s", class)
	}
}

func TestThinkingQualitySignalAndUsageFallbacks(t *testing.T) {
	q := defaultQualityConfig()
	if got := classifyQualitySignal(20, 200, false, q); got != "degraded" {
		t.Fatalf("missing thinking=%q", got)
	}
	if got := classifyQualitySignal(20, 200, true, q); got != "healthy" {
		t.Fatalf("thinking present=%q", got)
	}
	if got := classifyQualitySignal(20, q.MinOutputTokens-1, false, q); got != "ignored" {
		t.Fatalf("short missing-thinking sample=%q", got)
	}
	if !recordHasThinking(map[string]any{"Detail": map[string]any{"reasoning_tokens": float64(3)}}) {
		t.Fatal("reasoning_tokens should prove thinking")
	}
	if !recordHasThinking(map[string]any{"Detail": map[string]any{
		"completion_tokens_details": map[string]any{"reasoning_tokens": float64(223)},
	}}) {
		t.Fatal("nested completion token details should prove thinking")
	}
	if !recordHasThinking(map[string]any{"delta": map[string]any{"thinking_content": "step"}}) {
		t.Fatal("thinking_content should prove thinking")
	}
}

func TestStreamMetricsRecognizeNestedReasoningTokens(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": "stop"}},
		"usage": map[string]any{
			"completion_tokens_details": map[string]any{"reasoning_tokens": float64(89)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	done, _, hasThinking := extractStreamChunkMetrics(body)
	if !done || !hasThinking {
		t.Fatalf("final stream usage should preserve reasoning evidence: done=%v hasThinking=%v", done, hasThinking)
	}
}

func TestDegradedObservationRefreshesStaleReason(t *testing.T) {
	manager := newTestManager(t)
	profile := &Profile{
		ID:             "warp-1",
		Mode:           ProfileModeExternal,
		ProxyURL:       "socks5://127.0.0.1:41001",
		Healthy:        true,
		Degraded:       true,
		DegradedReason: "旧的探测超时原因",
	}
	if err := manager.store.AddProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err := manager.applyQualitySignal(profile.ID, 755, 792, true, "degraded", "probe"); err != nil {
		t.Fatal(err)
	}
	got := manager.store.Profile(profile.ID)
	if !strings.Contains(got.DegradedReason, "高输出 TPS") || strings.Contains(got.DegradedReason, "超时") {
		t.Fatalf("latest verified degradation must replace the stale reason, got %q", got.DegradedReason)
	}
}

func TestProbeAccountAndTransportErrorsStaySeparated(t *testing.T) {
	if !shouldRetryProbeAccount(true, 429, `{"error":"free-usage-exhausted"}`) {
		t.Fatal("free quota exhaustion should switch probe account")
	}
	if shouldRetryProbeAccount(false, 401, "expired") {
		t.Fatal("last account cannot be retried")
	}
	if !isProbeUnstableErr(assertError("unexpected EOF")) {
		t.Fatal("broken stream should be treated as unstable egress")
	}
}

func TestQualityPolicyMigrationEnablesNewGuardsOnce(t *testing.T) {
	legacy := QualityConfig{Enabled: true, SoftTPS: 500, ConsecutiveDegraded: 3}
	migrated := normalizeQualityConfig(legacy)
	if migrated.PolicySchema != qualityPolicySchema || !migrated.ThinkingGuard ||
		!migrated.ThinkingCrossVerify || !migrated.SoftCrossVerify {
		t.Fatalf("legacy policy not migrated: %+v", migrated)
	}
	migrated.ThinkingGuard = false
	migrated.ThinkingCrossVerify = false
	migrated.SoftCrossVerify = false
	normalizedAgain := normalizeQualityConfig(migrated)
	if normalizedAgain.ThinkingGuard || normalizedAgain.ThinkingCrossVerify || normalizedAgain.SoftCrossVerify {
		t.Fatalf("explicit schema-2 switches were overwritten: %+v", normalizedAgain)
	}
}

func TestQualityPolicyMigrationUpdatesLegacyProbeModelOnce(t *testing.T) {
	legacy := defaultQualityConfig()
	legacy.PolicySchema = 2
	legacy.Probe.Model = "grok-4"
	legacy.ThinkingGuard = false
	legacy.ThinkingCrossVerify = false
	legacy.SoftCrossVerify = false

	migrated := normalizeQualityConfig(legacy)
	if migrated.PolicySchema != qualityPolicySchema || migrated.Probe.Model != "grok-4.6" || migrated.HardTPS != 1000 || migrated.ConsecutiveErrors != 3 {
		t.Fatalf("legacy probe model was not migrated: %+v", migrated)
	}
	if migrated.ThinkingGuard || migrated.ThinkingCrossVerify || migrated.SoftCrossVerify {
		t.Fatalf("schema-2 guard choices must survive the model migration: %+v", migrated)
	}

	migrated.Probe.Model = "grok-4"
	if got := normalizeQualityConfig(migrated).Probe.Model; got != "grok-4" {
		t.Fatalf("current-schema explicit model choice was overwritten: %q", got)
	}
}

func TestXAIExtensionDefaultsToDisabled(t *testing.T) {
	if q := defaultQualityConfig(); q.Enabled {
		t.Fatal("optional xAI extension must not intercept core routing by default")
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	manager := NewManager()
	manager.cfg = defaultConfig()
	manager.store = NewStateStore(t.TempDir())
	// 旧回归用例验证 v0.6 的 TPS + 普通全局出口语义；新路由与 thinking
	// 行为由本文件和 egress_router_test.go 的专门用例覆盖。
	quality := manager.store.Quality()
	quality.Enabled = true
	quality.Route.Mode = XAIRouteModeFollowGlobal
	quality.ThinkingGuard = false
	quality.ThinkingCrossVerify = false
	quality.SoftCrossVerify = false
	if err := manager.store.SetQuality(quality); err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestReloadClearsOrphanedCrossVerificationState(t *testing.T) {
	dataDir := t.TempDir()
	store := NewStateStore(dataDir)
	profile := &Profile{
		ID:                     "verifying-profile",
		Mode:                   ProfileModeExternal,
		ProxyURL:               "socks5://127.0.0.1:41001",
		Healthy:                true,
		QualityClassification:  "verifying",
		QualityError:           "响应缺少 thinking，等待主动交叉验证",
		QualityStrikes:         2,
		QualityThinkingStrikes: 1,
	}
	if err := store.AddProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded := NewStateStore(dataDir)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	got := reloaded.Profile(profile.ID)
	if got.QualityClassification != "error" || got.Degraded ||
		got.QualityStrikes != 0 || got.QualityThinkingStrikes != 0 ||
		!strings.Contains(got.QualityError, "重载") {
		t.Fatalf("a reload cannot resume an in-memory cross verification task: %+v", got)
	}
}

func startTestSOCKSForwarder(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			client, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer client.Close()
				destination, socksErr := acceptSOCKS5(client)
				if socksErr != nil {
					return
				}
				remote, dialErr := net.DialTimeout("tcp", destination, time.Second)
				if dialErr != nil {
					_ = writeSOCKSReply(client, 0x05, nil)
					return
				}
				defer remote.Close()
				_ = writeSOCKSReply(client, 0, remote.LocalAddr())
				copyStream(client, remote)
			}()
		}
	}()
	return listener.Addr().String()
}

func addQualityStandbys(t *testing.T, manager *Manager, prefix string) {
	t.Helper()
	for _, profile := range []*Profile{
		{ID: prefix + "-1", Name: prefix + "-1", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41991", Healthy: true},
		{ID: prefix + "-2", Name: prefix + "-2", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41992", Healthy: true},
	} {
		if err := manager.store.AddProfile(profile); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHardTPSImmediatelyDegradesEgress(t *testing.T) {
	manager := newTestManager(t)
	target := &Profile{ID: "warp-hard", Name: "hard", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true}
	for _, profile := range []*Profile{
		target,
		{ID: "warp-standby-1", Name: "standby-1", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41002", Healthy: true},
		{ID: "warp-standby-2", Name: "standby-2", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41003", Healthy: true},
	} {
		if err := manager.store.AddProfile(profile); err != nil {
			t.Fatal(err)
		}
	}
	oldResolver := authProxyResolver
	authProxyResolver = func() map[string]string { return map[string]string{"idx-hard": target.ProxyURL} }
	defer func() { authProxyResolver = oldResolver }()

	q := manager.store.Quality()
	q.SoftTPS = 500
	q.HardTPS = 1000
	q.ConsecutiveDegraded = 99
	q.MinHealthy = 2
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}

	// 10s 总耗时、1s TTFT、9000 输出 Token => 1000 TPS。硬阈值是
	// 已确认的强降智证据，不等待软阈值累计，也不触发主动交叉复测。
	if err := manager.HandleUsage(map[string]any{
		"Provider": "xai", "AuthID": "idx-hard", "Latency": float64(10 * 1e9), "TTFT": float64(1 * 1e9),
		"Detail": map[string]any{"OutputTokens": float64(9000), "reasoning_tokens": float64(1200)},
	}); err != nil {
		t.Fatal(err)
	}
	got := manager.store.Profile(target.ID)
	if !got.Degraded || got.QualityClassification != "hard" {
		t.Fatalf("hard TPS must immediately isolate the egress: %+v", got)
	}
}

func TestConsecutiveProbeErrorsDegradeEgress(t *testing.T) {
	oldList, oldGet := authListForProbe, authGetForProbe
	defer func() { authListForProbe, authGetForProbe = oldList, oldGet }()
	authListForProbe = func() (hostAuthListResponse, error) {
		return hostAuthListResponse{Files: []hostAuthFileEntry{{AuthIndex: "probe", Provider: "xai", Status: "active"}}}, nil
	}
	authGetForProbe = func(string) (hostAuthGetResponse, error) {
		return hostAuthGetResponse{JSON: json.RawMessage(`{"access_token":"test-token","base_url":"https://api.x.ai/v1"}`)}, nil
	}

	manager := newTestManager(t)
	manager.invalidateXAIProbeAccounts()
	target := &Profile{ID: "warp-error", Name: "error", Mode: ProfileModeExternal, ProxyURL: "invalid-proxy-url", Healthy: true}
	for _, profile := range []*Profile{
		target,
		{ID: "warp-error-standby-1", Name: "standby-1", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41012", Healthy: true},
		{ID: "warp-error-standby-2", Name: "standby-2", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41013", Healthy: true},
	} {
		if err := manager.store.AddProfile(profile); err != nil {
			t.Fatal(err)
		}
	}
	q := manager.store.Quality()
	q.ConsecutiveErrors = 3
	q.MinHealthy = 2
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}

	for attempt := 1; attempt <= q.ConsecutiveErrors; attempt++ {
		_, _ = manager.ProbeProfile(target.ID)
		got := manager.store.Profile(target.ID)
		if attempt < q.ConsecutiveErrors && got.Degraded {
			t.Fatalf("probe error %d/%d must not isolate early: %+v", attempt, q.ConsecutiveErrors, got)
		}
	}
	got := manager.store.Profile(target.ID)
	if !got.Degraded || got.QualityErrorStrikes != q.ConsecutiveErrors {
		t.Fatalf("consecutive probe errors must isolate the egress: %+v", got)
	}
}

func TestUnavailableProbeAccountsDoNotConsumeEgressErrors(t *testing.T) {
	oldList := authListForProbe
	defer func() { authListForProbe = oldList }()
	authListForProbe = func() (hostAuthListResponse, error) { return hostAuthListResponse{}, nil }

	manager := newTestManager(t)
	manager.invalidateXAIProbeAccounts()
	profile := &Profile{ID: "warp-no-accounts", Name: "no-accounts", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41019", Healthy: true}
	if err := manager.store.AddProfile(profile); err != nil {
		t.Fatal(err)
	}
	q := manager.store.Quality()
	q.ConsecutiveErrors = 1
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}

	_, _ = manager.ProbeProfile(profile.ID)
	got := manager.store.Profile(profile.ID)
	if got.QualityErrorStrikes != 0 || got.Degraded {
		t.Fatalf("missing probe accounts are not egress evidence: %+v", got)
	}
}

func TestMinHealthySuppressesHardIsolation(t *testing.T) {
	manager := newTestManager(t)
	target := &Profile{ID: "warp-protected", Name: "protected", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41021", Healthy: true}
	standby := &Profile{ID: "warp-only-standby", Name: "only-standby", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41022", Healthy: true}
	for _, profile := range []*Profile{target, standby} {
		if err := manager.store.AddProfile(profile); err != nil {
			t.Fatal(err)
		}
	}
	oldResolver := authProxyResolver
	authProxyResolver = func() map[string]string { return map[string]string{"idx-protected": target.ProxyURL} }
	defer func() { authProxyResolver = oldResolver }()

	q := manager.store.Quality()
	q.HardTPS = 1000
	q.MinHealthy = 2
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	if err := manager.HandleUsage(map[string]any{
		"Provider": "xai", "AuthID": "idx-protected", "Latency": float64(10 * 1e9), "TTFT": float64(1 * 1e9),
		"Detail": map[string]any{"OutputTokens": float64(9000), "reasoning_tokens": float64(1200)},
	}); err != nil {
		t.Fatal(err)
	}
	got := manager.store.Profile(target.ID)
	if got.Degraded || got.QualityClassification != "suppressed" || !strings.Contains(got.QualityError, "最低健康出口") {
		t.Fatalf("hard isolation must be suppressed when it would violate min_healthy: %+v", got)
	}
}

func TestHandleUsageMarksDegradedAndRecovers(t *testing.T) {
	manager := newTestManager(t)
	profile := &Profile{
		ID: "warp-1", Name: "p1", Mode: ProfileModeExternal,
		ProxyURL: "socks5://127.0.0.1:41001", Healthy: true,
	}
	if err := manager.store.AddProfile(profile); err != nil {
		t.Fatal(err)
	}
	addQualityStandbys(t, manager, "usage-standby")
	oldResolver := authProxyResolver
	authProxyResolver = func() map[string]string { return map[string]string{"idx-1": profile.ProxyURL} }
	defer func() { authProxyResolver = oldResolver }()

	q := manager.store.Quality()
	q.ConsecutiveDegraded = 2
	q.HardTPS = 2000
	q.RecoveryObservations = 2
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}

	usage := func(tpsTokens int64) map[string]any {
		// Latency 10s（ns），TTFT 1s：生成窗口 9s → TPS = tokens / 9
		return map[string]any{
			"Provider":     "xai",
			"AuthID":       "idx-1",
			"Latency":      float64(10 * 1e9),
			"TTFT":         float64(1 * 1e9),
			"Detail":       map[string]any{"OutputTokens": float64(tpsTokens)},
			"OutputTokens": float64(tpsTokens),
		}
	}
	if err := manager.HandleUsage(usage(9000)); err != nil {
		t.Fatal(err)
	}
	if got := manager.store.Profile(profile.ID); got.Degraded {
		t.Fatalf("must not degrade on first strike: %+v", got)
	}
	if err := manager.HandleUsage(usage(9000)); err != nil {
		t.Fatal(err)
	}
	got := manager.store.Profile(profile.ID)
	if !got.Degraded || got.DegradedReason == "" {
		t.Fatalf("expected degraded after 2 consecutive high TPS, got %+v", got)
	}
	// 恢复：连续 2 次健康观测清除标记。
	for i := 0; i < q.RecoveryObservations; i++ {
		if err := manager.HandleUsage(usage(90)); err != nil {
			t.Fatal(err)
		}
	}
	if got := manager.store.Profile(profile.ID); got.Degraded {
		t.Fatalf("expected recovery, got %+v", got)
	}
}

func TestHandleUsageSkipsFailedAndTinyOutput(t *testing.T) {
	manager := newTestManager(t)
	profile := &Profile{ID: "warp-1", Name: "p1", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true}
	if err := manager.store.AddProfile(profile); err != nil {
		t.Fatal(err)
	}
	addQualityStandbys(t, manager, "skip-standby")
	oldResolver := authProxyResolver
	authProxyResolver = func() map[string]string { return map[string]string{"idx-1": profile.ProxyURL} }
	defer func() { authProxyResolver = oldResolver }()

	q := manager.store.Quality()
	q.ConsecutiveDegraded = 1
	q.MinOutputTokens = 32
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	if err := manager.HandleUsage(map[string]any{"Provider": "xai", "AuthID": "idx-1", "Latency": 1e9, "TTFT": 1e8, "Detail": map[string]any{"OutputTokens": float64(500)}, "Failed": true}); err != nil {
		t.Fatal(err)
	}
	if got := manager.store.Profile(profile.ID); got.Degraded {
		t.Fatalf("failed requests must not degrade the egress: %+v", got)
	}
	if err := manager.HandleUsage(map[string]any{"Provider": "xai", "AuthID": "idx-1", "Latency": 1e9, "TTFT": 1e8, "Detail": map[string]any{"OutputTokens": float64(5)}}); err != nil {
		t.Fatal(err)
	}
	if got := manager.store.Profile(profile.ID); got.Degraded {
		t.Fatalf("tiny outputs must not degrade the egress: %+v", got)
	}
	if err := manager.HandleUsage(map[string]any{"Provider": "xai", "AuthID": "idx-1", "Latency": 1e9, "TTFT": 1e8, "Detail": map[string]any{"OutputTokens": float64(500)}}); err != nil {
		t.Fatal(err)
	}
	if got := manager.store.Profile(profile.ID); !got.Degraded {
		t.Fatalf("expected degraded, got %+v", got)
	}
}

func TestResolveRouteSkipsDegraded(t *testing.T) {
	manager := newTestManager(t)
	profiles := []*Profile{
		{ID: "global", Name: "global", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true, Running: true},
		{ID: "type", Name: "type", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41002", Healthy: true, Running: true},
		{ID: "exact", Name: "exact", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41003", Healthy: true, Running: true, Degraded: true},
	}
	for _, p := range profiles {
		if err := manager.store.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	rules := Rules{
		GlobalProfileID: "global",
		TypeRules:       []TypeRule{{Key: "codex", ProfileID: "type", Enabled: true}},
		ExactRules:      map[string]string{"idx-1": "exact"},
	}
	if err := manager.store.SetRules(rules); err != nil {
		t.Fatal(err)
	}
	entry := hostAuthFileEntry{AuthIndex: "idx-1", Name: "a.json", Provider: "codex"}
	// exact 规则命中降智出口 → 跳过，落到类型规则。
	if route := manager.resolveRoute(entry); route.ProfileID != "type" || route.RuleType != "type" {
		t.Fatalf("expected fallback to type rule, got %+v", route)
	}
	rules.TypeRules[0].Enabled = false
	_ = manager.store.SetRules(rules)
	// 所有规则都跳过/失效 → 落到全局。
	if route := manager.resolveRoute(entry); route.ProfileID != "global" || route.RuleType != "global" {
		t.Fatalf("expected global route, got %+v", route)
	}
}

func TestResolveRouteGlobalDegradedFallback(t *testing.T) {
	manager := newTestManager(t)
	profiles := []*Profile{
		{ID: "bad", Name: "bad", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true, Running: true, Degraded: true},
		{ID: "good", Name: "good", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41002", Healthy: true, Running: true},
	}
	for _, p := range profiles {
		if err := manager.store.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.store.SetGlobalProfile("bad"); err != nil {
		t.Fatal(err)
	}
	entry := hostAuthFileEntry{AuthIndex: "idx-1", Name: "a.json"}
	route := manager.resolveRoute(entry)
	if route.ProfileID != "good" || route.RuleType != "global-fallback" {
		t.Fatalf("expected global-fallback to good, got %+v", route)
	}
	if route.ProxyURL != "socks5://127.0.0.1:41002" {
		t.Fatalf("fallback must carry the healthy proxy, got %q", route.ProxyURL)
	}
}

func TestEvaluateAutoSwitchSkipsDegradedAndFailsOver(t *testing.T) {
	manager := newTestManager(t)
	profiles := []*Profile{
		{ID: "a", Name: "a", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true, ExitIP: "203.0.113.1"},
		{ID: "b", Name: "b", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41002", Healthy: true, ExitIP: "203.0.113.2", Degraded: true},
		{ID: "c", Name: "c", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41003", Healthy: true, ExitIP: "203.0.113.3", QualityClassification: "healthy", QualityCheckedAt: time.Now()},
	}
	for _, p := range profiles {
		if err := manager.store.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.store.SetGlobalProfile("a"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveAutoSwitch(AutoSwitchConfig{Enabled: true, FailoverEnabled: true}); err != nil {
		t.Fatal(err)
	}
	// 手动强制切换：候选 b 降智必须被跳过。
	selected, err := manager.EvaluateAutoSwitch(true)
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.ID != "c" {
		t.Fatalf("expected c (b is degraded), got %+v", selected)
	}
	// 当前全局出口降智：自动故障转移。
	if err := manager.store.SetGlobalProfile("a"); err != nil {
		t.Fatal(err)
	}
	bad := manager.store.Profile("a")
	bad.Degraded = true
	bad.DegradedAt = time.Now()
	bad.DegradedReason = "test"
	if err := manager.store.UpdateProfile(bad); err != nil {
		t.Fatal(err)
	}
	selected, err = manager.EvaluateAutoSwitch(false)
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.ID != "c" || manager.store.Rules().GlobalProfileID != "c" {
		t.Fatalf("expected failover to c due to degraded global, got %+v", selected)
	}
	if reason := manager.store.AutoSwitch().LastReason; reason != "degraded" {
		t.Fatalf("expected reason degraded, got %q", reason)
	}
}

func TestEvaluateAutoSwitchDegradedFailoverIndependent(t *testing.T) {
	manager := newTestManager(t)
	profiles := []*Profile{
		{ID: "a", Name: "a", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true, ExitIP: "203.0.113.1"},
		{ID: "b", Name: "b", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41002", Healthy: true, ExitIP: "203.0.113.2", QualityClassification: "healthy", QualityCheckedAt: time.Now()},
	}
	for _, p := range profiles {
		if err := manager.store.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.store.SetGlobalProfile("a"); err != nil {
		t.Fatal(err)
	}
	// 自动切换总开关与故障转移都关闭：仅依赖 xAI 降智守护（默认开启）。
	if err := manager.SaveAutoSwitch(AutoSwitchConfig{}); err != nil {
		t.Fatal(err)
	}
	// 无降智：不触发任何切换（自动切换未启用）。
	selected, err := manager.EvaluateAutoSwitch(false)
	if err != nil {
		t.Fatal(err)
	}
	if selected != nil {
		t.Fatalf("auto switch disabled must not switch, got %+v", selected)
	}
	// 全局出口降智：仅凭降智守护执行切换，但候选必须近期实测健康。
	bad := manager.store.Profile("a")
	bad.Degraded = true
	bad.DegradedAt = time.Now()
	bad.DegradedReason = "test"
	if err := manager.store.UpdateProfile(bad); err != nil {
		t.Fatal(err)
	}
	selected, err = manager.EvaluateAutoSwitch(false)
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.ID != "b" || manager.store.Rules().GlobalProfileID != "b" {
		t.Fatalf("expected degraded failover to b with only quality guard on, got %+v", selected)
	}
	if reason := manager.store.AutoSwitch().LastReason; reason != "degraded" {
		t.Fatalf("expected reason degraded, got %q", reason)
	}
}

func TestEvaluateAutoSwitchQualityDisabledNoDegradedFailover(t *testing.T) {
	manager := newTestManager(t)
	profiles := []*Profile{
		{ID: "a", Name: "a", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true, ExitIP: "203.0.113.1"},
		{ID: "b", Name: "b", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41002", Healthy: true, ExitIP: "203.0.113.2"},
	}
	for _, p := range profiles {
		if err := manager.store.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.store.SetGlobalProfile("a"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveAutoSwitch(AutoSwitchConfig{}); err != nil {
		t.Fatal(err)
	}
	q := manager.store.Quality()
	q.Enabled = false
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	bad := manager.store.Profile("a")
	bad.Degraded = true
	bad.DegradedAt = time.Now()
	bad.DegradedReason = "test"
	if err := manager.store.UpdateProfile(bad); err != nil {
		t.Fatal(err)
	}
	selected, err := manager.EvaluateAutoSwitch(false)
	if err != nil {
		t.Fatal(err)
	}
	if selected != nil || manager.store.Rules().GlobalProfileID != "a" {
		t.Fatalf("quality guard disabled must not switch on degraded, got %+v", selected)
	}
}

func TestEvaluateAutoSwitchRespectsNoProxy(t *testing.T) {
	manager := newTestManager(t)
	profiles := []*Profile{
		{ID: "a", Name: "a", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true, ExitIP: "203.0.113.1"},
	}
	for _, p := range profiles {
		if err := manager.store.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	// 用户显式选择"不使用代理"（GlobalProfileID 为空）。
	if err := manager.SaveAutoSwitch(AutoSwitchConfig{Enabled: true, FailoverEnabled: true}); err != nil {
		t.Fatal(err)
	}
	// 手动执行与周期评估都不应自动选出口。
	for i := 0; i < 2; i++ {
		selected, err := manager.EvaluateAutoSwitch(i == 1)
		if err != nil {
			t.Fatal(err)
		}
		if selected != nil || manager.store.Rules().GlobalProfileID != "" {
			t.Fatalf("auto switch must not override no-proxy choice, got %+v", selected)
		}
	}
}

func TestAutoPruneClearsAllDegraded(t *testing.T) {
	manager := newTestManager(t)
	now := time.Now()
	profiles := []*Profile{
		{ID: "d1", Name: "d1", Mode: ProfileModeManaged, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true, Degraded: true, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "d2", Name: "d2", Mode: ProfileModeManaged, ProxyURL: "socks5://127.0.0.1:41002", Healthy: true, Degraded: true, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "ok", Name: "ok", Mode: ProfileModeManaged, ProxyURL: "socks5://127.0.0.1:41003", Healthy: true, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "global", Name: "global", Mode: ProfileModeManaged, ProxyURL: "socks5://127.0.0.1:41004", Healthy: true, Degraded: true, CreatedAt: now.Add(-24 * time.Hour)},
	}
	for _, p := range profiles {
		if err := manager.store.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.store.SetGlobalProfile("global"); err != nil {
		t.Fatal(err)
	}
	q := manager.store.Quality()
	q.AutoPrune = true
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	manager.autoPrune(q)
	if manager.store.Profile("d1") != nil || manager.store.Profile("d2") != nil {
		t.Fatal("all degraded egress must be cleared")
	}
	if manager.store.Profile("global") == nil {
		t.Fatal("degraded egress referenced by rules must not be auto-deleted")
	}
	if manager.store.Profile("ok") == nil {
		t.Fatal("non-degraded egress must survive")
	}
}

func TestHandleUsageOnlyTracksXAIProvider(t *testing.T) {
	manager := newTestManager(t)
	profile := &Profile{ID: "warp-1", Name: "p1", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true}
	if err := manager.store.AddProfile(profile); err != nil {
		t.Fatal(err)
	}
	addQualityStandbys(t, manager, "provider-standby")
	oldResolver := authProxyResolver
	authProxyResolver = func() map[string]string { return map[string]string{"idx-1": profile.ProxyURL} }
	defer func() { authProxyResolver = oldResolver }()

	q := manager.store.Quality()
	q.ConsecutiveDegraded = 1
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	// 非 xAI/grok 请求：即使 TPS 超高也不参与统计。
	for _, provider := range []string{"openai", "claude", "gemini", ""} {
		if err := manager.HandleUsage(map[string]any{"Provider": provider, "AuthID": "idx-1", "Latency": 1e9, "TTFT": 1e8, "Detail": map[string]any{"OutputTokens": float64(9000)}}); err != nil {
			t.Fatal(err)
		}
		if got := manager.store.Profile(profile.ID); got.Degraded {
			t.Fatalf("provider %q must not degrade the egress: %+v", provider, got)
		}
	}
	// xai 请求触发。
	if err := manager.HandleUsage(map[string]any{"Provider": "xai", "AuthID": "idx-1", "Latency": 1e9, "TTFT": 1e8, "Detail": map[string]any{"OutputTokens": float64(9000)}}); err != nil {
		t.Fatal(err)
	}
	if got := manager.store.Profile(profile.ID); !got.Degraded {
		t.Fatalf("xai request should degrade, got %+v", got)
	}
}

func TestQualityDisabledSkipsObservation(t *testing.T) {
	manager := newTestManager(t)
	profile := &Profile{ID: "warp-1", Name: "p1", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true}
	if err := manager.store.AddProfile(profile); err != nil {
		t.Fatal(err)
	}
	oldResolver := authProxyResolver
	authProxyResolver = func() map[string]string { return map[string]string{"idx-1": profile.ProxyURL} }
	defer func() { authProxyResolver = oldResolver }()

	q := manager.store.Quality()
	q.Enabled = false
	q.ConsecutiveDegraded = 1
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	if err := manager.HandleUsage(map[string]any{"Provider": "xai", "AuthID": "idx-1", "Latency": 1e9, "TTFT": 1e8, "Detail": map[string]any{"OutputTokens": float64(9000)}}); err != nil {
		t.Fatal(err)
	}
	if got := manager.store.Profile(profile.ID); got.Degraded {
		t.Fatalf("disabled quality must not mark profiles: %+v", got)
	}
}

func TestStreamChunkTrackingSettlesOnDone(t *testing.T) {
	manager := newTestManager(t)
	profile := &Profile{ID: "warp-1", Name: "p1", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true}
	if err := manager.store.AddProfile(profile); err != nil {
		t.Fatal(err)
	}
	addQualityStandbys(t, manager, "stream-standby")
	if err := manager.store.SetGlobalProfile("warp-1"); err != nil {
		t.Fatal(err)
	}
	q := manager.store.Quality()
	q.ConsecutiveDegraded = 1
	q.MinOutputTokens = 32
	q.SoftTPS = 10
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	// 请求开始（OpenAI 格式 grok 模型）。
	if err := manager.HandleRequestBefore(map[string]any{"RequestID": "r1", "Model": "grok-4.5", "Stream": true}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // 满足 min_generation_ms 生成窗口
	// 非 xAI 模型不统计。
	if err := manager.HandleRequestBefore(map[string]any{"RequestID": "r9", "Model": "gpt-5", "Stream": true}); err != nil {
		t.Fatal(err)
	}
	if manager.streamTracks["r9"] != nil {
		t.Fatal("non-xai model must not be tracked")
	}
	// header init + 内容 chunk（模拟 200+ 字符输出，TPS 必然超阈值）。
	_ = manager.HandleStreamChunk(map[string]any{"RequestID": "r1", "ChunkIndex": float64(-1)})
	content := ""
	for i := 0; i < 50; i++ {
		content += "abcdefghij"
	}
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": content}}}})
	chunk := "data: " + string(body)
	if err := manager.HandleStreamChunk(map[string]any{"RequestID": "r1", "ChunkIndex": float64(0), "Body": chunk}); err != nil {
		t.Fatal(err)
	}
	if got := manager.store.Profile(profile.ID); got.Degraded {
		t.Fatalf("must not degrade before stream ends: %+v", got)
	}
	// [DONE] 结算。
	if err := manager.HandleStreamChunk(map[string]any{"RequestID": "r1", "ChunkIndex": float64(1), "Body": "data: [DONE]"}); err != nil {
		t.Fatal(err)
	}
	got := manager.store.Profile(profile.ID)
	if !got.Degraded {
		t.Fatalf("expected degraded after stream settlement, got %+v", got)
	}
	if manager.streamTracks["r1"] != nil {
		t.Fatal("track must be cleaned after settlement")
	}
}

func TestStreamAndUsageCallbacksCountOneQualityObservationPerRequest(t *testing.T) {
	manager := newTestManager(t)
	profile := &Profile{ID: "warp-1", Name: "p1", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true}
	if err := manager.store.AddProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err := manager.store.SetGlobalProfile(profile.ID); err != nil {
		t.Fatal(err)
	}
	q := manager.store.Quality()
	q.SoftTPS = 10
	q.HardTPS = 1_000_000
	q.ConsecutiveDegraded = 99
	q.MinGenerationMs = 1
	q.MinOutputTokens = 1
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}

	if err := manager.HandleRequestBefore(map[string]any{"RequestID": "same-request", "Model": "grok-4.6", "Stream": true}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	body, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"delta":         map[string]any{"content": "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"},
			"finish_reason": "stop",
		}},
	})
	if err := manager.HandleStreamChunk(map[string]any{"RequestID": "same-request", "ChunkIndex": float64(0), "Body": body}); err != nil {
		t.Fatal(err)
	}
	if got := manager.store.Profile(profile.ID).QualityStrikes; got != 1 {
		t.Fatalf("stream callback should create one strike, got %d", got)
	}

	if err := manager.HandleUsage(map[string]any{
		"RequestID": "same-request", "Provider": "xai", "AuthID": "idx-1",
		"Latency": float64(10 * time.Millisecond), "TTFT": float64(time.Millisecond),
		"Detail": map[string]any{"OutputTokens": float64(100)},
	}); err != nil {
		t.Fatal(err)
	}
	if got := manager.store.Profile(profile.ID).QualityStrikes; got != 1 {
		t.Fatalf("stream and usage callbacks must not double count one request, got %d strikes", got)
	}
}

func TestLaterStreamReasoningCorrectsEarlierIncompleteUsageObservation(t *testing.T) {
	manager := newTestManager(t)
	profile := &Profile{ID: "warp-1", Name: "p1", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true}
	if err := manager.store.AddProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err := manager.store.SetGlobalProfile(profile.ID); err != nil {
		t.Fatal(err)
	}
	q := manager.store.Quality()
	q.ThinkingGuard = true
	q.ThinkingCrossVerify = false
	q.ConsecutiveMissingThinking = 2
	q.MinGenerationMs = 1
	q.MinOutputTokens = 1
	q.SoftTPS = 1_000_000_000
	q.HardTPS = 1_000_000_000
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}

	const requestID = "usage-before-final-stream"
	if err := manager.HandleRequestBefore(map[string]any{"RequestID": requestID, "Model": "grok-4.6", "Stream": true}); err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]any{"content": "enough visible output to form a quality observation"}}},
	})
	if err := manager.HandleStreamChunk(map[string]any{"RequestID": requestID, "ChunkIndex": float64(0), "Body": content}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)

	// CPA 的 usage 回调可能只保留总输出 token，没有下游结束帧中的
	// completion_tokens_details；这条较早但不完整的证据不能永久覆盖后续证据。
	if err := manager.HandleUsage(map[string]any{
		"RequestID": requestID, "Provider": "xai", "AuthID": "idx-1",
		"Latency": float64(10 * time.Second), "TTFT": float64(time.Second),
		"Detail": map[string]any{"OutputTokens": float64(100)},
	}); err != nil {
		t.Fatal(err)
	}
	if got := manager.store.Profile(profile.ID); got.QualityThinkingStrikes != 0 || got.QualityClassification == "degraded" || got.QualityClassification == "verifying" {
		t.Fatalf("usage without an explicit thinking field is unknown, not missing-thinking evidence: %+v", got)
	}

	finalChunk, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": "stop"}},
		"usage": map[string]any{
			"completion_tokens":         float64(100),
			"completion_tokens_details": map[string]any{"reasoning_tokens": float64(70)},
		},
	})
	if err := manager.HandleStreamChunk(map[string]any{"RequestID": requestID, "ChunkIndex": float64(1), "Body": finalChunk}); err != nil {
		t.Fatal(err)
	}
	got := manager.store.Profile(profile.ID)
	if got.QualityClassification != "healthy" || got.QualityThinkingStrikes != 0 || got.QualitySource != "stream" {
		t.Fatalf("final stream reasoning must correct the provisional missing-thinking verdict: %+v", got)
	}
}

func TestCrossVerificationStopsWaitingAndClearsVerifyingAtConfiguredTimeout(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"healthy probe output\"},\"finish_reason\":\"stop\"}],\"usage\":{\"completion_tokens\":64,\"completion_tokens_details\":{\"reasoning_tokens\":12}}}\n\n"))
		case <-r.Context().Done():
		}
	}))
	defer upstream.Close()

	oldList, oldGet := authListForProbe, authGetForProbe
	defer func() { authListForProbe, authGetForProbe = oldList, oldGet }()
	authListForProbe = func() (hostAuthListResponse, error) {
		return hostAuthListResponse{Files: []hostAuthFileEntry{{AuthIndex: "probe", Provider: "xai", Status: "active"}}}, nil
	}
	authGetForProbe = func(string) (hostAuthGetResponse, error) {
		return hostAuthGetResponse{JSON: json.RawMessage(`{"access_token":"test-token","base_url":"` + upstream.URL + `/v1"}`)}, nil
	}

	manager := newTestManager(t)
	profile := &Profile{ID: "probe-profile", Mode: ProfileModeExternal, ProxyURL: "socks5://" + startTestSOCKSForwarder(t), Running: true, Healthy: true}
	if err := manager.store.AddProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err := manager.store.SetGlobalProfile(profile.ID); err != nil {
		t.Fatal(err)
	}
	q := manager.store.Quality()
	q.Probe.Enabled = true
	q.Probe.Model = "grok-4.6"
	q.Probe.TimeoutSeconds = 5
	q.ThinkingGuard = true
	q.ThinkingCrossVerify = true
	q.ConsecutiveMissingThinking = 1
	q.MinGenerationMs = 1
	q.MinOutputTokens = 1
	q.SoftTPS = 1_000_000
	q.HardTPS = 1_000_000
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}

	manualDone := make(chan struct{})
	go func() {
		defer close(manualDone)
		_, _ = manager.ProbeProfile(profile.ID)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("manual probe did not reach the upstream boundary")
	}
	defer func() {
		close(release)
		select {
		case <-manualDone:
		case <-time.After(2 * time.Second):
		}
	}()

	// 手工探测已经占用串行槽位；交叉验证的总等待预算改为 1 秒。
	q = manager.store.Quality()
	q.Probe.TimeoutSeconds = 1
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	if err := manager.HandleRequestBefore(map[string]any{"RequestID": "cross-verify-timeout", "Model": "grok-4.6", "Stream": true}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	missingThinking, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"delta":         map[string]any{"content": "visible output without a reasoning field"},
			"finish_reason": "stop",
		}},
	})
	if err := manager.HandleStreamChunk(map[string]any{"RequestID": "cross-verify-timeout", "ChunkIndex": float64(0), "Body": missingThinking}); err != nil {
		t.Fatal(err)
	}
	if got := manager.store.Profile(profile.ID); got.QualityClassification != "verifying" {
		t.Fatalf("missing thinking should enter verifying before the bounded probe: %+v", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got := manager.store.Profile(profile.ID)
		if got.QualityClassification != "verifying" {
			if got.Degraded || got.QualityClassification != "error" || !strings.Contains(got.QualityError, "超时") {
				t.Fatalf("an inconclusive cross verification must keep the egress and expose a timeout: %+v", got)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("cross verification remained stuck after its configured timeout: %+v", manager.store.Profile(profile.ID))
}

func TestConcurrentQualityPathNoRace(t *testing.T) {
	manager := newTestManager(t)
	profiles := []*Profile{
		{ID: "a", Name: "a", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true},
		{ID: "b", Name: "b", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41002", Healthy: true},
		{ID: "c", Name: "c", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41003", Healthy: true},
	}
	for _, p := range profiles {
		if err := manager.store.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.store.SetGlobalProfile("a"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveAutoSwitch(AutoSwitchConfig{Enabled: true, FailoverEnabled: true}); err != nil {
		t.Fatal(err)
	}
	q := manager.store.Quality()
	q.ConsecutiveDegraded = 1
	q.SoftTPS = 1
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	oldResolver := authProxyResolver
	authProxyResolver = func() map[string]string {
		return map[string]string{"idx-1": "socks5://127.0.0.1:41001", "idx-2": "socks5://127.0.0.1:41002", "idx-3": "socks5://127.0.0.1:41003"}
	}
	defer func() { authProxyResolver = oldResolver }()

	// 并发：usage 观测 + 自动切换评估 + 状态读取 + 流式 chunk + 自动清理
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = manager.HandleUsage(map[string]any{
				"Provider": "xai", "AuthID": "idx-1",
				"Latency": float64(2 * 1e9), "TTFT": float64(1 * 1e8),
				"Detail": map[string]any{"OutputTokens": float64(9000)},
			})
		}(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = manager.EvaluateAutoSwitch(false)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = manager.Status()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = manager.HandleRequestBefore(map[string]any{"RequestID": "r", "Model": "grok-4.5", "Stream": true})
			_ = manager.HandleStreamChunk(map[string]any{"RequestID": "r", "ChunkIndex": float64(0), "Body": "data: {\"choices\":[{\"delta\":{\"content\":\"hello world\"}}]}"})
			_ = manager.HandleStreamChunk(map[string]any{"RequestID": "r", "ChunkIndex": float64(1), "Body": "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}"})
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			manager.autoPrune(manager.store.Quality())
		}()
	}
	wg.Wait()
}

func TestAutoProvisionCooldownOnFailure(t *testing.T) {
	manager := newTestManager(t)
	profiles := []*Profile{
		{ID: "a", Name: "a", Mode: ProfileModeManaged, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true, CreatedAt: time.Now()},
	}
	for _, p := range profiles {
		if err := manager.store.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	q := manager.store.Quality()
	q.MinHealthy = 2
	q.ProvisionCooldownMin = 1
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	// 第一次尝试失败（记录时间），冷却内不重复尝试。
	manager.lastProvisionAt = time.Now()
	manager.provisionError = "WARP 注册被 Cloudflare 限流（429）"
	if err := manager.autoProvision(q); err != nil {
		t.Fatalf("cooldown must suppress attempt: %v", err)
	}
	if len(manager.store.Profiles()) != 1 {
		t.Fatalf("cooldown must prevent new profile creation, got %d", len(manager.store.Profiles()))
	}
	// 冷却过期后允许尝试（这里会真实调 wgcf，直接验证 lastProvisionAt 更新逻辑）。
	manager.lastProvisionAt = time.Time{}
	_ = manager.autoProvision(q)
	manager.mu.Lock()
	attempted := !manager.lastProvisionAt.IsZero()
	manager.mu.Unlock()
	if !attempted {
		t.Fatal("after cooldown expiry a provision attempt must be recorded")
	}
}

func TestHealthyXAIProbeEntry(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name  string
		entry hostAuthFileEntry
		want  bool
	}{
		{name: "active", entry: hostAuthFileEntry{AuthIndex: "a", Provider: "xai", Status: "active"}, want: true},
		{name: "legacy status", entry: hostAuthFileEntry{AuthIndex: "a", Type: "grok"}, want: true},
		{name: "disabled", entry: hostAuthFileEntry{AuthIndex: "a", Provider: "xai", Disabled: true}, want: false},
		{name: "unavailable", entry: hostAuthFileEntry{AuthIndex: "a", Provider: "xai", Unavailable: true}, want: false},
		{name: "cooldown status", entry: hostAuthFileEntry{AuthIndex: "a", Provider: "xai", Status: "cooldown"}, want: false},
		{name: "retry later", entry: hostAuthFileEntry{AuthIndex: "a", Provider: "xai", NextRetryAfter: now.Add(time.Minute)}, want: false},
		{name: "runtime only", entry: hostAuthFileEntry{AuthIndex: "a", Provider: "xai", RuntimeOnly: true}, want: false},
		{name: "other provider", entry: hostAuthFileEntry{AuthIndex: "a", Provider: "codex", Status: "active"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isHealthyXAIProbeEntry(tc.entry, now); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestListXAIAccountsReadsOnlyHealthyCandidates(t *testing.T) {
	oldList, oldGet := authListForProbe, authGetForProbe
	defer func() { authListForProbe, authGetForProbe = oldList, oldGet }()

	files := make([]hostAuthFileEntry, 0, 1004)
	for i := 0; i < 1000; i++ {
		files = append(files, hostAuthFileEntry{AuthIndex: "disabled", Provider: "xai", Disabled: true})
	}
	files = append(files,
		hostAuthFileEntry{AuthIndex: "codex", Provider: "codex", Status: "active"},
		hostAuthFileEntry{AuthIndex: "expired", Provider: "xai", Status: "active"},
		hostAuthFileEntry{AuthIndex: "healthy", Provider: "xai", Status: "active"},
		hostAuthFileEntry{AuthIndex: "cooldown", Provider: "xai", NextRetryAfter: time.Now().Add(time.Hour)},
	)
	authListForProbe = func() (hostAuthListResponse, error) {
		return hostAuthListResponse{Files: files}, nil
	}
	gets := []string{}
	authGetForProbe = func(authIndex string) (hostAuthGetResponse, error) {
		gets = append(gets, authIndex)
		if authIndex == "expired" {
			return hostAuthGetResponse{JSON: json.RawMessage(`{"access_token":"old","expired":"2000-01-01T00:00:00Z"}`)}, nil
		}
		return hostAuthGetResponse{JSON: json.RawMessage(`{"access_token":"ok"}`)}, nil
	}

	accounts := listXAIAccounts(8)
	if len(accounts) != 1 || accounts[0].AccessToken != "ok" {
		t.Fatalf("expected one healthy account, got %+v", accounts)
	}
	if len(gets) != 2 || gets[0] != "expired" || gets[1] != "healthy" {
		t.Fatalf("must not read disabled/cooling/non-xAI account files, got %v", gets)
	}
}

func TestXAIProbeAccountsCachedAcrossProfiles(t *testing.T) {
	manager := newTestManager(t)
	oldList, oldGet := authListForProbe, authGetForProbe
	defer func() { authListForProbe, authGetForProbe = oldList, oldGet }()
	lists, gets := 0, 0
	authListForProbe = func() (hostAuthListResponse, error) {
		lists++
		return hostAuthListResponse{Files: []hostAuthFileEntry{{AuthIndex: "healthy", Provider: "xai", Status: "active"}}}, nil
	}
	authGetForProbe = func(string) (hostAuthGetResponse, error) {
		gets++
		return hostAuthGetResponse{JSON: json.RawMessage(`{"access_token":"ok"}`)}, nil
	}
	first := manager.xaiAccountsForProbe(8)
	second := manager.xaiAccountsForProbe(8)
	if len(first) != 1 || len(second) != 1 || lists != 1 || gets != 1 {
		t.Fatalf("healthy probe accounts must be reused from memory, lists=%d gets=%d", lists, gets)
	}
}

func TestProbeProfileRetriesNoOutputAccountAndStopsAtFinishReason(t *testing.T) {
	type observedProbeRequest struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	observedRequest := make(chan observedProbeRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request observedProbeRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode probe request: %v", err)
		}
		if r.Header.Get("Authorization") == "Bearer timeout-token" {
			// 单个账号可能被上游调度到无响应实例；出口结论必须由另一个
			// 健康账号交叉验证，不能把首次无首帧超时直接归因给 WARP。
			<-r.Context().Done()
			return
		}
		observedRequest <- request
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"probe output with enough text for a quality observation\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"completion_tokens\":64,\"completion_tokens_details\":{\"reasoning_tokens\":12}}}\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		// xAI may send the requested usage frame after finish_reason, then keep the SSE
		// connection alive and omit [DONE]. The probe must consume usage and stop.
		<-r.Context().Done()
	}))
	defer upstream.Close()

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxyListener.Close()
	go func() {
		for {
			client, acceptErr := proxyListener.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer client.Close()
				destination, socksErr := acceptSOCKS5(client)
				if socksErr != nil {
					return
				}
				remote, dialErr := net.DialTimeout("tcp", destination, time.Second)
				if dialErr != nil {
					_ = writeSOCKSReply(client, 0x05, nil)
					return
				}
				defer remote.Close()
				_ = writeSOCKSReply(client, 0, remote.LocalAddr())
				copyStream(client, remote)
			}()
		}
	}()

	oldList, oldGet := authListForProbe, authGetForProbe
	defer func() { authListForProbe, authGetForProbe = oldList, oldGet }()
	authListForProbe = func() (hostAuthListResponse, error) {
		return hostAuthListResponse{Files: []hostAuthFileEntry{
			{AuthIndex: "timeout", Provider: "xai", Status: "active"},
			{AuthIndex: "probe", Provider: "xai", Status: "active"},
		}}, nil
	}
	authGetForProbe = func(authIndex string) (hostAuthGetResponse, error) {
		token := "test-token"
		if authIndex == "timeout" {
			token = "timeout-token"
		}
		return hostAuthGetResponse{JSON: json.RawMessage(`{"access_token":"` + token + `","base_url":"` + upstream.URL + `/v1"}`)}, nil
	}

	manager := newTestManager(t)
	profile := &Profile{ID: "probe-profile", Mode: ProfileModeExternal, ProxyURL: "socks5://" + proxyListener.Addr().String(), Running: true, Healthy: true}
	if err := manager.store.AddProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err := manager.store.SetGlobalProfile(profile.ID); err != nil {
		t.Fatal(err)
	}
	q := manager.store.Quality()
	q.Probe.Enabled = true
	q.Probe.Model = "grok-4.6"
	q.Probe.TimeoutSeconds = 1
	q.MinGenerationMs = 1
	q.MinOutputTokens = 1
	q.SoftTPS = 1_000_000
	q.HardTPS = 1_000_000
	q.ThinkingGuard = true
	q.ThinkingCrossVerify = false
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	result, err := manager.ProbeProfile(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 750*time.Millisecond || elapsed >= 1750*time.Millisecond {
		t.Fatalf("probe must retry one no-output account, then stop at finish_reason: %v (%+v)", elapsed, result)
	}
	if result.Classification != "healthy" || !result.HasThinking {
		t.Fatalf("semantic finish frame should complete a healthy probe: %+v", result)
	}
	request := <-observedRequest
	if len(request.Messages) != 1 || request.Messages[0].Content != "Explain TCP slow start in exactly four short sentences, plain text only." {
		t.Fatalf("quality probe must use a bounded prompt that starts promptly, got %+v", request.Messages)
	}
	if !request.StreamOptions.IncludeUsage {
		t.Fatal("quality probe must request the final usage frame so hidden reasoning tokens remain observable")
	}
}

func TestSelectQualityProbeCandidateStaggersProfiles(t *testing.T) {
	now := time.Now()
	profiles := []*Profile{
		{ID: "recent", Running: true, Healthy: true, ProxyURL: "socks5://127.0.0.1:1", QualityCheckedAt: now.Add(-time.Minute)},
		{ID: "old", Running: true, Healthy: true, ProxyURL: "socks5://127.0.0.1:2", QualityCheckedAt: now.Add(-time.Hour)},
		{ID: "untested", Running: true, Healthy: true, ProxyURL: "socks5://127.0.0.1:3"},
		{ID: "stopped", Mode: ProfileModeManaged, Running: false, Healthy: true, ProxyURL: "socks5://127.0.0.1:4"},
		{ID: "unhealthy", Running: true, Healthy: false, ProxyURL: "socks5://127.0.0.1:5"},
	}
	if got := selectQualityProbeCandidate(profiles, 15*time.Minute, now); got == nil || got.ID != "untested" {
		t.Fatalf("untested profile must be probed first, got %+v", got)
	}
	profiles[2].QualityCheckedAt = now
	if got := selectQualityProbeCandidate(profiles, 15*time.Minute, now); got == nil || got.ID != "old" {
		t.Fatalf("oldest due profile must be selected, got %+v", got)
	}
}

func TestEvaluateDegradedSwitchUsesFreshVerifiedStandby(t *testing.T) {
	manager := newTestManager(t)
	now := time.Now()
	current := &Profile{ID: "current", Name: "a-current", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Running: true, Healthy: true, Degraded: true, ExitIP: "1.1.1.1"}
	standby := &Profile{ID: "standby", Name: "b-standby", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41002", Running: true, Healthy: true, ExitIP: "2.2.2.2", QualityClassification: "healthy", QualityCheckedAt: now}
	for _, profile := range []*Profile{current, standby} {
		if err := manager.store.AddProfile(profile); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.store.SetRules(Rules{GlobalProfileID: current.ID, ExactRules: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	q := manager.store.Quality()
	q.Probe.Enabled = true
	q.Probe.Model = "grok-4"
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	got, err := manager.EvaluateAutoSwitch(false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != standby.ID || manager.store.Rules().GlobalProfileID != standby.ID {
		t.Fatalf("expected verified standby switch, got %+v", got)
	}
}

func TestDegradedSwitchDoesNotReprobeFreshFailure(t *testing.T) {
	manager := newTestManager(t)
	now := time.Now()
	current := &Profile{ID: "current", Name: "a-current", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Running: true, Healthy: true, Degraded: true}
	failed := &Profile{ID: "failed", Name: "b-failed", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41002", Running: true, Healthy: true, QualityClassification: "error", QualityCheckedAt: now}
	for _, profile := range []*Profile{current, failed} {
		if err := manager.store.AddProfile(profile); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.store.SetRules(Rules{GlobalProfileID: current.ID, ExactRules: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	oldList := authListForProbe
	defer func() { authListForProbe = oldList }()
	listCalls := 0
	authListForProbe = func() (hostAuthListResponse, error) {
		listCalls++
		return hostAuthListResponse{}, nil
	}
	if selected, err := manager.EvaluateAutoSwitch(false); err == nil || selected != nil {
		t.Fatalf("fresh failed candidate must not be selected, selected=%+v err=%v", selected, err)
	}
	if listCalls != 0 {
		t.Fatalf("fresh failed candidate must wait for interval before reprobe, list calls=%d", listCalls)
	}
}

func TestMinHealthyClampedToMaxProfiles(t *testing.T) {
	manager := newTestManager(t)
	q := manager.store.Quality()
	q.MinHealthy = 10
	q.MaxProfiles = 4
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	got := manager.store.Quality()
	if got.MinHealthy != 4 {
		t.Fatalf("min_healthy must be clamped to max_profiles, got %d", got.MinHealthy)
	}
}

func TestQualityThresholdsKeepHardAtOrAboveSoft(t *testing.T) {
	manager := newTestManager(t)
	q := manager.store.Quality()
	q.SoftTPS = 800
	q.HardTPS = 700
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	got := manager.store.Quality()
	if got.HardTPS != got.SoftTPS {
		t.Fatalf("hard_tps must be clamped to soft_tps when configured lower: %+v", got)
	}
}

func TestStateJSONAuthoritativeOverride(t *testing.T) {
	manager := newTestManager(t)
	if err := manager.store.AddProfile(&Profile{ID: "local", Name: "local", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001"}); err != nil {
		t.Fatal(err)
	}
	// 配置文件权威：state-json 提供完整状态并覆盖本地 state.json。
	raw := `{"version":1,"profiles":[{"id":"cfg","name":"from-config","mode":"external","proxy_url":"socks5://127.0.0.1:41002","healthy":true}],"rules":{"global_profile_id":"cfg","type_rules":[],"regex_rules":[],"exact_rules":{"a":"cfg"}},"quality":{"enabled":true,"soft_tps":123},"settings":{"cleanup_unhealthy_enabled":true}}`
	var authoritative PersistedState
	if err := json.Unmarshal([]byte(raw), &authoritative); err != nil {
		t.Fatal(err)
	}
	if err := manager.store.ReplaceState(authoritative); err != nil {
		t.Fatal(err)
	}
	profiles := manager.store.Profiles()
	if len(profiles) != 1 || profiles[0].ID != "cfg" {
		t.Fatalf("state-json must replace local state, got %d profiles", len(profiles))
	}
	if profiles[0].Running || profiles[0].PID != 0 {
		t.Fatal("replaced profiles must be reset to stopped")
	}
	if got := manager.store.Quality(); got.SoftTPS != 123 || !got.Enabled {
		t.Fatalf("quality not applied from state-json: %+v", got)
	}
	if got := manager.store.Settings(); !got.CleanupUnhealthy {
		t.Fatal("settings not applied from state-json")
	}
	if got := manager.store.Rules(); got.GlobalProfileID != "cfg" || got.ExactRules["a"] != "cfg" {
		t.Fatalf("rules not applied from state-json: %+v", got)
	}
	// 落盘后可重新加载。
	if err := manager.store.Load(); err != nil {
		t.Fatal(err)
	}
	if got := manager.store.Profile("cfg"); got == nil {
		t.Fatal("state-json state must survive reload")
	}
}

func TestQualityStateDefaultsAndRoundTrip(t *testing.T) {
	manager := newTestManager(t)
	q := manager.store.Quality()
	if !q.Enabled || q.SoftTPS <= 0 || q.HardTPS < q.SoftTPS || q.ConsecutiveErrors < 1 || q.MinHealthy < 1 {
		t.Fatalf("unexpected defaults: %+v", q)
	}
	q.Enabled = false
	q.SoftTPS = 300
	q.HardTPS = 900
	q.ConsecutiveErrors = 5
	q.Probe.Enabled = true
	q.Probe.Model = "grok-4"
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	got := manager.store.Quality()
	if got.Enabled || got.SoftTPS != 300 || got.HardTPS != 900 || got.ConsecutiveErrors != 5 || !got.Probe.Enabled || got.Probe.Model != "grok-4" {
		t.Fatalf("quality did not round trip: %+v", got)
	}
}

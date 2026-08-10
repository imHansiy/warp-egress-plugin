package main

import (
	"encoding/json"
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

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	manager := NewManager()
	manager.cfg = defaultConfig()
	manager.store = NewStateStore(t.TempDir())
	return manager
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
	oldResolver := authProxyResolver
	authProxyResolver = func() map[string]string { return map[string]string{"idx-1": profile.ProxyURL} }
	defer func() { authProxyResolver = oldResolver }()

	q := manager.store.Quality()
	q.ConsecutiveDegraded = 2
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
	if !q.Enabled || q.SoftTPS <= 0 || q.MinHealthy < 1 {
		t.Fatalf("unexpected defaults: %+v", q)
	}
	q.Enabled = false
	q.SoftTPS = 300
	q.Probe.Enabled = true
	q.Probe.Model = "grok-4"
	if err := manager.store.SetQuality(q); err != nil {
		t.Fatal(err)
	}
	got := manager.store.Quality()
	if got.Enabled || got.SoftTPS != 300 || !got.Probe.Enabled || got.Probe.Model != "grok-4" {
		t.Fatalf("quality did not round trip: %+v", got)
	}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	mu           sync.RWMutex
	cfg          Config
	store        *StateStore
	relay        *SOCKSRelay
	processes    map[string]*managedProcess
	cancelHealth context.CancelFunc
	lastError    string
	configured   bool

	qualitySavePending   bool
	qualityTaskRunning   bool
	qualityProbeSlot     chan struct{}
	qualityProbeAuths    []xaiAccount
	qualityProbeAuthAt   time.Time
	qualityCrossVerifyMu sync.Mutex
	qualityCrossVerify   map[string]*qualityCrossVerifyTask
	qualityObservationMu sync.Mutex
	qualityObserved      map[string]qualityObservationClaim
	lastProvisionAt      time.Time
	provisionError       string
	// 被动 usage 诊断（供排查降智检测链路）
	usageEvents       int
	lastUsageProvider string
	lastUsageModel    string
	lastUsageAuth     string
	lastUsageTokens   int64
	lastUsageLatency  string
	// 系统代理
	systemProxyInstance *systemProxy
	// 流式补偿轨道
	streamMu           sync.Mutex
	streamTracks       map[string]*streamTrack
	cancelStream       context.CancelFunc
	streamBeforeEvents int
	streamChunkEvents  int
	streamTrackChars   int
	streamTrackDone    bool
	streamChunkIndexes []int
	streamSample       string
}

func NewManager() *Manager {
	return &Manager{
		processes:          map[string]*managedProcess{},
		qualityProbeSlot:   make(chan struct{}, 1),
		qualityCrossVerify: map[string]*qualityCrossVerifyTask{},
		qualityObserved:    map[string]qualityObservationClaim{},
	}
}

func (m *Manager) Configure(raw []byte) error {
	var req lifecycleRequest
	if len(raw) > 0 {
		if err := decodeJSON(raw, &req); err != nil {
			return err
		}
	}
	cfg, err := parseConfig(decodeConfigYAML(req.ConfigYAML))
	if err != nil {
		return err
	}
	if err := ensureDataDir(cfg); err != nil {
		return err
	}
	// 未显式配置 wgcf-path/wireproxy-path 且 PATH 中找不到时，
	// 自动解压插件内置二进制到 data-dir/bin，实现服务器零安装。
	cfg, err = ensureBundledTools(cfg)
	if err != nil {
		return err
	}

	m.stopOwnedProcesses()
	m.mu.Lock()
	oldRelay := m.relay
	oldCancel := m.cancelHealth
	m.cfg = cfg
	m.store = NewStateStore(cfg.DataDir)
	router := NewEgressRouter(m.stateStore)
	m.relay = NewSOCKSRelay(net.JoinHostPort(cfg.ListenHost, fmt.Sprintf("%d", cfg.GlobalPort)), router.Decide)
	m.cancelHealth = nil
	m.configured = true
	m.lastError = ""
	m.mu.Unlock()

	if oldCancel != nil {
		oldCancel()
	}
	if oldRelay != nil {
		oldRelay.Close()
	}
	if err := m.store.Load(); err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	// 配置文件权威：config.yaml 插件段的 state-json（CPA 配置体系/数据库
	// 管理的插件配置）优先于本地 state.json，重配/重启后恢复文件值。
	if cfg.StateJSON != "" {
		var authoritative PersistedState
		if err := json.Unmarshal([]byte(cfg.StateJSON), &authoritative); err != nil {
			return fmt.Errorf("state-json: %w", err)
		}
		if err := m.store.ReplaceState(authoritative); err != nil {
			return fmt.Errorf("apply state-json: %w", err)
		}
	}
	if err := m.relay.Start(); err != nil {
		m.setLastError(err.Error())
		return fmt.Errorf("start global relay: %w", err)
	}
	if cfg.AutoStart {
		for _, p := range m.store.Profiles() {
			if p.Mode == ProfileModeManaged {
				_ = m.StartProfile(p.ID)
			}
		}
	}
	// 加载/启动出口后为独立 xAI 路由选定一个可用出口。该步骤只遍历
	// 少量 profile，不读取认证目录；没有候选时中继保持 fail-closed。
	_, _ = m.ensureXAIActiveProfile(false)
	m.startHealthLoop()
	m.mu.Lock()
	if m.cancelStream != nil {
		m.cancelStream()
	}
	streamCtx, streamCancel := context.WithCancel(context.Background())
	m.cancelStream = streamCancel
	m.streamTracks = map[string]*streamTrack{}
	m.mu.Unlock()
	m.startStreamTrackTTLCleanup(streamCtx)
	// 系统代理：按设置启动（写入系统环境文件）。
	if m.stateStore() != nil && m.stateStore().Settings().SystemProxy.Enabled {
		if err := m.ApplySystemProxy(m.stateStore().Settings().SystemProxy, ""); err != nil {
			m.setLastError("系统代理启动失败: " + err.Error())
		}
	}
	return nil
}

func (m *Manager) stopOwnedProcesses() {
	m.mu.Lock()
	processes := make([]*managedProcess, 0, len(m.processes))
	for _, process := range m.processes {
		if process != nil {
			processes = append(processes, process)
		}
	}
	m.processes = map[string]*managedProcess{}
	m.mu.Unlock()
	for _, process := range processes {
		if process.cmd != nil && process.cmd.Process != nil {
			_ = process.cmd.Process.Kill()
		}
	}
}

func (m *Manager) Shutdown() {
	m.mu.Lock()
	if m.cancelHealth != nil {
		m.cancelHealth()
		m.cancelHealth = nil
	}
	if m.cancelStream != nil {
		m.cancelStream()
		m.cancelStream = nil
	}
	sysProxy := m.systemProxyInstance
	relay := m.relay
	ids := make([]string, 0, len(m.processes))
	for id := range m.processes {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	if sysProxy != nil {
		sysProxy.Stop()
	}
	if relay != nil {
		relay.Close()
	}
	for _, id := range ids {
		_ = m.StopProfile(id)
	}
}

func (m *Manager) currentConfig() Config       { m.mu.RLock(); defer m.mu.RUnlock(); return m.cfg }
func (m *Manager) stateStore() *StateStore     { m.mu.RLock(); defer m.mu.RUnlock(); return m.store }
func (m *Manager) setLastError(message string) { m.mu.Lock(); m.lastError = message; m.mu.Unlock() }

func (m *Manager) startHealthLoop() {
	cfg := m.currentConfig()
	if cfg.HealthCheckInterval <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.cancelHealth = cancel
	m.mu.Unlock()
	go func() {
		timer := time.NewTicker(cfg.HealthCheckInterval)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				_ = m.CheckAllProfiles()
				_, _ = m.EvaluateAutoSwitch(false)
				_, _ = m.EvaluateXAISwitch(false)
				m.cleanupUnhealthy()
				m.runQualityTasksOnce()
			}
		}
	}()
}

// runQualityTasksOnce 周期触发自动补充/清理；异步执行且防重入，
// 避免 wgcf 注册卡住时下一个 tick 再次触发。
func (m *Manager) runQualityTasksOnce() {
	m.mu.Lock()
	if m.qualityTaskRunning {
		m.mu.Unlock()
		return
	}
	m.qualityTaskRunning = true
	m.mu.Unlock()
	go func() {
		defer func() {
			m.mu.Lock()
			m.qualityTaskRunning = false
			m.mu.Unlock()
		}()
		m.evaluateQualityTasks()
	}()
}

func (m *Manager) selectedProfileProxy() (string, error) {
	store := m.stateStore()
	if store == nil {
		return "", errors.New("plugin is not configured")
	}
	rules := store.Rules()
	if rules.GlobalProfileID == "" {
		return "", errors.New("no global WARP profile selected")
	}
	profile := store.Profile(rules.GlobalProfileID)
	if profile == nil {
		return "", errors.New("selected global profile does not exist")
	}
	if strings.TrimSpace(profile.ProxyURL) == "" {
		return "", errors.New("selected profile has no proxy URL")
	}
	return profile.ProxyURL, nil
}

func (m *Manager) Status() statusResponse {
	cfg := m.currentConfig()
	store := m.stateStore()
	response := statusResponse{PluginID: pluginID, Version: pluginVersion, GlobalRelayURL: "socks5://" + net.JoinHostPort(cfg.ListenHost, fmt.Sprintf("%d", cfg.GlobalPort)), RequiredHostProxyURL: "socks5://" + net.JoinHostPort(cfg.ListenHost, fmt.Sprintf("%d", cfg.GlobalPort)), DataDir: cfg.DataDir}
	m.mu.RLock()
	response.LastError = m.lastError
	if m.relay != nil {
		response.GlobalRelayRunning = m.relay.Running()
	}
	m.mu.RUnlock()
	if store == nil {
		return response
	}
	response.Profiles = store.Profiles()
	response.AutoSwitch = store.AutoSwitch()
	rules := store.Rules()
	response.GlobalProfileID = rules.GlobalProfileID
	response.GlobalProfile = store.Profile(rules.GlobalProfileID)
	duplicate := map[string][]string{}
	for _, p := range response.Profiles {
		if p.Healthy && p.ExitIP != "" {
			duplicate[p.ExitIP] = append(duplicate[p.ExitIP], p.ID)
		}
	}
	for ip, ids := range duplicate {
		if len(ids) < 2 {
			delete(duplicate, ip)
		} else {
			sort.Strings(ids)
		}
	}
	response.DuplicateExitIPs = duplicate
	response.AutoProvision = m.buildAutoProvisionStatus(store.Quality())
	response.UsageDiagnostics = m.usageDiagnostics()
	return response
}

// buildAutoProvisionStatus 汇总自动补充出口的状态：健康托管出口数、
// 最近一次注册结果与下次重试时间。
func (m *Manager) buildAutoProvisionStatus(q QualityConfig) *autoProvisionStatus {
	status := &autoProvisionStatus{Enabled: q.Enabled && q.AutoProvision, MinHealthy: q.MinHealthy, MaxProfiles: q.MaxProfiles}
	store := m.stateStore()
	if store == nil {
		return status
	}
	now := time.Now()
	for _, p := range store.Profiles() {
		if p.Mode == ProfileModeManaged && p.Healthy && !p.Degraded &&
			(!q.Probe.Enabled || qualityObservationFresh(p, q, now)) {
			status.HealthyManaged++
		}
	}
	if status.Enabled {
		m.mu.RLock()
		lastAttempt := m.lastProvisionAt
		lastError := m.provisionError
		m.mu.RUnlock()
		status.LastAttemptAt = lastAttempt
		status.LastError = lastError
		cooldown := time.Duration(q.ProvisionCooldownMin) * time.Minute
		if cooldown <= 0 {
			cooldown = 15 * time.Minute
		}
		if !lastAttempt.IsZero() {
			remaining := cooldown - time.Since(lastAttempt)
			if remaining > 0 {
				status.NextAttemptInSeconds = int64(remaining.Seconds())
			}
		}
	}
	return status
}

// UsageDiagnostics 被动 usage 事件诊断信息（排查降智检测链路）。
type UsageDiagnostics struct {
	Events        int    `json:"events"`
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	AuthID        string `json:"auth_id,omitempty"`
	OutputTokens  int64  `json:"output_tokens,omitempty"`
	Latency       string `json:"latency,omitempty"`
	StreamBefore  int    `json:"stream_before,omitempty"`
	StreamChunks  int    `json:"stream_chunks,omitempty"`
	StreamChars   int    `json:"stream_chars,omitempty"`
	StreamDone    bool   `json:"stream_done,omitempty"`
	StreamIndexes []int  `json:"stream_indexes,omitempty"`
	StreamSample  string `json:"stream_sample,omitempty"`
}

func (m *Manager) usageDiagnostics() UsageDiagnostics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return UsageDiagnostics{
		Events:        m.usageEvents,
		Provider:      m.lastUsageProvider,
		Model:         m.lastUsageModel,
		AuthID:        m.lastUsageAuth,
		OutputTokens:  m.lastUsageTokens,
		Latency:       m.lastUsageLatency,
		StreamBefore:  m.streamBeforeEvents,
		StreamChunks:  m.streamChunkEvents,
		StreamChars:   m.streamTrackChars,
		StreamDone:    m.streamTrackDone,
		StreamIndexes: append([]int(nil), m.streamChunkIndexes...),
		StreamSample:  m.streamSample,
	}
}

func (m *Manager) AllocatePort() (int, error) {
	cfg := m.currentConfig()
	used := map[int]bool{cfg.GlobalPort: true}
	store := m.stateStore()
	if store != nil {
		for _, p := range store.Profiles() {
			if p.ListenPort > 0 {
				used[p.ListenPort] = true
			}
		}
	}
	for port := cfg.ProfilePortStart; port <= cfg.ProfilePortEnd; port++ {
		if used[port] {
			continue
		}
		listener, err := net.Listen("tcp", net.JoinHostPort(cfg.ListenHost, fmt.Sprintf("%d", port)))
		if err != nil {
			continue
		}
		listener.Close()
		return port, nil
	}
	return 0, errors.New("no free profile port")
}

func (m *Manager) CreateProfile(req createProfileRequest) (*Profile, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	mode := ProfileMode(strings.ToLower(strings.TrimSpace(req.Mode)))
	if mode != ProfileModeManaged && mode != ProfileModeExternal {
		return nil, errors.New("mode must be managed or external")
	}
	now := time.Now()
	profile := &Profile{ID: newID("warp"), Name: name, Mode: mode, CreatedAt: now, UpdatedAt: now, Origin: req.Origin}
	if mode == ProfileModeExternal {
		proxyURL, err := normalizeSOCKSURL(req.ProxyURL)
		if err != nil {
			return nil, err
		}
		cfg := m.currentConfig()
		globalRelay := "socks5://" + net.JoinHostPort(cfg.ListenHost, fmt.Sprintf("%d", cfg.GlobalPort))
		if proxyURL == globalRelay {
			return nil, errors.New("external profile cannot point to the global relay itself")
		}
		profile.ProxyURL = proxyURL
	} else {
		port, err := m.AllocatePort()
		if err != nil {
			return nil, err
		}
		cfg := m.currentConfig()
		profile.ListenHost = cfg.ListenHost
		profile.ListenPort = port
		profile.ProxyURL = "socks5://" + net.JoinHostPort(cfg.ListenHost, fmt.Sprintf("%d", port))
		profile.Directory = filepath.Join(cfg.DataDir, "profiles", profile.ID)
		if err := os.MkdirAll(profile.Directory, 0o700); err != nil {
			return nil, err
		}
		registerVia, err := m.resolveRegisterProxy(req.RegisterVia)
		if err != nil {
			return nil, err
		}
		if err := m.RegisterManagedProfile(profile, registerVia); err != nil {
			return nil, err
		}
	}
	if err := m.stateStore().AddProfile(profile); err != nil {
		return nil, err
	}
	if req.AutoStart || (mode == ProfileModeManaged && m.currentConfig().AutoStart) {
		if err := m.StartProfile(profile.ID); err != nil {
			profile.LastError = err.Error()
			_ = m.stateStore().UpdateProfile(profile)
		}
	}
	_ = m.CheckProfile(profile.ID)
	return m.stateStore().Profile(profile.ID), nil
}

// resolveRegisterProxy 把创建请求里的 register_via 解析为代理地址：
// 空串表示直连；socks5://、socks5h://、http://、https:// 为自定义代理；
// 否则视为已有托管出口的 ID（解析为其 SOCKS5 地址）。
func (m *Manager) resolveRegisterProxy(via string) (string, error) {
	via = strings.TrimSpace(via)
	if via == "" {
		return "", nil
	}
	lower := strings.ToLower(via)
	switch {
	case strings.HasPrefix(lower, "socks5://"), strings.HasPrefix(lower, "socks5h://"):
		parsed, err := url.Parse(via)
		if err != nil || parsed.Hostname() == "" || parsed.Port() == "" {
			return "", errors.New("invalid register proxy URL: must include host and port")
		}
		return via, nil
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		parsed, err := url.Parse(via)
		if err != nil || parsed.Hostname() == "" || parsed.Port() == "" {
			return "", errors.New("invalid register proxy URL: must include host and port")
		}
		return via, nil
	}
	p := m.stateStore().Profile(via)
	if p == nil {
		return "", fmt.Errorf("register_via profile %q not found", via)
	}
	if p.Mode != ProfileModeManaged {
		return "", errors.New("register_via must be a managed profile")
	}
	if !p.Running {
		return "", errors.New("register_via profile is not running")
	}
	return p.ProxyURL, nil
}

func (m *Manager) ImportProfile(req importProfileRequest) (*Profile, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	content := strings.TrimSpace(req.WGCFProfile)
	if content == "" {
		return nil, errors.New("wgcf_profile is required")
	}
	cfg := m.currentConfig()
	port, err := m.AllocatePort()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	profile := &Profile{
		ID:         newID("warp"),
		Name:       name,
		Mode:       ProfileModeManaged,
		CreatedAt:  now,
		UpdatedAt:  now,
		ListenHost: cfg.ListenHost,
		ListenPort: port,
		ProxyURL:   "socks5://" + net.JoinHostPort(cfg.ListenHost, fmt.Sprintf("%d", port)),
		Directory:  filepath.Join(cfg.DataDir, "profiles", newID("warp")),
	}
	// Use a stable directory derived from the ID assigned above.
	profile.Directory = filepath.Join(cfg.DataDir, "profiles", profile.ID)
	if err := os.MkdirAll(profile.Directory, 0o700); err != nil {
		return nil, err
	}
	wgPath := filepath.Join(profile.Directory, "wgcf-profile.conf")
	if err := os.WriteFile(wgPath, []byte(content), 0o600); err != nil {
		return nil, err
	}
	if err := m.RegisterManagedProfile(profile, ""); err != nil {
		return nil, err
	}
	if err := m.stateStore().AddProfile(profile); err != nil {
		return nil, err
	}
	if err := m.StartProfile(profile.ID); err != nil {
		profile.LastError = err.Error()
		_ = m.stateStore().UpdateProfile(profile)
	}
	_ = m.CheckProfile(profile.ID)
	return m.stateStore().Profile(profile.ID), nil
}

func (m *Manager) SwitchGlobal(profileID string) error {
	if profileID != "" && m.stateStore().Profile(profileID) == nil {
		return errors.New("profile not found")
	}
	if err := m.stateStore().SetGlobalProfile(profileID); err != nil {
		return err
	}
	return m.stateStore().RecordSwitch(profileID, "manual")
}

func (m *Manager) SaveAutoSwitch(config AutoSwitchConfig) error {
	if config.RotateIntervalSeconds != 0 && config.RotateIntervalSeconds < 60 {
		return errors.New("rotate_interval_seconds must be 0 or at least 60")
	}
	return m.stateStore().SetAutoSwitch(config)
}

func (m *Manager) EvaluateAutoSwitch(force bool) (*Profile, error) {
	store := m.stateStore()
	if store == nil {
		return nil, errors.New("plugin is not configured")
	}
	auto := store.AutoSwitch()
	// xAI 降智守护开启时，全局出口被标记降智也会执行切换：
	// 降智检测与降智切换是一体的，不需要额外开关。
	quality := store.Quality()
	// xAI 独立模式只切换 Quality.Route.ActiveProfileID，不能因为 xAI
	// 降智顺带改掉普通全局代理。只有 follow_global 才沿用旧切换语义。
	qualityEnabled := quality.Enabled && quality.Route.Mode == XAIRouteModeFollowGlobal
	if !auto.Enabled && !force && !qualityEnabled {
		return nil, nil
	}
	rules := store.Rules()
	current := store.Profile(rules.GlobalProfileID)
	reason := ""
	if current == nil {
		if rules.GlobalProfileID == "" {
			// 用户显式选择"不使用代理"：自动切换不干预。
			reason = ""
		} else if auto.FailoverEnabled || force {
			reason = "failover"
		}
	} else if !current.Healthy {
		if auto.FailoverEnabled || force {
			reason = "failover"
		}
	} else if current.Degraded && qualityEnabled {
		reason = "degraded"
	} else if force {
		reason = "manual-auto"
	} else if auto.Enabled && auto.RotateIntervalSeconds > 0 {
		interval := time.Duration(auto.RotateIntervalSeconds) * time.Second
		if auto.LastSwitchAt.IsZero() || time.Since(auto.LastSwitchAt) >= interval {
			reason = "interval"
		}
	}
	if reason == "" {
		return nil, nil
	}
	profiles := store.Profiles()
	if len(profiles) == 0 {
		return nil, errors.New("no WARP profiles available")
	}
	if reason == "degraded" && quality.Probe.Enabled && strings.TrimSpace(quality.Probe.Model) != "" {
		return m.switchToVerifiedQualityCandidate(current, profiles, auto)
	}
	start := -1
	for index, profile := range profiles {
		if current != nil && profile.ID == current.ID {
			start = index
			break
		}
	}
	for offset := 1; offset <= len(profiles); offset++ {
		index := (start + offset + len(profiles)) % len(profiles)
		candidate := profiles[index]
		if candidate == nil || !candidate.Healthy || candidate.Degraded || candidate.ID == rules.GlobalProfileID {
			continue
		}
		if auto.RequireDifferentIP && current != nil && current.ExitIP != "" && candidate.ExitIP == current.ExitIP {
			continue
		}
		if err := store.SetGlobalProfile(candidate.ID); err != nil {
			return nil, err
		}
		if err := store.RecordSwitch(candidate.ID, reason); err != nil {
			return nil, err
		}
		return store.Profile(candidate.ID), nil
	}
	return nil, errors.New("no eligible healthy profile for automatic switch")
}

func (m *Manager) DeleteProfile(id string) error {
	if _, err := sanitizeID(id); err != nil {
		return err
	}
	profile := m.stateStore().Profile(id)
	if profile == nil {
		return errors.New("profile not found")
	}
	_ = m.StopProfile(id)
	if profile.Mode == ProfileModeManaged && profile.Directory != "" {
		_ = os.RemoveAll(profile.Directory)
	}
	return m.stateStore().DeleteProfile(id)
}

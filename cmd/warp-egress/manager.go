package main

import (
	"context"
	"errors"
	"fmt"
	"net"
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
}

func NewManager() *Manager { return &Manager{processes: map[string]*managedProcess{}} }

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

	m.stopOwnedProcesses()
	m.mu.Lock()
	oldRelay := m.relay
	oldCancel := m.cancelHealth
	m.cfg = cfg
	m.store = NewStateStore(cfg.DataDir)
	m.relay = NewSOCKSRelay(net.JoinHostPort(cfg.ListenHost, fmt.Sprintf("%d", cfg.GlobalPort)), m.selectedProfileProxy)
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
	m.startHealthLoop()
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
	relay := m.relay
	ids := make([]string, 0, len(m.processes))
	for id := range m.processes {
		ids = append(ids, id)
	}
	m.mu.Unlock()
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
			}
		}
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
	return response
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
	profile := &Profile{ID: newID("warp"), Name: name, Mode: mode, CreatedAt: now, UpdatedAt: now}
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
		if err := m.RegisterManagedProfile(profile); err != nil {
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
	if !auto.Enabled && !force {
		return nil, nil
	}
	rules := store.Rules()
	current := store.Profile(rules.GlobalProfileID)
	reason := ""
	if current == nil || !current.Healthy {
		if auto.FailoverEnabled || force {
			reason = "failover"
		}
	} else if force {
		reason = "manual-auto"
	} else if auto.RotateIntervalSeconds > 0 {
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
		if candidate == nil || !candidate.Healthy || candidate.ID == rules.GlobalProfileID {
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

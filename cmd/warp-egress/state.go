package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type StateStore struct {
	mu    sync.RWMutex
	path  string
	state PersistedState
}

func NewStateStore(dataDir string) *StateStore {
	return &StateStore{path: filepath.Join(dataDir, "state.json"), state: PersistedState{Version: 1, Rules: Rules{ExactRules: map[string]string{}}}}
}

func (s *StateStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var state PersistedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	if state.Version == 0 {
		state.Version = 1
	}
	if state.Rules.ExactRules == nil {
		state.Rules.ExactRules = map[string]string{}
	}
	for _, p := range state.Profiles {
		if p == nil {
			continue
		}
		p.Running = false
		p.PID = 0
	}
	s.state = state
	return nil
}

func (s *StateStore) Save() error {
	s.mu.RLock()
	snapshot := cloneState(s.state)
	s.mu.RUnlock()
	return writeAtomicJSON(s.path, snapshot, 0o600)
}

func cloneState(src PersistedState) PersistedState {
	dst := PersistedState{Version: src.Version, Rules: cloneRules(src.Rules), Auto: src.Auto, Quality: src.Quality, Settings: src.Settings, AutoBoundAuths: map[string]string{}}
	for key, value := range src.AutoBoundAuths {
		dst.AutoBoundAuths[key] = value
	}
	for _, p := range src.Profiles {
		dst.Profiles = append(dst.Profiles, cloneProfile(p))
	}
	return dst
}

func cloneRules(src Rules) Rules {
	dst := Rules{GlobalProfileID: src.GlobalProfileID, TypeRules: append([]TypeRule(nil), src.TypeRules...), RegexRules: append([]RegexRule(nil), src.RegexRules...), ExactRules: map[string]string{}}
	for key, value := range src.ExactRules {
		dst.ExactRules[key] = value
	}
	return dst
}

func (s *StateStore) Snapshot() PersistedState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.state)
}

func (s *StateStore) Profiles() []*Profile {
	state := s.Snapshot()
	sort.Slice(state.Profiles, func(i, j int) bool {
		return strings.ToLower(state.Profiles[i].Name) < strings.ToLower(state.Profiles[j].Name)
	})
	return state.Profiles
}

func (s *StateStore) Profile(id string) *Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.state.Profiles {
		if p != nil && p.ID == id {
			return cloneProfile(p)
		}
	}
	return nil
}

func (s *StateStore) AddProfile(profile *Profile) error {
	s.mu.Lock()
	for _, existing := range s.state.Profiles {
		if existing != nil && existing.ID == profile.ID {
			s.mu.Unlock()
			return errors.New("profile id already exists")
		}
	}
	s.state.Profiles = append(s.state.Profiles, cloneProfile(profile))
	s.mu.Unlock()
	return s.Save()
}

func (s *StateStore) UpdateProfile(profile *Profile) error {
	s.mu.Lock()
	found := false
	for index, existing := range s.state.Profiles {
		if existing != nil && existing.ID == profile.ID {
			s.state.Profiles[index] = cloneProfile(profile)
			found = true
			break
		}
	}
	s.mu.Unlock()
	if !found {
		return errors.New("profile not found")
	}
	return s.Save()
}

// UpdateProfileQuiet 只更新内存中的 profile，不落盘。
// 用于高频的被动质量观测：观测统计由调用方防抖后统一 Save，
// 避免每条 usage 事件都全量写 state.json。
func (s *StateStore) UpdateProfileQuiet(profile *Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, existing := range s.state.Profiles {
		if existing != nil && existing.ID == profile.ID {
			s.state.Profiles[index] = cloneProfile(profile)
			return nil
		}
	}
	return errors.New("profile not found")
}

func (s *StateStore) DeleteProfile(id string) error {
	s.mu.Lock()
	next := s.state.Profiles[:0]
	found := false
	for _, p := range s.state.Profiles {
		if p != nil && p.ID == id {
			found = true
			continue
		}
		next = append(next, p)
	}
	s.state.Profiles = next
	if s.state.Rules.GlobalProfileID == id {
		s.state.Rules.GlobalProfileID = ""
	}
	for i := range s.state.Rules.TypeRules {
		if s.state.Rules.TypeRules[i].ProfileID == id {
			s.state.Rules.TypeRules[i].Enabled = false
		}
	}
	for i := range s.state.Rules.RegexRules {
		if s.state.Rules.RegexRules[i].ProfileID == id {
			s.state.Rules.RegexRules[i].Enabled = false
		}
	}
	for key, value := range s.state.Rules.ExactRules {
		if value == id {
			delete(s.state.Rules.ExactRules, key)
		}
	}
	s.mu.Unlock()
	if !found {
		return errors.New("profile not found")
	}
	return s.Save()
}

func (s *StateStore) Rules() Rules {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneRules(s.state.Rules)
}

func (s *StateStore) SetRules(rules Rules) error {
	if rules.ExactRules == nil {
		rules.ExactRules = map[string]string{}
	}
	s.mu.Lock()
	s.state.Rules = cloneRules(rules)
	s.mu.Unlock()
	return s.Save()
}

func (s *StateStore) SetGlobalProfile(id string) error {
	s.mu.Lock()
	s.state.Rules.GlobalProfileID = id
	s.mu.Unlock()
	return s.Save()
}

func (s *StateStore) AssignExact(authIndex, profileID string) error {
	s.mu.Lock()
	if s.state.Rules.ExactRules == nil {
		s.state.Rules.ExactRules = map[string]string{}
	}
	if strings.TrimSpace(profileID) == "" {
		delete(s.state.Rules.ExactRules, authIndex)
	} else {
		s.state.Rules.ExactRules[authIndex] = profileID
	}
	s.mu.Unlock()
	return s.Save()
}

func (s *StateStore) AutoSwitch() AutoSwitchConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Auto
}

func (s *StateStore) SetAutoSwitch(config AutoSwitchConfig) error {
	if config.RotateIntervalSeconds < 0 {
		config.RotateIntervalSeconds = 0
	}
	s.mu.Lock()
	previous := s.state.Auto
	config.LastSwitchAt = previous.LastSwitchAt
	config.LastProfileID = previous.LastProfileID
	config.LastReason = previous.LastReason
	s.state.Auto = config
	s.mu.Unlock()
	return s.Save()
}

func (s *StateStore) SetAutoBoundAuths(bound map[string]string) error {
	s.mu.Lock()
	if bound == nil {
		bound = map[string]string{}
	}
	s.state.AutoBoundAuths = bound
	s.mu.Unlock()
	return s.Save()
}

func (s *StateStore) AutoBoundAuths() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]string{}
	for key, value := range s.state.AutoBoundAuths {
		out[key] = value
	}
	return out
}

func (s *StateStore) Settings() SettingsConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.Settings.CleanupUnhealthy == false && s.state.Settings.CleanupUnhealthyMinutes == 0 {
		return defaultSettingsConfig()
	}
	return s.state.Settings
}

func (s *StateStore) SetSettings(config SettingsConfig) error {
	defaults := defaultSettingsConfig()
	if config.CleanupUnhealthyMinutes < 0 {
		config.CleanupUnhealthyMinutes = defaults.CleanupUnhealthyMinutes
	}
	s.mu.Lock()
	s.state.Settings = config
	s.mu.Unlock()
	return s.Save()
}

func (s *StateStore) RecordSwitch(profileID, reason string) error {
	s.mu.Lock()
	s.state.Auto.LastSwitchAt = time.Now()
	s.state.Auto.LastProfileID = profileID
	s.state.Auto.LastReason = reason
	s.mu.Unlock()
	return s.Save()
}

func (s *StateStore) Quality() QualityConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.Quality.Enabled == false && s.state.Quality.SoftTPS == 0 && s.state.Quality.ConsecutiveDegraded == 0 {
		return defaultQualityConfig()
	}
	return s.state.Quality
}

func (s *StateStore) SetQuality(config QualityConfig) error {
	defaults := defaultQualityConfig()
	// 只回填显式为零的字段（避免 UI 少传字段时把默认值清掉）。
	if config.SoftTPS <= 0 {
		config.SoftTPS = defaults.SoftTPS
	}
	if config.ConsecutiveDegraded <= 0 {
		config.ConsecutiveDegraded = defaults.ConsecutiveDegraded
	}
	if config.RecoveryObservations <= 0 {
		config.RecoveryObservations = defaults.RecoveryObservations
	}
	if config.MinGenerationMs <= 0 {
		config.MinGenerationMs = defaults.MinGenerationMs
	}
	if config.MinOutputTokens < 0 {
		config.MinOutputTokens = defaults.MinOutputTokens
	}
	if config.MinHealthy < 1 {
		config.MinHealthy = defaults.MinHealthy
	}
	if config.MaxProfiles < 1 {
		config.MaxProfiles = defaults.MaxProfiles
	}
	// 防配置死锁：min_healthy 不能超过 max_profiles，否则补充永远无法达标。
	if config.MinHealthy > config.MaxProfiles {
		config.MinHealthy = config.MaxProfiles
	}
	if config.ProvisionCooldownMin <= 0 {
		config.ProvisionCooldownMin = defaults.ProvisionCooldownMin
	}
	if config.Probe.MaxTokens <= 0 {
		config.Probe.MaxTokens = defaults.Probe.MaxTokens
	}
	if config.Probe.TimeoutSeconds <= 0 {
		config.Probe.TimeoutSeconds = defaults.Probe.TimeoutSeconds
	}
	s.mu.Lock()
	s.state.Quality = config
	s.mu.Unlock()
	return s.Save()
}

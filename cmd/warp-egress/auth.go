package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

func (m *Manager) resolveRoute(entry hostAuthFileEntry) EffectiveRoute {
	store := m.stateStore()
	if store == nil {
		return EffectiveRoute{RuleType: "none"}
	}
	rules := store.Rules()
	profileFor := func(id, ruleType, ruleKey string) EffectiveRoute {
		p := store.Profile(id)
		if p == nil {
			return EffectiveRoute{ProfileID: id, RuleType: ruleType, RuleKey: ruleKey}
		}
		return EffectiveRoute{ProfileID: id, RuleType: ruleType, RuleKey: ruleKey, ProxyURL: p.ProxyURL}
	}
	// usable 判断出口是否可用：被 xAI 降智守护标记（降智）的出口不参与分流。
	usable := func(id string) bool {
		p := store.Profile(id)
		return p != nil && !p.Degraded && p.ProxyURL != ""
	}
	exactKeys := []string{entry.AuthIndex, entry.ID, entry.Name}
	for _, key := range exactKeys {
		if key == "" {
			continue
		}
		if id := strings.TrimSpace(rules.ExactRules[key]); id != "" {
			if proxyURL, ok := customExactProxy(id); ok {
				return EffectiveRoute{RuleType: "exact", RuleKey: key, ProxyURL: proxyURL}
			}
			if id == exactDirect {
				// 不设置代理：清除 proxy_url，跟随 CPA 全局配置，且不被其他规则接管。
				return EffectiveRoute{RuleType: "inherit", RuleKey: key}
			}
			if usable(id) {
				return profileFor(id, "exact", key)
			}
			// 命中出口已降智：跳过该规则，继续按后续优先级匹配。
		}
	}
	for _, rule := range rules.RegexRules {
		if !rule.Enabled || strings.TrimSpace(rule.Pattern) == "" || !usable(rule.ProfileID) {
			continue
		}
		compiled, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}
		target := regexTargetValue(entry, rule.Target)
		if compiled.MatchString(target) {
			return profileFor(rule.ProfileID, "regex", rule.Pattern)
		}
	}
	provider := strings.ToLower(strings.TrimSpace(entry.Provider))
	authType := strings.ToLower(strings.TrimSpace(entry.Type))
	for _, rule := range rules.TypeRules {
		if !rule.Enabled || !usable(rule.ProfileID) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(rule.Key))
		if key != "" && (key == provider || key == authType) {
			return profileFor(rule.ProfileID, "type", rule.Key)
		}
	}
	if rules.GlobalProfileID != "" {
		global := store.Profile(rules.GlobalProfileID)
		if global != nil && !global.Degraded {
			return profileFor(global.ID, "global", "")
		}
		// 全局出口被标记降智：回落到第一个健康且未降智的出口，
		// 保证新请求不会继续打到降智的 IP 上。
		for _, p := range store.Profiles() {
			if p.ID == rules.GlobalProfileID {
				continue
			}
			if p.Healthy && !p.Degraded && p.Running && p.ProxyURL != "" {
				return EffectiveRoute{ProfileID: p.ID, RuleType: "global-fallback", ProxyURL: p.ProxyURL}
			}
		}
		return profileFor(global.ID, "global", "")
	}
	return EffectiveRoute{RuleType: "inherit"}
}

func regexTargetValue(entry hostAuthFileEntry, target string) string {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "label":
		return entry.Label
	case "email":
		return entry.Email
	case "provider":
		return entry.Provider
	case "type":
		return entry.Type
	case "id":
		return entry.ID
	case "auth_index":
		return entry.AuthIndex
	case "all":
		return strings.Join([]string{entry.Name, entry.Label, entry.Email, entry.Provider, entry.Type, entry.ID, entry.AuthIndex}, "\n")
	default:
		return entry.Name
	}
}

// customExactProxy 判断 exact 规则值是否为自定义代理（"custom:<proxy_url>"），并返回代理地址。
func customExactProxy(value string) (string, bool) {
	if strings.HasPrefix(value, exactCustomPrefix) {
		proxyURL := strings.TrimSpace(strings.TrimPrefix(value, exactCustomPrefix))
		return proxyURL, proxyURL != ""
	}
	return "", false
}

func validateRules(store *StateStore, rules Rules) error {
	for _, rule := range rules.RegexRules {
		if !rule.Enabled {
			continue
		}
		if strings.TrimSpace(rule.ID) == "" {
			rule.ID = newID("regex")
		}
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("invalid regex %q: %w", rule.Pattern, err)
		}
		if store.Profile(rule.ProfileID) == nil {
			return fmt.Errorf("regex rule profile not found: %s", rule.ProfileID)
		}
	}
	for _, rule := range rules.TypeRules {
		if !rule.Enabled {
			continue
		}
		if strings.TrimSpace(rule.Key) == "" {
			return errors.New("type rule key is required")
		}
		if store.Profile(rule.ProfileID) == nil {
			return fmt.Errorf("type rule profile not found: %s", rule.ProfileID)
		}
	}
	for _, id := range rules.ExactRules {
		if id != "" && id != exactDirect {
			if _, ok := customExactProxy(id); ok {
				continue
			}
			if store.Profile(id) == nil {
				return fmt.Errorf("exact rule profile not found: %s", id)
			}
		}
	}
	if rules.GlobalProfileID != "" && store.Profile(rules.GlobalProfileID) == nil {
		return errors.New("global profile not found")
	}
	return nil
}

func normalizeRules(rules Rules) Rules {
	if rules.ExactRules == nil {
		rules.ExactRules = map[string]string{}
	}
	for i := range rules.RegexRules {
		rules.RegexRules[i].Pattern = strings.TrimSpace(rules.RegexRules[i].Pattern)
		rules.RegexRules[i].Target = strings.ToLower(strings.TrimSpace(rules.RegexRules[i].Target))
		if rules.RegexRules[i].Target == "" {
			rules.RegexRules[i].Target = "name"
		}
		if rules.RegexRules[i].ID == "" {
			rules.RegexRules[i].ID = newID("regex")
		}
	}
	for i := range rules.TypeRules {
		rules.TypeRules[i].Key = strings.ToLower(strings.TrimSpace(rules.TypeRules[i].Key))
	}
	return rules
}

func (m *Manager) ListAuthFiles() ([]AuthFileView, error) {
	entries, err := callHostAuthList()
	if err != nil {
		return nil, err
	}
	views := make([]AuthFileView, 0, len(entries.Files))
	for _, entry := range entries.Files {
		view := AuthFileView{hostAuthFileEntry: entry, Effective: m.resolveRoute(entry)}
		if !entry.RuntimeOnly && entry.AuthIndex != "" {
			if got, errGet := callHostAuthGet(entry.AuthIndex); errGet == nil {
				entry.ProxyURL = proxyURLFromAuthJSON(got.JSON)
				view.hostAuthFileEntry = entry
			}
		}
		views = append(views, view)
	}
	sort.Slice(views, func(i, j int) bool {
		left := strings.ToLower(views[i].Provider + "/" + views[i].Name)
		right := strings.ToLower(views[j].Provider + "/" + views[j].Name)
		return left < right
	})
	return views, nil
}

func proxyURLFromAuthJSON(raw json.RawMessage) string {
	var data map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&data) != nil {
		return ""
	}
	value, _ := data["proxy_url"].(string)
	return strings.TrimSpace(value)
}

func setProxyURLInAuthJSON(raw json.RawMessage, proxyURL string) (json.RawMessage, bool, error) {
	var data map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		return nil, false, err
	}
	if data == nil {
		data = map[string]any{}
	}
	current, _ := data["proxy_url"].(string)
	if strings.TrimSpace(current) == strings.TrimSpace(proxyURL) {
		return raw, false, nil
	}
	if strings.TrimSpace(proxyURL) == "" {
		delete(data, "proxy_url")
	} else {
		data["proxy_url"] = proxyURL
	}
	updated, err := json.Marshal(data)
	return updated, true, err
}

func (m *Manager) ApplyRules() (ApplyRulesResult, error) {
	entries, err := callHostAuthList()
	if err != nil {
		return ApplyRulesResult{}, err
	}
	result := ApplyRulesResult{Total: len(entries.Files), Items: make([]ApplyItemResult, 0, len(entries.Files))}
	for _, entry := range entries.Files {
		item := ApplyItemResult{AuthIndex: entry.AuthIndex, Name: entry.Name}
		if entry.RuntimeOnly || entry.AuthIndex == "" || !strings.HasSuffix(strings.ToLower(entry.Name), ".json") {
			item.Skipped = true
			item.Error = "runtime-only or no physical JSON auth file"
			result.Skipped++
			result.Items = append(result.Items, item)
			continue
		}
		route := m.resolveRoute(entry)
		item.RuleType = route.RuleType
		item.ProfileID = route.ProfileID
		proxyURL := route.ProxyURL
		// Global rules inherit the host-level proxy-url, which should point at the plugin relay.
		if route.RuleType == "global" || route.RuleType == "inherit" {
			proxyURL = ""
		}
		item.ProxyURL = proxyURL
		got, errGet := callHostAuthGet(entry.AuthIndex)
		if errGet != nil {
			item.Error = errGet.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		updated, changed, errSet := setProxyURLInAuthJSON(got.JSON, proxyURL)
		if errSet != nil {
			item.Error = errSet.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		if !changed {
			item.Skipped = true
			result.Skipped++
			result.Items = append(result.Items, item)
			continue
		}
		saveName := got.Name
		if saveName == "" {
			saveName = entry.Name
		}
		if _, errSave := callHostAuthSave(saveName, updated); errSave != nil {
			item.Error = errSave.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		item.Changed = true
		result.Changed++
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func (m *Manager) AssignExact(req exactAssignRequest) (ApplyItemResult, error) {
	authIndex := strings.TrimSpace(req.AuthIndex)
	if authIndex == "" {
		return ApplyItemResult{}, errors.New("auth_index is required")
	}
	exactValue := strings.TrimSpace(req.ProfileID)
	if exactValue == "" && strings.TrimSpace(req.ProxyURL) != "" {
		exactValue = exactCustomPrefix + strings.TrimSpace(req.ProxyURL)
	}
	if exactValue != "" && exactValue != exactDirect {
		if _, isCustom := customExactProxy(exactValue); isCustom {
			// 自定义代理：直接写 CPA 认证文件的 proxy_url 字段，不依赖插件出口。
		} else if m.stateStore().Profile(exactValue) == nil {
			return ApplyItemResult{}, errors.New("profile not found")
		}
	}
	if err := m.stateStore().AssignExact(authIndex, exactValue); err != nil {
		return ApplyItemResult{}, err
	}
	if !req.ApplyNow {
		return ApplyItemResult{AuthIndex: authIndex, ProfileID: exactValue, Changed: true}, nil
	}
	entries, err := callHostAuthList()
	if err != nil {
		return ApplyItemResult{}, err
	}
	for _, entry := range entries.Files {
		if entry.AuthIndex != authIndex {
			continue
		}
		if entry.RuntimeOnly || !strings.HasSuffix(strings.ToLower(entry.Name), ".json") {
			return ApplyItemResult{}, errors.New("auth is runtime-only")
		}
		route := m.resolveRoute(entry)
		got, errGet := callHostAuthGet(authIndex)
		if errGet != nil {
			return ApplyItemResult{}, errGet
		}
		proxyURL := route.ProxyURL
		if route.RuleType == "global" || route.RuleType == "inherit" {
			proxyURL = ""
		}
		updated, changed, errSet := setProxyURLInAuthJSON(got.JSON, proxyURL)
		if errSet != nil {
			return ApplyItemResult{}, errSet
		}
		if changed {
			if _, errSave := callHostAuthSave(got.Name, updated); errSave != nil {
				return ApplyItemResult{}, errSave
			}
		}
		return ApplyItemResult{AuthIndex: authIndex, Name: entry.Name, RuleType: route.RuleType, ProfileID: route.ProfileID, ProxyURL: proxyURL, Changed: changed, Skipped: !changed}, nil
	}
	return ApplyItemResult{}, errors.New("auth file not found")
}

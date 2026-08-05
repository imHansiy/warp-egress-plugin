package main

import (
	"encoding/json"
	"errors"
	"strings"
)

// 自动绑定：xAI 降智守护开启后，默认检测所有 XAI / Grok 认证文件。
// 未绑定出口的 XAI 认证文件自动写入健康托管出口的 proxy_url（与 CPA 面板
// 分流同一字段，两处完全同步）；xAI 降智守护关闭时自动解绑，恢复"跟随全局"
// （全局"不使用代理"时即直连），不影响其他非 XAI 认证文件。
// 用户手动绑定的认证文件（proxy_url 已指向出口/自定义代理）不受影响、不记录。

// host auth 调用注入点（单元测试替换为 fake）。
var (
	authListForBind = func() (hostAuthListResponse, error) { return callHostAuthList() }
	authGetForBind  = func(authIndex string) (hostAuthGetResponse, error) { return callHostAuthGet(authIndex) }
	authSaveForBind = func(name string, payload json.RawMessage) error {
		_, err := callHostAuthSave(name, payload)
		return err
	}
)

func isXAIEntry(entry hostAuthFileEntry) bool {
	provider := strings.ToLower(entry.Provider)
	authType := strings.ToLower(entry.Type)
	return strings.Contains(provider, "xai") || strings.Contains(provider, "grok") ||
		strings.Contains(authType, "xai") || strings.Contains(authType, "grok")
}

// syncAutoBoundAuths 同步自动绑定：
//   - xAI 降智守护开启：扫描 XAI 认证文件，未绑定出口的写入健康托管出口 proxy_url；
//   - xAI 降智守护关闭：解绑此前自动绑定的认证文件（清空 proxy_url）。
func (m *Manager) syncAutoBoundAuths() error {
	store := m.stateStore()
	if store == nil {
		return nil
	}
	q := store.Quality()
	bound := store.AutoBoundAuths()
	entries, err := authListForBind()
	if err != nil {
		return err
	}
	if !q.Enabled {
		// 解绑所有自动绑定的认证文件。
		changed := false
		for _, entry := range entries.Files {
			if entry.AuthIndex == "" {
				continue
			}
			if _, auto := bound[entry.AuthIndex]; auto {
				if errSet := m.setAuthProxy(entry, ""); errSet == nil {
					changed = true
				}
			}
		}
		if changed {
			return store.SetAutoBoundAuths(map[string]string{})
		}
		return nil
	}
	// 自动绑定目标出口：健康且未降智的托管出口。
	target := ""
	for _, p := range store.Profiles() {
		if p.Mode == ProfileModeManaged && p.Healthy && !p.Degraded && p.ProxyURL != "" {
			target = p.ProxyURL
			break
		}
	}
	if target == "" {
		return nil
	}
	next := map[string]string{}
	for _, entry := range entries.Files {
		authIndex := entry.AuthIndex
		if authIndex == "" || entry.RuntimeOnly {
			continue
		}
		if !isXAIEntry(entry) {
			continue
		}
		proxy := entry.ProxyURL
		if proxy == "" {
			if got, errGet := authGetForBind(authIndex); errGet == nil {
				proxy = proxyURLFromAuthJSON(got.JSON)
			}
		}
		if proxy == target {
			next[authIndex] = target
			continue
		}
		if proxy != "" {
			// 已绑定其他出口（托管或自定义）：用户手动管理，不动不记录。
			continue
		}
		if errSet := m.setAuthProxy(entry, target); errSet == nil {
			next[authIndex] = target
		}
	}
	// 记录中已不再自动绑定的条目（用户改绑/文件删除）清理掉。
	for index := range bound {
		if _, keep := next[index]; !keep {
			delete(next, index)
		}
	}
	return store.SetAutoBoundAuths(next)
}

// setAuthProxy 写入/清除认证文件的 proxy_url（host.auth.save）。
func (m *Manager) setAuthProxy(entry hostAuthFileEntry, proxyURL string) error {
	got, err := authGetForBind(entry.AuthIndex)
	if err != nil {
		return err
	}
	updated, changed, err := setProxyURLInAuthJSON(got.JSON, proxyURL)
	if err != nil || !changed {
		return err
	}
	name := got.Name
	if name == "" {
		name = entry.Name
	}
	if name == "" {
		return errors.New("auth name not found")
	}
	return authSaveForBind(name, updated)
}

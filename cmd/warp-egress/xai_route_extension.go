package main

import (
	"errors"
	"net"
	"net/url"
	"sort"
	"strings"
)

// xaiRouteExtension 是可选的目标路由 Adapter。扩展总开关关闭时始终
// handled=false，核心中继完全按原普通全局出口逻辑工作。
type xaiRouteExtension struct {
	store func() *StateStore
}

func init() {
	registerTargetRouteExtension(func(store func() *StateStore) targetRouteExtension {
		return &xaiRouteExtension{store: store}
	})
}

func (x *xaiRouteExtension) DecideTarget(target string) (relayRoute, bool, error) {
	if x == nil || x.store == nil {
		return relayRoute{}, false, nil
	}
	store := x.store()
	if store == nil {
		return relayRoute{}, false, nil
	}
	quality := store.Quality()
	if !quality.Enabled || !xaiTargetMatches(target, quality.Route.Hosts) {
		return relayRoute{}, false, nil
	}
	switch quality.Route.Mode {
	case XAIRouteModeDirect:
		return relayRoute{Kind: "xai-direct", Direct: true}, true, nil
	case XAIRouteModeFollowGlobal:
		route, err := ordinaryRelayRoute(store, "xai-follow-global")
		return route, true, err
	case XAIRouteModeIndependent:
		profile := store.Profile(quality.Route.ActiveProfileID)
		if !profileUsableForXAIRoute(profile) {
			// 独立模式明确承诺“xAI 只走健康代理”，因此扩展开启期间
			// 没有候选就拒绝连接；关闭扩展则恢复核心普通路由。
			return relayRoute{}, true, errors.New("xAI route blocked: no healthy independent egress")
		}
		return relayRoute{ProxyURL: profile.ProxyURL, ProfileID: profile.ID, Kind: "xai-independent"}, true, nil
	default:
		return relayRoute{}, false, nil
	}
}

func profileUsableForXAIRoute(profile *Profile) bool {
	if profile == nil || !profile.Healthy || profile.Degraded || strings.TrimSpace(profile.ProxyURL) == "" {
		return false
	}
	return profile.Mode != ProfileModeManaged || profile.Running
}

func normalizeXAIHosts(hosts []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(hosts))
	for _, raw := range hosts {
		host := normalizeXAIHost(raw)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		out = append(out, host)
	}
	sort.Strings(out)
	return out
}

func normalizeXAIHost(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	wildcard := strings.HasPrefix(value, "*.")
	if wildcard {
		value = strings.TrimPrefix(value, "*.")
	}
	parsedValue := value
	if !strings.Contains(parsedValue, "://") {
		parsedValue = "//" + parsedValue
	}
	parsed, err := url.Parse(parsedValue)
	if err != nil {
		return ""
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || net.ParseIP(host) != nil {
		return ""
	}
	if wildcard {
		return "*." + host
	}
	return host
}

func xaiTargetMatches(target string, hosts []string) bool {
	host := targetHost(target)
	if host == "" {
		return false
	}
	// Hosts 在 Load/SetQuality 时已规范化；请求热路径只做小列表精确比较，
	// 不重复解析 URL、排序或访问认证目录。
	for _, candidate := range hosts {
		if strings.HasPrefix(candidate, "*.") {
			suffix := strings.TrimPrefix(candidate, "*")
			if strings.HasSuffix(host, suffix) && host != strings.TrimPrefix(suffix, ".") {
				return true
			}
			continue
		}
		if host == candidate {
			return true
		}
	}
	return false
}

func targetHost(target string) string {
	value := strings.TrimSpace(target)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	}
	parsedValue := value
	if !strings.Contains(parsedValue, "://") {
		parsedValue = "//" + parsedValue
	}
	if parsed, err := url.Parse(parsedValue); err == nil && parsed.Hostname() != "" {
		return strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	}
	return strings.TrimSuffix(strings.ToLower(strings.Trim(value, "[]")), ".")
}

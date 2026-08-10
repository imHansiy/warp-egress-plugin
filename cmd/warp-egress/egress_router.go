package main

import (
	"errors"
	"strings"
)

type relayRoute struct {
	ProxyURL  string
	ProfileID string
	Kind      string
	Direct    bool
}

// targetRouteExtension 是核心中继为可选目标路由模块提供的 seam。
// handled=false 表示扩展不接管该目标，核心继续使用普通全局出口逻辑。
type targetRouteExtension interface {
	DecideTarget(target string) (route relayRoute, handled bool, err error)
}

type targetRouteExtensionFactory func(store func() *StateStore) targetRouteExtension

var targetRouteExtensionFactories []targetRouteExtensionFactory

// registerTargetRouteExtension 由扩展文件在 init 中自注册。核心 EgressRouter
// 不引用任何 provider；删除路由适配器后，普通全局路由仍独立工作。
func registerTargetRouteExtension(factory targetRouteExtensionFactory) {
	if factory != nil {
		targetRouteExtensionFactories = append(targetRouteExtensionFactories, factory)
	}
}

// EgressRouter 是 CPA 本地 SOCKS 中继唯一的目标路由决策点。
// 它只读取少量配置与出口状态，不扫描认证文件，因此账号数量不会进入请求热路径。
type EgressRouter struct {
	store      func() *StateStore
	extensions []targetRouteExtension
}

func NewEgressRouter(store func() *StateStore) *EgressRouter {
	router := &EgressRouter{store: store}
	for _, factory := range targetRouteExtensionFactories {
		if extension := factory(store); extension != nil {
			router.extensions = append(router.extensions, extension)
		}
	}
	return router
}

func (r *EgressRouter) Decide(target string) (relayRoute, error) {
	if r == nil || r.store == nil {
		return relayRoute{}, errors.New("plugin is not configured")
	}
	store := r.store()
	if store == nil {
		return relayRoute{}, errors.New("plugin is not configured")
	}
	for _, extension := range r.extensions {
		route, handled, err := extension.DecideTarget(target)
		if err != nil || handled {
			return route, err
		}
	}
	return ordinaryRelayRoute(store, "global")
}

func ordinaryRelayRoute(store *StateStore, kind string) (relayRoute, error) {
	rules := store.Rules()
	if rules.GlobalProfileID == "" {
		return relayRoute{Kind: kind + "-direct", Direct: true}, nil
	}
	profile := store.Profile(rules.GlobalProfileID)
	if profile == nil || strings.TrimSpace(profile.ProxyURL) == "" {
		return relayRoute{}, errors.New("selected global profile is unavailable")
	}
	return relayRoute{ProxyURL: profile.ProxyURL, ProfileID: profile.ID, Kind: kind}, nil
}

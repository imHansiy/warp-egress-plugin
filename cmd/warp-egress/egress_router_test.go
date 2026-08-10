package main

import (
	"testing"
	"time"
)

func newRouterTestStore(t *testing.T) *StateStore {
	t.Helper()
	store := NewStateStore(t.TempDir())
	profiles := []*Profile{
		{ID: "ordinary", Name: "ordinary", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true},
		{ID: "xai", Name: "xai", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41002", Healthy: true, QualityClassification: "healthy", QualityCheckedAt: time.Now()},
		{ID: "standby", Name: "standby", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41003", Healthy: true, QualityClassification: "healthy", QualityCheckedAt: time.Now()},
	}
	for _, profile := range profiles {
		if err := store.AddProfile(profile); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestEgressRouterIndependentXAIWithOrdinaryGlobalDisabled(t *testing.T) {
	store := newRouterTestStore(t)
	quality := store.Quality()
	quality.Enabled = true
	quality.Route.Mode = XAIRouteModeIndependent
	quality.Route.ActiveProfileID = "xai"
	if err := store.SetQuality(quality); err != nil {
		t.Fatal(err)
	}
	router := NewEgressRouter(func() *StateStore { return store })

	xai, err := router.Decide("cli-chat-proxy.grok.com:443")
	if err != nil || xai.ProfileID != "xai" || xai.Direct {
		t.Fatalf("xAI route=%+v err=%v", xai, err)
	}
	ordinary, err := router.Decide("example.com:443")
	if err != nil || !ordinary.Direct {
		t.Fatalf("ordinary route=%+v err=%v", ordinary, err)
	}
}

func TestDisabledXAIExtensionRestoresCoreGlobalRouting(t *testing.T) {
	store := newRouterTestStore(t)
	if err := store.SetRules(Rules{GlobalProfileID: "ordinary", ExactRules: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	quality := store.Quality()
	quality.Enabled = false
	quality.Route.Mode = XAIRouteModeIndependent
	quality.Route.ActiveProfileID = "xai"
	if err := store.SetQuality(quality); err != nil {
		t.Fatal(err)
	}
	router := NewEgressRouter(func() *StateStore { return store })
	route, err := router.Decide("cli-chat-proxy.grok.com:443")
	if err != nil || route.ProfileID != "ordinary" || route.Kind != "global" {
		t.Fatalf("disabled extension must not intercept: route=%+v err=%v", route, err)
	}
}

func TestEgressRouterIndependentFailsClosedWithoutHealthyProfile(t *testing.T) {
	store := newRouterTestStore(t)
	quality := store.Quality()
	quality.Enabled = true
	quality.Route.Mode = XAIRouteModeIndependent
	quality.Route.ActiveProfileID = "xai"
	if err := store.SetQuality(quality); err != nil {
		t.Fatal(err)
	}
	profile := store.Profile("xai")
	profile.Degraded = true
	if err := store.UpdateProfile(profile); err != nil {
		t.Fatal(err)
	}
	router := NewEgressRouter(func() *StateStore { return store })
	if route, err := router.Decide("api.x.ai:443"); err == nil || route.Direct {
		t.Fatalf("degraded xAI route must fail closed: route=%+v err=%v", route, err)
	}
}

func TestEgressRouterFollowGlobalAndCustomHost(t *testing.T) {
	store := newRouterTestStore(t)
	if err := store.SetRules(Rules{GlobalProfileID: "ordinary", ExactRules: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	quality := store.Quality()
	quality.Enabled = true
	quality.Route.Mode = XAIRouteModeFollowGlobal
	quality.Route.Hosts = []string{"HTTPS://Gateway.Example.COM/v1", "*.xai.internal"}
	if err := store.SetQuality(quality); err != nil {
		t.Fatal(err)
	}
	router := NewEgressRouter(func() *StateStore { return store })
	for _, target := range []string{"gateway.example.com:443", "edge.xai.internal:443"} {
		route, err := router.Decide(target)
		if err != nil || route.ProfileID != "ordinary" {
			t.Fatalf("target=%s route=%+v err=%v", target, route, err)
		}
	}
}

func TestEgressRouterHotPathNeverScansAuthInventory(t *testing.T) {
	store := newRouterTestStore(t)
	quality := store.Quality()
	quality.Enabled = true
	quality.Route.ActiveProfileID = "xai"
	if err := store.SetQuality(quality); err != nil {
		t.Fatal(err)
	}
	calls := 0
	oldList := authListForProbe
	authListForProbe = func() (hostAuthListResponse, error) {
		calls++
		return hostAuthListResponse{}, nil
	}
	defer func() { authListForProbe = oldList }()
	router := NewEgressRouter(func() *StateStore { return store })
	for i := 0; i < 1000; i++ {
		if _, err := router.Decide("cli-chat-proxy.grok.com:443"); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatalf("route hot path scanned auth inventory %d times", calls)
	}
}

func TestXAISwitchNeverChangesOrdinaryGlobal(t *testing.T) {
	store := newRouterTestStore(t)
	if err := store.SetRules(Rules{GlobalProfileID: "ordinary", ExactRules: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	quality := store.Quality()
	quality.Enabled = true
	quality.Route.Mode = XAIRouteModeIndependent
	quality.Route.ActiveProfileID = "xai"
	if err := store.SetQuality(quality); err != nil {
		t.Fatal(err)
	}
	failed := store.Profile("xai")
	failed.Degraded = true
	if err := store.UpdateProfile(failed); err != nil {
		t.Fatal(err)
	}
	ordinary := store.Profile("ordinary")
	ordinary.Healthy = false
	if err := store.UpdateProfile(ordinary); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	manager.store = store
	selected, err := manager.EvaluateXAISwitch(false)
	if err != nil || selected == nil || selected.ID != "standby" {
		t.Fatalf("selected=%+v err=%v", selected, err)
	}
	if got := store.Rules().GlobalProfileID; got != "ordinary" {
		t.Fatalf("ordinary global changed to %q", got)
	}
	if got := store.Quality().Route.ActiveProfileID; got != "standby" {
		t.Fatalf("xAI active=%q", got)
	}
}

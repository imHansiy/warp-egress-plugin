package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseConfig(t *testing.T) {
	raw := []byte("data-dir: ./data\nglobal-port: 40100\nauto-start: false\nhealth-check-interval: 2m\n")
	cfg, err := parseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GlobalPort != 40100 || cfg.AutoStart || cfg.HealthCheckInterval.Seconds() != 120 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if !filepath.IsAbs(cfg.DataDir) {
		t.Fatalf("data dir must be absolute: %s", cfg.DataDir)
	}
}

func TestParseTrace(t *testing.T) {
	values := parseTrace([]byte("ip=203.0.113.10\nwarp=on\ncolo=SIN\n"))
	if values["ip"] != "203.0.113.10" || values["warp"] != "on" || values["colo"] != "SIN" {
		t.Fatalf("unexpected trace: %#v", values)
	}
}

func TestSetProxyURLInAuthJSON(t *testing.T) {
	raw := json.RawMessage(`{"type":"codex","email":"a@example.com","proxy_url":"socks5://127.0.0.1:1"}`)
	updated, changed, err := setProxyURLInAuthJSON(raw, "socks5://127.0.0.1:2")
	if err != nil || !changed {
		t.Fatalf("update failed: changed=%v err=%v", changed, err)
	}
	if got := proxyURLFromAuthJSON(updated); got != "socks5://127.0.0.1:2" {
		t.Fatalf("unexpected proxy: %s", got)
	}
	removed, changed, err := setProxyURLInAuthJSON(updated, "")
	if err != nil || !changed {
		t.Fatalf("remove failed: changed=%v err=%v", changed, err)
	}
	if got := proxyURLFromAuthJSON(removed); got != "" {
		t.Fatalf("proxy was not removed: %s", got)
	}
}

func TestRulePrecedence(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager()
	manager.cfg = defaultConfig()
	manager.store = NewStateStore(dir)
	profiles := []*Profile{
		{ID: "global", Name: "global", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001"},
		{ID: "type", Name: "type", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41002"},
		{ID: "regex", Name: "regex", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41003"},
		{ID: "exact", Name: "exact", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41004"},
	}
	for _, p := range profiles {
		if err := manager.store.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	rules := Rules{GlobalProfileID: "global", TypeRules: []TypeRule{{Key: "codex", ProfileID: "type", Enabled: true}}, RegexRules: []RegexRule{{ID: "r1", Pattern: "example\\.com", Target: "email", ProfileID: "regex", Enabled: true}}, ExactRules: map[string]string{"idx-1": "exact"}}
	if err := manager.store.SetRules(rules); err != nil {
		t.Fatal(err)
	}
	entry := hostAuthFileEntry{AuthIndex: "idx-1", Name: "codex-a.json", Provider: "codex", Email: "a@example.com"}
	if route := manager.resolveRoute(entry); route.ProfileID != "exact" || route.RuleType != "exact" {
		t.Fatalf("unexpected exact route: %+v", route)
	}
	delete(rules.ExactRules, "idx-1")
	_ = manager.store.SetRules(rules)
	if route := manager.resolveRoute(entry); route.ProfileID != "regex" || route.RuleType != "regex" {
		t.Fatalf("unexpected regex route: %+v", route)
	}
	rules.RegexRules[0].Enabled = false
	_ = manager.store.SetRules(rules)
	if route := manager.resolveRoute(entry); route.ProfileID != "type" || route.RuleType != "type" {
		t.Fatalf("unexpected type route: %+v", route)
	}
	rules.TypeRules[0].Enabled = false
	_ = manager.store.SetRules(rules)
	if route := manager.resolveRoute(entry); route.ProfileID != "global" || route.RuleType != "global" {
		t.Fatalf("unexpected global route: %+v", route)
	}
}

func TestAutomaticSwitch(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager()
	manager.cfg = defaultConfig()
	manager.store = NewStateStore(dir)
	profiles := []*Profile{
		{ID: "a", Name: "a", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41001", Healthy: true, ExitIP: "203.0.113.1"},
		{ID: "b", Name: "b", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41002", Healthy: true, ExitIP: "203.0.113.2"},
	}
	for _, profile := range profiles {
		if err := manager.store.AddProfile(profile); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.store.SetGlobalProfile("a"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveAutoSwitch(AutoSwitchConfig{Enabled: true, FailoverEnabled: true, RequireDifferentIP: true}); err != nil {
		t.Fatal(err)
	}
	selected, err := manager.EvaluateAutoSwitch(true)
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.ID != "b" {
		t.Fatalf("unexpected selected profile: %+v", selected)
	}
	if got := manager.store.Rules().GlobalProfileID; got != "b" {
		t.Fatalf("global profile not updated: %s", got)
	}
}

func TestSOCKSRelayEndToEnd(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("relay-ok"))
	}))
	defer target.Close()

	upstreamListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstreamListener.Close()
	go func() {
		for {
			client, acceptErr := upstreamListener.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer client.Close()
				destination, acceptSOCKSErr := acceptSOCKS5(client)
				if acceptSOCKSErr != nil {
					return
				}
				remote, dialErr := net.DialTimeout("tcp", destination, 3*time.Second)
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

	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	relayAddress := portListener.Addr().String()
	_ = portListener.Close()
	relay := NewSOCKSRelay(relayAddress, func() (string, error) {
		return "socks5://" + upstreamListener.Addr().String(), nil
	})
	if err := relay.Start(); err != nil {
		t.Fatal(err)
	}
	defer relay.Close()

	transport := &http.Transport{DialContext: func(ctx context.Context, _ string, address string) (net.Conn, error) {
		return dialSOCKS5(ctx, relayAddress, address)
	}}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "relay-ok" {
		t.Fatalf("unexpected response: %s", body)
	}
	_, targetPort, _ := net.SplitHostPort(upstreamListener.Addr().String())
	if _, err := strconv.Atoi(targetPort); err != nil {
		t.Fatal(err)
	}
}

func TestPanelHTMLContainsImprovedWorkflows(t *testing.T) {
	required := []string{
		"data-view=\"overview\"",
		"data-view=\"profiles\"",
		"data-view=\"routing\"",
		"data-view=\"auth\"",
		"data-view=\"automation\"",
		"data-action=\"bulk-assign\"",
		"data-action=\"open-switch\"",
		"rulesDirty",
		"connectModal",
	}
	for _, marker := range required {
		if !strings.Contains(panelHTML, marker) {
			t.Fatalf("panel is missing %q", marker)
		}
	}
	if strings.Contains(strings.ToLower(panelHTML), "#16a34a") || strings.Contains(strings.ToLower(panelHTML), "green") {
		t.Fatal("panel must not use the disallowed green theme")
	}
}

func TestPluginVersionIsUpdated(t *testing.T) {
	if pluginVersion != "0.2.0" {
		t.Fatalf("pluginVersion = %q, want 0.2.0", pluginVersion)
	}
}

package main

import (
	"testing"
	"time"
)

func TestCleanupUnhealthy(t *testing.T) {
	manager := newTestManager(t)
	now := time.Now()
	profiles := []*Profile{
		{ID: "bad", Name: "bad", Mode: ProfileModeManaged, ProxyURL: "socks5://127.0.0.1:41001", CreatedAt: now.Add(-48 * time.Hour), LastChecked: now.Add(-2 * time.Hour)},
		{ID: "ok", Name: "ok", Mode: ProfileModeManaged, ProxyURL: "socks5://127.0.0.1:41002", Healthy: true, CreatedAt: now.Add(-48 * time.Hour), LastChecked: now},
		{ID: "fresh", Name: "fresh", Mode: ProfileModeManaged, ProxyURL: "socks5://127.0.0.1:41003", CreatedAt: now.Add(-2 * time.Minute), LastChecked: now},
		{ID: "global", Name: "global", Mode: ProfileModeManaged, ProxyURL: "socks5://127.0.0.1:41004", CreatedAt: now.Add(-48 * time.Hour), LastChecked: now.Add(-2 * time.Hour)},
		{ID: "ext", Name: "ext", Mode: ProfileModeExternal, ProxyURL: "socks5://127.0.0.1:41005", CreatedAt: now.Add(-48 * time.Hour), LastChecked: now.Add(-2 * time.Hour)},
	}
	for _, p := range profiles {
		if err := manager.store.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.store.SetGlobalProfile("global"); err != nil {
		t.Fatal(err)
	}
	settings := manager.store.Settings()
	settings.CleanupUnhealthy = true
	settings.CleanupUnhealthyMinutes = 60
	if err := manager.store.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	manager.cleanupUnhealthy()
	if manager.store.Profile("bad") != nil {
		t.Fatal("long-unhealthy unreferenced managed egress must be cleaned")
	}
	if manager.store.Profile("ok") == nil {
		t.Fatal("healthy egress must survive")
	}
	if manager.store.Profile("fresh") == nil {
		t.Fatal("newly created egress must be protected from cleanup")
	}
	if manager.store.Profile("global") == nil {
		t.Fatal("referenced (global) egress must survive")
	}
	if manager.store.Profile("ext") == nil {
		t.Fatal("external egress must never be auto-cleaned")
	}
}

func TestCleanupDisabled(t *testing.T) {
	manager := newTestManager(t)
	now := time.Now()
	bad := &Profile{ID: "bad", Name: "bad", Mode: ProfileModeManaged, ProxyURL: "socks5://127.0.0.1:41001", CreatedAt: now.Add(-48 * time.Hour), LastChecked: now.Add(-2 * time.Hour)}
	if err := manager.store.AddProfile(bad); err != nil {
		t.Fatal(err)
	}
	settings := manager.store.Settings()
	settings.CleanupUnhealthy = false
	if err := manager.store.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	manager.cleanupUnhealthy()
	if manager.store.Profile("bad") == nil {
		t.Fatal("cleanup disabled must not delete egress")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	manager := newTestManager(t)
	got := manager.store.Settings()
	if got.CleanupUnhealthy || got.CleanupUnhealthyMinutes != 10 {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	got.CleanupUnhealthy = true
	got.CleanupUnhealthyMinutes = 30
	if err := manager.store.SetSettings(got); err != nil {
		t.Fatal(err)
	}
	again := manager.store.Settings()
	if !again.CleanupUnhealthy || again.CleanupUnhealthyMinutes != 30 {
		t.Fatalf("settings did not round trip: %+v", again)
	}
}

package main

import (
	"time"
)

// 通用设置：自动清理不健康出口。
// 与 xAI 降智守护完全独立：连通失败（Healthy=false）且未被任何规则
// 引用的托管出口自动删除，防止异常出口堆积占满出口池导致自动补充
// 无法继续。新创建的出口有保护期，避免启动抖动误删。

// cleanupUnhealthy 删除长期不健康的托管出口（未被规则引用）。
func (m *Manager) cleanupUnhealthy() {
	store := m.stateStore()
	if store == nil {
		return
	}
	settings := store.Settings()
	if !settings.CleanupUnhealthy {
		return
	}
	profiles := store.Profiles()
	if len(profiles) == 0 {
		return
	}
	referenced := map[string]bool{}
	rules := store.Rules()
	if rules.GlobalProfileID != "" {
		referenced[rules.GlobalProfileID] = true
	}
	for _, rule := range rules.TypeRules {
		if rule.Enabled {
			referenced[rule.ProfileID] = true
		}
	}
	for _, rule := range rules.RegexRules {
		if rule.Enabled {
			referenced[rule.ProfileID] = true
		}
	}
	for _, id := range rules.ExactRules {
		if id != "" && id != exactDirect {
			if _, custom := customExactProxy(id); !custom {
				referenced[id] = true
			}
		}
	}
	now := time.Now()
	for _, p := range profiles {
		if p == nil || p.Mode != ProfileModeManaged || p.Healthy || referenced[p.ID] {
			continue
		}
		// 保护期：新创建的出口（含刚补充的）不立即清理。
		if now.Sub(p.CreatedAt) < 10*time.Minute {
			continue
		}
		// 持续异常时长：0 表示立即清理；>0 要求最近检测早于阈值。
		if settings.CleanupUnhealthyMinutes > 0 {
			if p.LastChecked.IsZero() || !p.LastChecked.Before(now.Add(-time.Duration(settings.CleanupUnhealthyMinutes)*time.Minute)) {
				continue
			}
		}
		if err := m.DeleteProfile(p.ID); err == nil {
			m.setLastError("自动清理不健康出口: " + p.Name)
		}
	}
}

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

func pluginRegistration() registration {
	return registration{
		SchemaVersion: schemaVersion,
		Metadata: pluginMetadata{
			Name:             pluginID,
			Version:          pluginVersion,
			Author:           "寒思逸",
			GitHubRepository: "https://github.com/imHansiy/warp-egress-plugin",
			Logo:             "",
			ConfigFields: []configField{
				{Name: "data-dir", Type: "string", Description: "插件状态、WARP 注册和日志目录。"},
				{Name: "wgcf-path", Type: "string", Description: "wgcf 可执行文件路径。"},
				{Name: "wireproxy-path", Type: "string", Description: "wireproxy 可执行文件路径。"},
				{Name: "listen-host", Type: "string", Description: "本地 SOCKS5 监听地址，默认 127.0.0.1。"},
				{Name: "global-port", Type: "integer", Description: "CLIProxyAPI 全局 proxy-url 指向的固定 SOCKS5 端口。"},
				{Name: "profile-port-start", Type: "integer", Description: "WARP 配置实例端口池起始值。"},
				{Name: "profile-port-end", Type: "integer", Description: "WARP 配置实例端口池结束值。"},
				{Name: "auto-start", Type: "boolean", Description: "插件启动时自动启动已注册的托管 WARP 实例。"},
				{Name: "health-check-interval", Type: "string", Description: "自动出口检测间隔，如 60s；设为 0 禁用。"},
				{Name: "ip-check-url", Type: "string", Description: "出口 IP/WARP 状态检测地址。"},
				{Name: "allow-remote-listen", Type: "boolean", Description: "允许 SOCKS5 监听非回环地址；默认关闭。"},
			},
		},
		Capabilities: registrationCapabilities{ManagementAPI: true, UsagePlugin: true, RequestInterceptor: true, ResponseStreamInterceptor: true},
	}
}

func managementRoutes() managementRegistration {
	return managementRegistration{
		Resources: []resourceRoute{{Path: "/panel", Menu: "WARP 出口管理", Description: "管理 WARP 配置、全局出口与认证文件出口规则。"}},
		Routes: []managementRoute{
			{Method: http.MethodGet, Path: "/warp-egress/status"},
			{Method: http.MethodGet, Path: "/warp-egress/profiles"},
			{Method: http.MethodPost, Path: "/warp-egress/profiles/create"},
			{Method: http.MethodPost, Path: "/warp-egress/profiles/import"},
			{Method: http.MethodPost, Path: "/warp-egress/profiles/action"},
			{Method: http.MethodPost, Path: "/warp-egress/profiles/delete"},
			{Method: http.MethodPost, Path: "/warp-egress/global/switch"},
			{Method: http.MethodGet, Path: "/warp-egress/auth-files"},
			{Method: http.MethodPost, Path: "/warp-egress/auth-files/assign"},
			{Method: http.MethodGet, Path: "/warp-egress/rules"},
			{Method: http.MethodPost, Path: "/warp-egress/rules/save"},
			{Method: http.MethodPost, Path: "/warp-egress/rules/apply"},
			{Method: http.MethodGet, Path: "/warp-egress/auto"},
			{Method: http.MethodPost, Path: "/warp-egress/auto/save"},
			{Method: http.MethodPost, Path: "/warp-egress/auto/run"},
			{Method: http.MethodGet, Path: "/warp-egress/quality"},
			{Method: http.MethodPost, Path: "/warp-egress/quality/save"},
			{Method: http.MethodPost, Path: "/warp-egress/profiles/probe"},
			{Method: http.MethodPost, Path: "/warp-egress/quality/prune"},
		},
	}
}

func normalizeManagementPath(path string) string {
	path = strings.TrimSpace(path)
	if strings.HasSuffix(path, "/panel") {
		return "/panel"
	}
	if idx := strings.Index(path, "/warp-egress/"); idx >= 0 {
		return path[idx:]
	}
	return path
}

func (m *Manager) HandleManagement(raw []byte) (managementResponse, error) {
	var req managementRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return managementResponse{}, err
		}
	}
	path := normalizeManagementPath(req.Path)
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	if path == "/panel" {
		return htmlResponse(http.StatusOK, []byte(renderPanelHTML(m))), nil
	}
	switch method + " " + path {
	case "GET /warp-egress/status":
		return jsonResponse(http.StatusOK, m.Status()), nil
	case "GET /warp-egress/profiles":
		return jsonResponse(http.StatusOK, map[string]any{"profiles": m.stateStore().Profiles()}), nil
	case "POST /warp-egress/profiles/create":
		var body createProfileRequest
		if err := decodeJSON(req.Body, &body); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		profile, err := m.CreateProfile(body)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		return jsonResponse(http.StatusCreated, profile), nil
	case "POST /warp-egress/profiles/import":
		var body importProfileRequest
		if err := decodeJSON(req.Body, &body); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		profile, err := m.ImportProfile(body)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		return jsonResponse(http.StatusCreated, profile), nil
	case "POST /warp-egress/profiles/action":
		var body profileActionRequest
		if err := decodeJSON(req.Body, &body); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		err := m.runProfileAction(body)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		return jsonResponse(http.StatusOK, m.stateStore().Profile(body.ID)), nil
	case "POST /warp-egress/profiles/delete":
		var body profileDeleteRequest
		if err := decodeJSON(req.Body, &body); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		if err := m.DeleteProfile(body.ID); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{"status": "ok"}), nil
	case "POST /warp-egress/global/switch":
		var body globalSwitchRequest
		if err := decodeJSON(req.Body, &body); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		if err := m.SwitchGlobal(body.ProfileID); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		return jsonResponse(http.StatusOK, m.Status()), nil
	case "GET /warp-egress/auth-files":
		files, err := m.ListAuthFiles()
		if err != nil {
			return jsonResponse(http.StatusBadGateway, map[string]any{"error": err.Error()}), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{"files": files}), nil
	case "POST /warp-egress/auth-files/assign":
		var body exactAssignRequest
		if err := decodeJSON(req.Body, &body); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		result, err := m.AssignExact(body)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		return jsonResponse(http.StatusOK, result), nil
	case "GET /warp-egress/rules":
		return jsonResponse(http.StatusOK, m.stateStore().Rules()), nil
	case "POST /warp-egress/rules/save":
		var rules Rules
		if err := decodeJSON(req.Body, &rules); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		rules = normalizeRules(rules)
		if err := validateRules(m.stateStore(), rules); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		if err := m.stateStore().SetRules(rules); err != nil {
			return managementResponse{}, err
		}
		return jsonResponse(http.StatusOK, rules), nil
	case "POST /warp-egress/rules/apply":
		result, err := m.ApplyRules()
		if err != nil {
			return jsonResponse(http.StatusBadGateway, map[string]any{"error": err.Error()}), nil
		}
		return jsonResponse(http.StatusOK, result), nil
	case "GET /warp-egress/auto":
		return jsonResponse(http.StatusOK, m.stateStore().AutoSwitch()), nil
	case "POST /warp-egress/auto/save":
		var config AutoSwitchConfig
		if err := decodeJSON(req.Body, &config); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		if err := m.SaveAutoSwitch(config); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		return jsonResponse(http.StatusOK, m.stateStore().AutoSwitch()), nil
	case "POST /warp-egress/auto/run":
		profile, err := m.EvaluateAutoSwitch(true)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{"profile": profile, "auto_switch": m.stateStore().AutoSwitch()}), nil
	case "GET /warp-egress/quality":
		store := m.stateStore()
		profiles := store.Profiles()
		healthy, degraded := 0, 0
		for _, p := range profiles {
			if p.Healthy {
				healthy++
			}
			if p.Degraded {
				degraded++
			}
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"quality": store.Quality(),
			"summary": map[string]int{"total": len(profiles), "healthy": healthy, "degraded": degraded},
		}), nil
	case "POST /warp-egress/quality/save":
		var body QualityConfig
		if err := decodeJSON(req.Body, &body); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		if err := m.stateStore().SetQuality(body); err != nil {
			return managementResponse{}, err
		}
		// 质量守护开关变化后立即同步 XAI 认证文件的自动绑定/解绑。
		go func() {
			m.mu.Lock()
			m.lastAutoBindSync = time.Time{}
			m.mu.Unlock()
			m.syncAutoBoundAuths()
		}()
		return jsonResponse(http.StatusOK, m.stateStore().Quality()), nil
	case "POST /warp-egress/profiles/probe":
		var body profileActionRequest
		if err := decodeJSON(req.Body, &body); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		result, err := m.ProbeProfile(body.ID)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
		}
		return jsonResponse(http.StatusOK, result), nil
	case "POST /warp-egress/quality/prune":
		m.evaluateQualityTasks()
		return jsonResponse(http.StatusOK, m.stateStore().Quality()), nil
	default:
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "route not found", "method": method, "path": path}), nil
	}
}

func (m *Manager) runProfileAction(body profileActionRequest) error {
	if strings.TrimSpace(body.ID) == "" {
		return errors.New("id is required")
	}
	switch strings.ToLower(strings.TrimSpace(body.Action)) {
	case "start":
		return m.StartProfile(body.ID)
	case "stop":
		return m.StopProfile(body.ID)
	case "check":
		return m.CheckProfile(body.ID)
	case "recreate", "rotate":
		return m.RecreateProfile(body.ID)
	default:
		return errors.New("action must be start, stop, check, or recreate")
	}
}

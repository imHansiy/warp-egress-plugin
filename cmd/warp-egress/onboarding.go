package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// onboardingStep describes one setup item shown on the panel guide page.
type onboardingStep struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
	Hint  string `json:"hint"`
}

// onboardingStatus is the full setup-readiness snapshot injected into the
// panel HTML so the guide page renders without any management API call.
type onboardingStatus struct {
	WGCFInstalled      bool             `json:"wgcf_installed"`
	WireproxyInstalled bool             `json:"wireproxy_installed"`
	WGCFPath           string           `json:"wgcf_path"`
	WireproxyPath      string           `json:"wireproxy_path"`
	HasWARPProfiles    bool             `json:"has_warp_profiles"`
	HasGlobalExit      bool             `json:"has_global_exit"`
	ProxyConfigured    bool             `json:"proxy_configured"`
	ProxyURL           string           `json:"proxy_url"`
	ExpectedProxyURL   string           `json:"expected_proxy_url"`
	CPAConfigPath      string           `json:"cpa_config_path"`
	DataDir            string           `json:"data_dir"`
	AllReady           bool             `json:"all_ready"`
	Steps              []onboardingStep `json:"steps"`
}

// candidateCPAConfigPaths lists places where the CPA runtime config.yaml is
// commonly found (HF Space /data mount, process CWD, repo root).
var candidateCPAConfigPaths = []string{
	"/data/config.yaml",
	"./config.yaml",
	"config.yaml",
	"/app/config.yaml",
}

// commandExists reports whether an executable can be found either via PATH or
// as an absolute/relative file path.
func commandExists(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}
	if strings.ContainsAny(cmd, `/\`) {
		if fi, err := os.Stat(cmd); err == nil && !fi.IsDir() {
			return true
		}
		return false
	}
	_, err := exec.LookPath(cmd)
	return err == nil
}

// findCPAConfigPath returns the first existing candidate config path, or "".
func findCPAConfigPath() string {
	for _, candidate := range candidateCPAConfigPaths {
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			abs, absErr := filepath.Abs(candidate)
			if absErr == nil {
				return abs
			}
			return candidate
		}
	}
	return ""
}

// readCPAProxyURL reads proxy-url from the CPA config.yaml found on disk.
func readCPAProxyURL() (string, string) {
	path := findCPAConfigPath()
	if path == "" {
		return "", ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", path
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(trimmed, "proxy-url:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, "proxy-url:"))
		value = strings.Trim(value, `"'`)
		return value, path
	}
	return "", path
}

// renderPanelHTML injects the onboarding status into the panel page.
func renderPanelHTML(m *Manager) string {
	raw, err := json.Marshal(m.OnboardingStatus())
	if err != nil {
		return panelHTML
	}
	return strings.Replace(panelHTML, "/*__ONBOARDING_INJECT__*/", string(raw), 1)
}

// OnboardingStatus builds the setup-readiness snapshot for the guide page.
func (m *Manager) OnboardingStatus() onboardingStatus {
	cfg := m.currentConfig()
	expected := "socks5://" + strings.TrimSpace(cfg.ListenHost) + ":" + strconv.Itoa(cfg.GlobalPort)

	status := onboardingStatus{
		WGCFInstalled:    commandExists(cfg.WGCFPath),
		WireproxyInstalled: commandExists(cfg.WireproxyPath),
		WGCFPath:         cfg.WGCFPath,
		WireproxyPath:    cfg.WireproxyPath,
		ExpectedProxyURL: expected,
		DataDir:          cfg.DataDir,
	}

	store := m.stateStore()
	if store != nil {
		for _, p := range store.Profiles() {
			if p != nil && p.Mode == ProfileModeManaged {
				status.HasWARPProfiles = true
			}
		}
		state := store.Snapshot()
		status.HasGlobalExit = state.Rules.GlobalProfileID != ""
	}

	status.ProxyURL, status.CPAConfigPath = readCPAProxyURL()
	status.ProxyConfigured = strings.TrimSpace(status.ProxyURL) != "" &&
		strings.Contains(status.ProxyURL, strings.TrimPrefix(expected, "socks5://"))

	status.Steps = []onboardingStep{
		{
			Key:   "wgcf",
			Title: "安装 wgcf（WARP 注册工具）",
			Done:  status.WGCFInstalled,
			Hint:  "执行 ./scripts/install-tools.sh，或把 wgcf 二进制放入 PATH；也可在插件配置中指定 wgcf-path。",
		},
		{
			Key:   "wireproxy",
			Title: "安装 wireproxy（WireGuard → SOCKS5）",
			Done:  status.WireproxyInstalled,
			Hint:  "wireproxy 负责把每个 WARP 配置暴露为独立 SOCKS5 端口，安装方式同上。",
		},
		{
			Key:   "profiles",
			Title: "创建至少一个托管 WARP 配置",
			Done:  status.HasWARPProfiles,
			Hint:  "进入「WARP 配置」页，点击「新增配置」注册 WARP 账号（需要 wgcf）。",
		},
		{
			Key:   "global",
			Title: "设置全局出口",
			Done:  status.HasGlobalExit,
			Hint:  "在「WARP 配置」页把某个配置设为全局出口。",
		},
		{
			Key:   "proxy",
			Title: "配置 CPA 代理指向插件中继",
			Done:  status.ProxyConfigured,
			Hint:  "在 CPA config.yaml 中设置 proxy-url: " + expected + "，然后重启 CPA。当前检测到: " +
				func() string {
					if status.ProxyURL == "" {
						return "未配置（未找到 proxy-url）"
					}
					return status.ProxyURL
				}(),
		},
	}

	allReady := true
	for _, step := range status.Steps {
		if !step.Done {
			allReady = false
			break
		}
	}
	status.AllReady = allReady
	return status
}

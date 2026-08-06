package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 系统代理：把当前全局出口应用到系统。系统其他进程（AI 服务等）经
// 本地 HTTP 桥（CONNECT 隧道 → 插件中继 → 当前全局出口）出网。
// 独立开关：设置面板「系统代理」选择应用到系统或不应用。
//
// 平台适配：
//   - Linux 桌面（GNOME）：gsettings 设置 org.gnome.system.proxy（系统设置面板同款）
//   - macOS：networksetup 设置系统偏好网络代理
//   - Windows：注册表 Internet Settings + ProxyEnable
//   - 无桌面（服务器）或无 gsettings/networksetup：回退写入 /etc/profile.d 环境变量

const defaultSystemProxyPort = 40001
const defaultSystemProxyFile = "/etc/profile.d/warp-egress-proxy.sh"

// systemProxy 管理 HTTP 桥与系统环境文件。
type systemProxy struct {
	mu       sync.Mutex
	port     int
	file     string
	server   *http.Server
	listener net.Listener
	selector func() (string, error)
}

func newSystemProxy(port int, file string, selector func() (string, error)) *systemProxy {
	if port <= 0 {
		port = defaultSystemProxyPort
	}
	if strings.TrimSpace(file) == "" {
		file = defaultSystemProxyFile
	}
	return &systemProxy{port: port, file: file, selector: selector}
}

// Start 启动 HTTP 桥监听。
func (p *systemProxy) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.listener != nil {
		return nil
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.port)))
	if err != nil {
		return fmt.Errorf("系统代理桥监听 %d 失败: %w", p.port, err)
	}
	p.listener = listener
	p.server = &http.Server{Handler: p}
	go func() {
		_ = p.server.Serve(listener)
	}()
	return nil
}

// Stop 停止 HTTP 桥。
func (p *systemProxy) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.server != nil {
		_ = p.server.Close()
		p.server = nil
	}
	if p.listener != nil {
		_ = p.listener.Close()
		p.listener = nil
	}
}

// Running 桥是否在运行。
func (p *systemProxy) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.listener != nil
}

// ServeHTTP 处理 HTTP 正向代理请求（CONNECT 隧道）。
func (p *systemProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	target := ensureHostPort(r.Host)
	proxyURL, err := p.selector()
	var upstream net.Conn
	if err != nil {
		// 无已选出口：直连（与中继回退一致）。
		upstream, err = (&net.Dialer{Timeout: 20 * time.Second}).DialContext(r.Context(), "tcp", target)
	} else {
		proxyAddr, addrErr := socksProxyAddress(proxyURL)
		if addrErr != nil {
			upstream, err = nil, addrErr
		} else {
			upstream, err = dialSOCKS5(r.Context(), proxyAddr, target)
		}
	}
	if err != nil {
		http.Error(w, "dial failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	client, brw, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	if _, err := client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		client.Close()
		upstream.Close()
		return
	}
	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			client.Close()
			upstream.Close()
		})
	}
	go func() {
		_, _ = io.Copy(upstream, brw)
		if cw, ok := upstream.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		closeBoth()
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		if cw, ok := client.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		closeBoth()
	}()
}

// ApplySystemProxy 开启/关闭系统代理：
//   - 开启：启动 HTTP 桥并写入系统环境文件；
//   - 关闭：停止桥并删除环境文件。
// file 用于测试注入（真实场景为 /etc/profile.d/...）。
func (m *Manager) ApplySystemProxy(cfg SystemProxyConfig, fileOverride string) error {
	selector := func() (string, error) { return m.selectedProfileProxy() }
	if fileOverride == "" {
		fileOverride = cfg.File
	}
	if fileOverride == "" {
		fileOverride = defaultSystemProxyFile
	}
	cfg.File = fileOverride
	m.mu.Lock()
	if m.systemProxyInstance == nil {
		m.systemProxyInstance = newSystemProxy(cfg.Port, fileOverride, selector)
	} else {
		m.systemProxyInstance.port = cfg.Port
		m.systemProxyInstance.file = fileOverride
	}
	instance := m.systemProxyInstance
	m.mu.Unlock()

	if cfg.Enabled {
		if err := instance.Start(); err != nil {
			return err
		}
		if err := applySystemProxySettings(true, cfg.Port, instance.file); err != nil {
			instance.Stop()
			return err
		}
		return nil
	}
	instance.Stop()
	return applySystemProxySettings(false, cfg.Port, instance.file)
}

// applySystemProxySettings 按平台把代理应用到系统：
//   - darwin：networksetup（系统偏好网络代理）
//   - windows：注册表 Internet Settings
//   - 其他（linux）：优先 GNOME gsettings（系统设置面板），
//     无桌面/无 gsettings 时回退写入环境变量文件。
func applySystemProxySettings(enabled bool, port int, fallbackFile string) error {
	proxyURL := "http://127.0.0.1:" + strconv.Itoa(port)
	switch runtime.GOOS {
	case "darwin":
		if err := applyDarwinSystemProxy(enabled, proxyURL); err != nil {
			return err
		}
		return nil
	case "windows":
		return applyWindowsSystemProxy(enabled, port)
	default:
		if err := applyGNOMESystemProxy(enabled, port); err == nil {
			return nil
		}
		// 无桌面/无 gsettings：回退环境变量文件。
		if enabled {
			return writeSystemProxyEnv(fallbackFile, port)
		}
		return removeSystemProxyEnv(fallbackFile)
	}
}

// applyGNOMESystemProxy 通过 gsettings 设置 GNOME 系统代理
// （与「设置 → 网络 → 网络代理」面板同款，即时生效）。
func applyGNOMESystemProxy(enabled bool, port int) error {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return errors.New("gsettings 不可用（非 GNOME 桌面）")
	}
	gs := func(args ...string) error {
		cmd := exec.Command("gsettings", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("gsettings %s 失败: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
		}
		return nil
	}
	portStr := strconv.Itoa(port)
	if enabled {
		if err := gs("set", "org.gnome.system.proxy", "mode", "manual"); err != nil {
			return err
		}
		for _, schema := range []string{"http", "https"} {
			if err := gs("set", "org.gnome.system.proxy."+schema, "host", "127.0.0.1"); err != nil {
				return err
			}
			if err := gs("set", "org.gnome.system.proxy."+schema, "port", portStr); err != nil {
				return err
			}
		}
		return nil
	}
	return gs("set", "org.gnome.system.proxy", "mode", "none")
}

// applyDarwinSystemProxy 通过 networksetup 设置 macOS 系统网络代理
// （所有启用的网络服务，HTTP + HTTPS；与系统偏好设置同款）。
func applyDarwinSystemProxy(enabled bool, proxyURL string) error {
	if _, err := exec.LookPath("networksetup"); err != nil {
		return errors.New("networksetup 不可用")
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return err
	}
	host := parsed.Hostname()
	port := parsed.Port()
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return fmt.Errorf("networksetup 列表服务失败: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		service := strings.TrimSpace(line)
		if service == "" || strings.HasPrefix(service, "*") || strings.HasPrefix(service, "An asterisk") {
			continue
		}
		if enabled {
			_ = exec.Command("networksetup", "-setwebproxy", service, host, port).Run()
			_ = exec.Command("networksetup", "-setsecurewebproxy", service, host, port).Run()
		} else {
			_ = exec.Command("networksetup", "-setwebproxystate", service, "off").Run()
			_ = exec.Command("networksetup", "-setsecurewebproxystate", service, "off").Run()
		}
	}
	return nil
}

// applyWindowsSystemProxy 通过注册表设置 Windows 系统代理
// （HKCU Internet Settings，与「设置 → 网络 → 代理」同款）。
func applyWindowsSystemProxy(enabled bool, port int) error {
	regPath := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	if enabled {
		cmds := [][]string{
			{"add", regPath, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f"},
			{"add", regPath, "/v", "ProxyServer", "/t", "REG_SZ", "/d", "127.0.0.1:" + strconv.Itoa(port), "/f"},
			{"add", regPath, "/v", "ProxyOverride", "/t", "REG_SZ", "/d", "<local>", "/f"},
		}
		for _, args := range cmds {
			if out, err := exec.Command("reg", args...).CombinedOutput(); err != nil {
				return fmt.Errorf("reg %s 失败: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
			}
		}
		return nil
	}
	if out, err := exec.Command("reg", "add", regPath, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("reg 关闭代理失败: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// writeSystemProxyEnv 写入系统环境文件（http_proxy/https_proxy/all_proxy/no_proxy）。
func writeSystemProxyEnv(file string, port int) error {
	proxyURL := "http://127.0.0.1:" + strconv.Itoa(port)
	content := "# Managed by warp-egress-plugin - 系统代理（当前全局出口）\n" +
		"export http_proxy=\"" + proxyURL + "\"\n" +
		"export https_proxy=\"" + proxyURL + "\"\n" +
		"export all_proxy=\"" + proxyURL + "\"\n" +
		"export no_proxy=\"localhost,127.0.0.1,::1,10.*,172.16.*,172.17.*,172.18.*,172.19.*,172.20.*,172.21.*,172.22.*,172.23.*,172.24.*,172.25.*,172.26.*,172.27.*,172.28.*,172.29.*,172.30.*,172.31.*,192.168.*\"\n" +
		"export NO_PROXY=\"localhost,127.0.0.1,::1,10.*,172.16.*,172.17.*,172.18.*,172.19.*,172.20.*,172.21.*,172.22.*,172.23.*,172.24.*,172.25.*,172.26.*,172.27.*,172.28.*,172.29.*,172.30.*,172.31.*,192.168.*\"\n"
	if err := os.MkdirAll(dirOf(file), 0o755); err != nil {
		return fmt.Errorf("创建系统代理目录失败: %w", err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入系统代理环境文件失败（需要 root 权限）: %w", err)
	}
	return nil
}

// removeSystemProxyEnv 删除系统环境文件。
func removeSystemProxyEnv(file string) error {
	err := os.Remove(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("删除系统代理环境文件失败: %w", err)
	}
	return nil
}

func dirOf(path string) string {
	idx := strings.LastIndexByte(path, '/')
	if idx <= 0 {
		return "/"
	}
	return path[:idx]
}


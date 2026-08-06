package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 系统代理：把当前全局出口应用到系统。系统其他进程（AI 服务等）经
// HTTP 环境变量（http_proxy/https_proxy/all_proxy）把流量送到本地 HTTP 桥，
// 桥通过插件中继的 selector 选择当前全局出口（无已选出口时直连）。
// 独立开关：设置面板「系统代理」选择应用到系统或不应用。

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
		if err := writeSystemProxyEnv(instance.file, cfg.Port); err != nil {
			instance.Stop()
			return err
		}
		return nil
	}
	instance.Stop()
	return removeSystemProxyEnv(instance.file)
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

var _ = context.Background
var _ = time.Now

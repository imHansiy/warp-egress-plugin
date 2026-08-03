package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type managedProcess struct {
	cmd     *exec.Cmd
	logFile *os.File
	mu      sync.Mutex
}

func normalizeSOCKSURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("proxy_url is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid proxy_url: %w", err)
	}
	if parsed.User != nil {
		return "", errors.New("SOCKS5 username/password authentication is not supported")
	}
	if parsed.Scheme != "socks5" && parsed.Scheme != "socks5h" {
		return "", errors.New("only socks5:// or socks5h:// is supported")
	}
	if parsed.Hostname() == "" || parsed.Port() == "" {
		return "", errors.New("proxy_url must include host and port")
	}
	if _, err := strconv.Atoi(parsed.Port()); err != nil {
		return "", errors.New("proxy_url port is invalid")
	}
	return parsed.String(), nil
}

func (m *Manager) RegisterManagedProfile(profile *Profile) error {
	if profile == nil || profile.Mode != ProfileModeManaged {
		return errors.New("managed profile is required")
	}
	cfg := m.currentConfig()
	wgPath := filepath.Join(profile.Directory, "wgcf-profile.conf")
	if _, err := os.Stat(wgPath); err == nil {
		// A profile was imported (or previously registered): skip wgcf
		// register/generate and reuse the existing WireGuard config.
		return m.writeWireproxyConfig(profile, wgPath)
	}
	if _, err := exec.LookPath(cfg.WGCFPath); err != nil {
		return fmt.Errorf("wgcf not found: %w", err)
	}
	if _, err := exec.LookPath(cfg.WireproxyPath); err != nil {
		return fmt.Errorf("wireproxy not found: %w", err)
	}
	if err := os.MkdirAll(profile.Directory, 0o700); err != nil {
		return err
	}
	register := exec.Command(cfg.WGCFPath, "register", "--accept-tos")
	register.Dir = profile.Directory
	register.Env = append(os.Environ(), "HOME="+profile.Directory)
	output, err := register.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wgcf register failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	generate := exec.Command(cfg.WGCFPath, "generate")
	generate.Dir = profile.Directory
	generate.Env = append(os.Environ(), "HOME="+profile.Directory)
	output, err = generate.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wgcf generate failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	if _, err := os.Stat(wgPath); err != nil {
		return fmt.Errorf("wgcf profile missing: %w", err)
	}
	return m.writeWireproxyConfig(profile, wgPath)
}

func (m *Manager) writeWireproxyConfig(profile *Profile, wgPath string) error {
	if err := os.MkdirAll(profile.Directory, 0o700); err != nil {
		return err
	}
	wireproxyConfig := fmt.Sprintf("WGConfig = %s\n\n[Socks5]\nBindAddress = %s\n", wgPath, net.JoinHostPort(profile.ListenHost, strconv.Itoa(profile.ListenPort)))
	configPath := filepath.Join(profile.Directory, "wireproxy.conf")
	if err := os.WriteFile(configPath, []byte(wireproxyConfig), 0o600); err != nil {
		return err
	}
	return nil
}

func (m *Manager) StartProfile(id string) error {
	profile := m.stateStore().Profile(id)
	if profile == nil {
		return errors.New("profile not found")
	}
	if profile.Mode == ProfileModeExternal {
		profile.Running = true
		profile.UpdatedAt = time.Now()
		return m.stateStore().UpdateProfile(profile)
	}
	m.mu.Lock()
	if running := m.processes[id]; running != nil && running.cmd != nil && running.cmd.Process != nil {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	cfg := m.currentConfig()
	configPath := filepath.Join(profile.Directory, "wireproxy.conf")
	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("wireproxy config missing: %w", err)
	}
	logPath := filepath.Join(profile.Directory, "wireproxy.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(cfg.WireproxyPath, "-c", configPath, "-s")
	cmd.Dir = profile.Directory
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start wireproxy: %w", err)
	}
	process := &managedProcess{cmd: cmd, logFile: logFile}
	store := m.stateStore()
	m.mu.Lock()
	m.processes[id] = process
	m.mu.Unlock()
	profile.PID = cmd.Process.Pid
	profile.Running = true
	profile.LastError = ""
	profile.UpdatedAt = time.Now()
	if err := m.stateStore().UpdateProfile(profile); err != nil {
		return err
	}
	go func() {
		errWait := cmd.Wait()
		process.mu.Lock()
		_ = logFile.Close()
		process.mu.Unlock()
		m.mu.Lock()
		delete(m.processes, id)
		m.mu.Unlock()
		latest := store.Profile(id)
		if latest != nil {
			latest.Running = false
			latest.PID = 0
			latest.Healthy = false
			latest.UpdatedAt = time.Now()
			if errWait != nil {
				latest.LastError = errWait.Error()
			}
			_ = store.UpdateProfile(latest)
		}
	}()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(profile.ListenHost, strconv.Itoa(profile.ListenPort)), 300*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return errors.New("wireproxy did not open its SOCKS5 port")
}

func (m *Manager) StopProfile(id string) error {
	profile := m.stateStore().Profile(id)
	if profile == nil {
		return errors.New("profile not found")
	}
	m.mu.Lock()
	process := m.processes[id]
	m.mu.Unlock()
	if process != nil && process.cmd != nil && process.cmd.Process != nil {
		_ = process.cmd.Process.Kill()
	}
	profile.Running = false
	profile.Healthy = false
	profile.PID = 0
	profile.UpdatedAt = time.Now()
	return m.stateStore().UpdateProfile(profile)
}

func (m *Manager) RecreateProfile(id string) error {
	profile := m.stateStore().Profile(id)
	if profile == nil {
		return errors.New("profile not found")
	}
	if profile.Mode != ProfileModeManaged {
		return errors.New("only managed profiles can be recreated")
	}
	_ = m.StopProfile(id)
	for _, name := range []string{"wgcf-account.toml", "wgcf-profile.conf", "wireproxy.conf"} {
		_ = os.Remove(filepath.Join(profile.Directory, name))
	}
	if err := m.RegisterManagedProfile(profile); err != nil {
		return err
	}
	if err := m.StartProfile(id); err != nil {
		return err
	}
	return m.CheckProfile(id)
}

func (m *Manager) CheckAllProfiles() error {
	profiles := m.stateStore().Profiles()
	var firstErr error
	for _, profile := range profiles {
		if err := m.CheckProfile(profile.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *Manager) CheckProfile(id string) error {
	profile := m.stateStore().Profile(id)
	if profile == nil {
		return errors.New("profile not found")
	}
	cfg := m.currentConfig()
	start := time.Now()
	trace, err := fetchTraceViaSOCKS(profile.ProxyURL, cfg.IPCheckURL, 12*time.Second)
	profile.LastChecked = time.Now()
	profile.LatencyMS = time.Since(start).Milliseconds()
	profile.UpdatedAt = time.Now()
	if err != nil {
		profile.Healthy = false
		profile.LastError = err.Error()
		if profile.Mode == ProfileModeExternal {
			profile.Running = false
		}
		_ = m.stateStore().UpdateProfile(profile)
		return err
	}
	values := parseTrace(trace)
	profile.ExitIP = values["ip"]
	profile.ExitIPV4 = ""
	profile.ExitIPV6 = ""
	if parsedIP := net.ParseIP(profile.ExitIP); parsedIP != nil {
		if parsedIP.To4() != nil {
			profile.ExitIPV4 = profile.ExitIP
		} else {
			profile.ExitIPV6 = profile.ExitIP
		}
	}
	// Complement the other address family through the same WARP exit. These
	// lookups are best-effort: failures never affect health status.
	if profile.ExitIPV4 == "" {
		if body, err := fetchTraceViaSOCKS(profile.ProxyURL, "https://api.ipify.org", 8*time.Second); err == nil {
			if v := strings.TrimSpace(string(body)); v != "" && net.ParseIP(v) != nil {
				profile.ExitIPV4 = v
			}
		}
	}
	if profile.ExitIPV6 == "" {
		if body, err := fetchTraceViaSOCKS(profile.ProxyURL, "https://api6.ipify.org", 8*time.Second); err == nil {
			if v := strings.TrimSpace(string(body)); v != "" && net.ParseIP(v) != nil {
				profile.ExitIPV6 = v
			}
		}
	}
	profile.Colo = values["colo"]
	profile.WarpMode = values["warp"]
	profile.Healthy = profile.ExitIP != "" && (profile.WarpMode == "on" || profile.WarpMode == "plus")
	profile.Running = true
	if !profile.Healthy {
		profile.LastError = "trace did not confirm WARP (warp=" + profile.WarpMode + ")"
	} else {
		profile.LastError = ""
	}
	if err := m.stateStore().UpdateProfile(profile); err != nil {
		return err
	}
	if !profile.Healthy {
		return errors.New(profile.LastError)
	}
	return nil
}

func fetchTraceViaSOCKS(proxyURL, targetURL string, timeout time.Duration) ([]byte, error) {
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	proxyAddr := parsed.Host
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialSOCKS5(ctx, proxyAddr, address)
	}, ForceAttemptHTTP2: true}
	client := &http.Client{Transport: transport, Timeout: timeout}
	request, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("user-agent", "warp-egress-plugin/"+pluginVersion)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("trace endpoint returned %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, 64*1024))
}

func parseTrace(raw []byte) map[string]string {
	out := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		line := scanner.Text()
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		out[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
	}
	return out
}

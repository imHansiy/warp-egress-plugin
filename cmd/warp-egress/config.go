package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DataDir             string
	WGCFPath            string
	WireproxyPath       string
	WGCFPathSet         bool // 配置中显式指定了 wgcf-path
	WireproxyPathSet    bool // 配置中显式指定了 wireproxy-path
	ListenHost          string
	GlobalPort          int
	ProfilePortStart    int
	ProfilePortEnd      int
	AutoStart           bool
	HealthCheckInterval time.Duration
	IPCheckURL          string
	AllowRemoteListen   bool
}

func defaultConfig() Config {
	return Config{
		DataDir:             "./warp-egress-data",
		WGCFPath:            "wgcf",
		WireproxyPath:       "wireproxy",
		ListenHost:          "127.0.0.1",
		GlobalPort:          40000,
		ProfilePortStart:    41000,
		ProfilePortEnd:      41999,
		AutoStart:           true,
		HealthCheckInterval: 60 * time.Second,
		IPCheckURL:          "https://www.cloudflare.com/cdn-cgi/trace",
		AllowRemoteListen:   false,
	}
}

func parseConfig(raw []byte) (Config, error) {
	cfg := defaultConfig()
	values := parseFlatYAML(raw)
	if value := values["data-dir"]; value != "" {
		cfg.DataDir = value
	}
	if value := values["wgcf-path"]; value != "" {
		cfg.WGCFPath = value
		cfg.WGCFPathSet = true
	}
	if value := values["wireproxy-path"]; value != "" {
		cfg.WireproxyPath = value
		cfg.WireproxyPathSet = true
	}
	if value := values["listen-host"]; value != "" {
		cfg.ListenHost = value
	}
	if value := values["global-port"]; value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return cfg, fmt.Errorf("global-port: %w", err)
		}
		cfg.GlobalPort = n
	}
	if value := values["profile-port-start"]; value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return cfg, fmt.Errorf("profile-port-start: %w", err)
		}
		cfg.ProfilePortStart = n
	}
	if value := values["profile-port-end"]; value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return cfg, fmt.Errorf("profile-port-end: %w", err)
		}
		cfg.ProfilePortEnd = n
	}
	if value := values["auto-start"]; value != "" {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return cfg, fmt.Errorf("auto-start: %w", err)
		}
		cfg.AutoStart = b
	}
	if value := values["health-check-interval"]; value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			cfg.HealthCheckInterval = time.Duration(n) * time.Second
		} else {
			d, errDuration := time.ParseDuration(value)
			if errDuration != nil {
				return cfg, fmt.Errorf("health-check-interval: %w", errDuration)
			}
			cfg.HealthCheckInterval = d
		}
	}
	if value := values["ip-check-url"]; value != "" {
		cfg.IPCheckURL = value
	}
	if value := values["allow-remote-listen"]; value != "" {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return cfg, fmt.Errorf("allow-remote-listen: %w", err)
		}
		cfg.AllowRemoteListen = b
	}
	if cfg.GlobalPort < 1 || cfg.GlobalPort > 65535 {
		return cfg, fmt.Errorf("global-port out of range")
	}
	if cfg.ProfilePortStart < 1 || cfg.ProfilePortEnd > 65535 || cfg.ProfilePortStart > cfg.ProfilePortEnd {
		return cfg, fmt.Errorf("invalid profile port range")
	}
	if !cfg.AllowRemoteListen && !isLoopbackHost(cfg.ListenHost) {
		return cfg, fmt.Errorf("listen-host must be loopback unless allow-remote-listen=true")
	}
	abs, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return cfg, fmt.Errorf("resolve data-dir: %w", err)
	}
	cfg.DataDir = abs
	return cfg, nil
}

func parseFlatYAML(raw []byte) map[string]string {
	out := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if comment := strings.Index(value, " #"); comment >= 0 {
			value = strings.TrimSpace(value[:comment])
		}
		value = strings.Trim(value, "\"'")
		out[key] = value
	}
	return out
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func ensureDataDir(cfg Config) error {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	return os.Chmod(cfg.DataDir, 0o700)
}

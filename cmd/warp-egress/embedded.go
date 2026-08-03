package main

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// embeddedToolsFS 内嵌 wgcf/wireproxy 官方预编译二进制。
// 二进制由 scripts/download-tools.sh 在构建前下载到 embedded_tools/（不入库），
// 插件首次启动时解压到 <data-dir>/bin/，实现服务器零安装。
//
//go:embed embedded_tools/wgcf
//go:embed embedded_tools/wireproxy
var embeddedToolsFS embed.FS

// bundledTool 描述一个内嵌工具的嵌入路径与解压后的文件名。
type bundledTool struct {
	EmbedPath string
	FileName  string
}

// bundledToolList 是插件内置的两个外部工具，顺序即依赖声明顺序。
var bundledToolList = []bundledTool{
	{EmbedPath: "embedded_tools/wgcf", FileName: "wgcf"},
	{EmbedPath: "embedded_tools/wireproxy", FileName: "wireproxy"},
}

// ensureBundledTools 保证 wgcf/wireproxy 可用：
//   - 配置的路径（或 PATH 中的同名命令）实际可用 → 沿用，不干预；
//   - 配置的路径不可用（显式配置但文件不存在，或默认名不在 PATH）→
//     把内嵌二进制解压到 <data-dir>/bin/，并把配置路径指向解压产物，实现服务器零安装。
//
// 返回更新后的 Config（仅当发生解压时才改写路径）。
func ensureBundledTools(cfg Config) (Config, error) {
	needWGCF := !commandExists(cfg.WGCFPath)
	needWireproxy := !commandExists(cfg.WireproxyPath)
	if !needWGCF && !needWireproxy {
		return cfg, nil
	}

	binDir := filepath.Join(cfg.DataDir, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return cfg, fmt.Errorf("create bundled tools dir: %w", err)
	}
	for _, tool := range bundledToolList {
		if (tool.FileName == "wgcf" && !needWGCF) || (tool.FileName == "wireproxy" && !needWireproxy) {
			continue
		}
		path, err := extractBundledTool(binDir, tool)
		if err != nil {
			return cfg, fmt.Errorf("extract bundled %s: %w", tool.FileName, err)
		}
		if tool.FileName == "wgcf" {
			cfg.WGCFPath = path
		} else {
			cfg.WireproxyPath = path
		}
	}
	return cfg, nil
}

// extractBundledTool 把内嵌二进制写入 binDir，返回解压后的绝对路径。
// 目标文件已存在且内容一致时直接复用，避免每次启动重写磁盘。
func extractBundledTool(binDir string, tool bundledTool) (string, error) {
	data, err := embeddedToolsFS.ReadFile(tool.EmbedPath)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(binDir, tool.FileName)
	if existing, err := os.ReadFile(dst); err == nil && bytes.Equal(existing, data) {
		return dst, nil
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return "", err
	}
	if err := os.Chmod(dst, 0o755); err != nil {
		return "", err
	}
	return dst, nil
}

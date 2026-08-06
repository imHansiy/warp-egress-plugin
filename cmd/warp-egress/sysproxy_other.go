//go:build !windows

package main

// refreshWindowsProxySettings 仅 Windows 使用；其他平台为空实现。
func refreshWindowsProxySettings() {}

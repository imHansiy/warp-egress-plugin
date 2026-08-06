//go:build windows

package main

import "syscall"

// refreshWindowsProxySettings 广播代理设置变更，让系统及已运行进程
// （WinINet 应用、浏览器等）立即感知新的代理配置。
func refreshWindowsProxySettings() {
	wininet := syscall.NewLazyDLL("wininet.dll")
	internetSetOption := wininet.NewProc("InternetSetOptionW")
	const (
		internetOptionSettingsChanged = 39
		internetOptionRefresh         = 37
	)
	_, _, _ = internetSetOption.Call(0, internetOptionSettingsChanged, 0, 0)
	_, _, _ = internetSetOption.Call(0, internetOptionRefresh, 0, 0)
}

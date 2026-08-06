package main

import (
		"net"
	"net/http"
	"io"
	"crypto/tls"
	"strconv"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApplySystemProxyFallsBackToEnvFile(t *testing.T) {
	// 非桌面/无 gsettings 环境：开启应写入环境文件，关闭应删除。
	dir := t.TempDir()
	file := filepath.Join(dir, "proxy.sh")
	if err := applySystemProxySettings(true, 40001, file); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("fallback env file must be written")
	}
	if err := applySystemProxySettings(false, 40001, file); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("fallback env file must be removed on disable")
	}
}

func TestWriteAndRemoveSystemProxyEnv(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "warp-egress-proxy.sh")
	if err := writeSystemProxyEnv(file, 40001); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{
		`http_proxy="http://127.0.0.1:40001"`,
		`https_proxy="http://127.0.0.1:40001"`,
		`all_proxy="http://127.0.0.1:40001"`,
		"no_proxy=",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("env file missing %q: %s", want, text)
		}
	}
	if err := removeSystemProxyEnv(file); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("env file must be removed")
	}
}

func TestSystemProxyBridge(t *testing.T) {
	// 目标服务器（TLS：经代理走 CONNECT 隧道）
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("sys-proxy-ok"))
	}))
	defer target.Close()

	// 上游 SOCKS5 服务器（作为当前全局出口）
	upstreamListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstreamListener.Close()
	go func() {
		for {
			client, err := upstreamListener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer client.Close()
				dest, err := acceptSOCKS5(client)
				if err != nil {
					return
				}
				remote, err := net.DialTimeout("tcp", dest, 5*time.Second)
				if err != nil {
					_ = writeSOCKSReply(client, 0x05, nil)
					return
				}
				defer remote.Close()
				_ = writeSOCKSReply(client, 0, remote.LocalAddr())
				copyStream(client, remote)
			}()
		}
	}()

	// 系统代理桥：selector 返回该 SOCKS5 出口（独立随机端口）
	manager := newTestManager(t)
	manager.cfg = defaultConfig()
	bridgePort := freeTCPPort(t)
	sysProxy := newSystemProxy(bridgePort, filepath.Join(t.TempDir(), "proxy.sh"), func() (string, error) {
		return "socks5://" + upstreamListener.Addr().String(), nil
	})
	if err := sysProxy.Start(); err != nil {
		t.Fatal(err)
	}
	defer sysProxy.Stop()
	if !sysProxy.Running() {
		t.Fatal("bridge must be running")
	}

	// 经 HTTP 桥访问目标
	proxyAddr := "127.0.0.1:" + strconv.Itoa(bridgePort)
	transport := &http.Transport{Proxy: http.ProxyURL(mustURL("http://" + proxyAddr)), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := &http.Client{Transport: transport}
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "sys-proxy-ok") {
		t.Fatalf("unexpected response: %s", body)
	}

	// 端口占用时启动失败提示
	occupied, err := net.Listen("tcp", "127.0.0.1:40001")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	sysProxy2 := newSystemProxy(40001, filepath.Join(t.TempDir(), "p.sh"), func() (string, error) { return "", nil })
	if err := sysProxy2.Start(); err == nil {
		t.Fatal("bridge must fail when port is occupied")
	}
	_ = sysProxy2
}

func mustURL(raw string) *url.URL {
	u, _ := url.Parse(raw)
	return u
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

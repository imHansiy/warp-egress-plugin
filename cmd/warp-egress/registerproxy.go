package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

// registerProxyLog 输出注册代理的调试信息（写入 CLIProxyAPI 进程的 stderr）。
var registerProxyLog = os.Stderr

// registerProxyAddr 供 wgcf register 使用的本地 HTTP CONNECT 代理地址（如 127.0.0.1:port）。
type registerProxy struct {
	addr string
	done chan struct{}
	once sync.Once
}

// socks5Dial 通过无认证的 SOCKS5 代理（RFC 1928）连接目标 host:port。
func socks5Dial(proxyAddr, target string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		conn.Close()
		return nil, err
	}
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		conn.Close()
		return nil, err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		conn.Close()
		return nil, err
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		conn.Close()
		return nil, errors.New("socks5 proxy rejected no-auth method")
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		conn.Close()
		return nil, err
	}
	portInt, err := parsePort(port)
	if err != nil {
		conn.Close()
		return nil, err
	}
	req := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			req = append(req, 0x01)
			req = append(req, v4...)
		} else {
			req = append(req, 0x04)
			req = append(req, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			conn.Close()
			return nil, errors.New("target host too long")
		}
		req = append(req, 0x03, byte(len(host)))
		req = append(req, host...)
	}
	req = append(req, byte(portInt>>8), byte(portInt))
	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		conn.Close()
		return nil, err
	}
	if head[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 connect failed: reply=%d", head[1])
	}
	var addrLen int
	switch head[3] {
	case 0x01:
		addrLen = 4
	case 0x04:
		addrLen = 16
	case 0x03:
		if _, err := io.ReadFull(conn, head[:1]); err != nil {
			conn.Close()
			return nil, err
		}
		addrLen = int(head[0])
	default:
		conn.Close()
		return nil, errors.New("socks5 invalid bind address type")
	}
	rest := make([]byte, addrLen+2)
	if _, err := io.ReadFull(conn, rest); err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func parsePort(port string) (int, error) {
	var n int
	for _, c := range port {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid port")
		}
		n = n*10 + int(c-'0')
		if n > 65535 {
			return 0, errors.New("invalid port")
		}
	}
	if n == 0 {
		return 0, errors.New("invalid port")
	}
	return n, nil
}

// startRegisterProxy 启动本地 HTTP CONNECT 代理，把 CONNECT 隧道转发到 socks5Addr（socks5://host:port 或 host:port）。
func startRegisterProxy(socks5Addr string) (*registerProxy, error) {
	socks5Host := socks5Addr
	if parsed, err := url.Parse(socks5Addr); err == nil && (parsed.Scheme == "socks5" || parsed.Scheme == "socks5h") {
		if parsed.Hostname() == "" || parsed.Port() == "" {
			return nil, fmt.Errorf("invalid socks5 addr %q: missing host or port", socks5Addr)
		}
		socks5Host = parsed.Host
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	proxy := &registerProxy{addr: listener.Addr().String(), done: make(chan struct{})}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		target := ensureHostPort(r.Host)
		upstream, err := socks5Dial(socks5Host, target, 30*time.Second)
		if err != nil {
			fmt.Fprintf(registerProxyLog, "register proxy: socks5 dial %q via %s failed: %v\n", target, socks5Host, err)
			http.Error(w, "socks5 dial failed: "+err.Error(), http.StatusBadGateway)
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
	})
	server := &http.Server{Handler: handler}
	go func() {
		<-proxy.done
		_ = server.Close()
	}()
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = listener.Close()
		}
	}()
	return proxy, nil
}

// URL 返回 HTTP 代理地址，供 HTTPS_PROXY 环境变量使用。
func (p *registerProxy) URL() string {
	return "http://" + p.addr
}

// Close 停止代理。
func (p *registerProxy) Close() {
	p.once.Do(func() { close(p.done) })
}

// ensureHostPort 补全缺省端口，供 CONNECT 目标使用。
func ensureHostPort(target string) string {
	if _, _, err := net.SplitHostPort(target); err == nil {
		return target
	}
	return target + ":443"
}

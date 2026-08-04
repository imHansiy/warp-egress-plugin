package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// startFakeSocks5 启动一个只验证握手和目标地址的无认证 SOCKS5 服务器。
func startFakeSocks5(t *testing.T) (addr string, gotTarget chan string, data chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gotTarget = make(chan string, 1)
	data = make(chan string, 1)
	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 3)
		if _, err := io.ReadFull(conn, buf); err != nil {
			t.Error("read greeting:", err)
			return
		}
		if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
			return
		}
		head := make([]byte, 4)
		if _, err := io.ReadFull(conn, head); err != nil {
			return
		}
		var target string
		switch head[3] {
		case 0x03:
			lb := make([]byte, 1)
			if _, err := io.ReadFull(conn, lb); err != nil {
				return
			}
			hb := make([]byte, lb[0])
			if _, err := io.ReadFull(conn, hb); err != nil {
				return
			}
			rest := make([]byte, 2)
			if _, err := io.ReadFull(conn, rest); err != nil {
				return
			}
			target = fmt.Sprintf("%s:%d", string(hb), uint16(rest[0])<<8|uint16(rest[1]))
		case 0x01:
			rest := make([]byte, 6)
			if _, err := io.ReadFull(conn, rest); err != nil {
				return
			}
			target = fmt.Sprintf("%d.%d.%d.%d:%d", rest[0], rest[1], rest[2], rest[3], uint16(rest[4])<<8|uint16(rest[5]))
		default:
			t.Error("unexpected atyp", head[3])
			return
		}
		gotTarget <- target
		if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
			return
		}
		reader := bufio.NewReader(conn)
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		data <- line
	}()
	return ln.Addr().String(), gotTarget, data
}

func TestSocks5DialAndConnectProxy(t *testing.T) {
	socksAddr, gotTarget, data := startFakeSocks5(t)
	proxy, err := startRegisterProxy(socksAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	conn, err := net.DialTimeout("tcp", proxy.addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	req := "CONNECT api.cloudflareclient.com:443 HTTP/1.1\r\nHost: api.cloudflareclient.com:443\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if len(status) < 12 || status[9:12] != "200" {
		t.Fatalf("unexpected CONNECT status: %q", status)
	}
	// 隧道已建立：读取 socks 服务器回写的测试负载，同时向桥写入数据。
	if _, err := conn.Write([]byte("hello-through-proxy\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case target := <-gotTarget:
		if target != "api.cloudflareclient.com:443" {
			t.Fatalf("socks5 got target %q, want api.cloudflareclient.com:443", target)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("socks5 server never received a target")
	}
	select {
	case line := <-data:
		if line != "hello-through-proxy\n" {
			t.Fatalf("relay payload mismatch: %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("relay payload not received")
	}
}

func TestResolveRegisterProxy(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{store: NewStateStore(dir), cfg: Config{DataDir: dir}}
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"socks5://127.0.0.1:1080", "socks5://127.0.0.1:1080", false},
		{"socks5h://example.com:1080", "socks5h://example.com:1080", false},
		{"http://user:pass@proxy.example.com:3128", "http://user:pass@proxy.example.com:3128", false},
		{"https://proxy.example.com:8443", "https://proxy.example.com:8443", false},
		{"http://no-port", "", true},
		{"warp-xxxxxxxxxxxx", "", true},
	}
	for _, c := range cases {
		got, err := m.resolveRegisterProxy(c.in)
		if c.wantErr {
			if err == nil {
				t.Fatalf("resolveRegisterProxy(%q): want error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("resolveRegisterProxy(%q): unexpected error %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("resolveRegisterProxy(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSocks5AuthHandshake(t *testing.T) {
	// 模拟需要用户名/密码认证的 SOCKS5 服务器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	gotUser := make(chan string, 1)
	gotPass := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4)
		io.ReadFull(conn, buf)         // 0x05 0x02 0x00 0x02
		conn.Write([]byte{0x05, 0x02}) // 要求认证
		h := make([]byte, 1)
		io.ReadFull(conn, h) // 0x01
		lb := make([]byte, 1)
		io.ReadFull(conn, lb)
		u := make([]byte, lb[0])
		io.ReadFull(conn, u)
		gotUser <- string(u)
		io.ReadFull(conn, lb)
		p := make([]byte, lb[0])
		io.ReadFull(conn, p)
		gotPass <- string(p)
		conn.Write([]byte{0x01, 0x00})
		// 读连接请求后返回成功
		head := make([]byte, 4)
		io.ReadFull(conn, head)
		conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	}()
	conn, err := socks5Dial("socks5://alice:s3cret@"+ln.Addr().String(), "example.com:443", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if u := <-gotUser; u != "alice" {
		t.Fatalf("user = %q, want alice", u)
	}
	if p := <-gotPass; p != "s3cret" {
		t.Fatalf("pass = %q, want s3cret", p)
	}
}

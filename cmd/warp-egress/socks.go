package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SOCKSRelay struct {
	mu       sync.RWMutex
	address  string
	listener net.Listener
	running  bool
	selector func() (string, error)
	closeCh  chan struct{}
}

func NewSOCKSRelay(address string, selector func() (string, error)) *SOCKSRelay {
	return &SOCKSRelay{address: address, selector: selector, closeCh: make(chan struct{})}
}

func (s *SOCKSRelay) Start() error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = listener
	s.running = true
	s.closeCh = make(chan struct{})
	s.mu.Unlock()
	go s.serve(listener)
	return nil
}

func (s *SOCKSRelay) Running() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.running }

func (s *SOCKSRelay) Close() {
	s.mu.Lock()
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if s.running {
		close(s.closeCh)
	}
	s.running = false
	s.listener = nil
	s.mu.Unlock()
}

func (s *SOCKSRelay) serve(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *SOCKSRelay) handle(client net.Conn) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(30 * time.Second))
	target, err := acceptSOCKS5(client)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	proxyURL, err := s.selector()
	var upstream net.Conn
	if err != nil {
		// 没有可用的已选出口（未配置/未选择全局出口）：回退直连。
		// 否则 CPA 管理请求（配额查询等）会被插件中继阻断。
		upstream, err = (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", target)
	} else {
		proxyAddr, addrErr := socksProxyAddress(proxyURL)
		if addrErr != nil {
			err = addrErr
		} else {
			upstream, err = dialSOCKS5(ctx, proxyAddr, target)
		}
	}
	if err != nil {
		_ = writeSOCKSReply(client, 0x05, nil)
		return
	}
	defer upstream.Close()
	_ = client.SetDeadline(time.Time{})
	_ = upstream.SetDeadline(time.Time{})
	_ = writeSOCKSReply(client, 0x00, upstream.LocalAddr())
	copyStream(client, upstream)
}

func socksProxyAddress(proxyURL string) (string, error) {
	normalized, err := normalizeSOCKSURL(proxyURL)
	if err != nil {
		return "", err
	}
	idx := strings.Index(normalized, "://")
	return normalized[idx+3:], nil
}

func acceptSOCKS5(conn net.Conn) (string, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}
	if header[0] != 5 {
		return "", errors.New("unsupported SOCKS version")
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", err
	}
	hasNoAuth := false
	for _, method := range methods {
		if method == 0 {
			hasNoAuth = true
			break
		}
	}
	if !hasNoAuth {
		_, _ = conn.Write([]byte{5, 0xff})
		return "", errors.New("no-auth method unavailable")
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return "", err
	}
	request := make([]byte, 4)
	if _, err := io.ReadFull(conn, request); err != nil {
		return "", err
	}
	if request[0] != 5 || request[1] != 1 {
		return "", errors.New("only CONNECT is supported")
	}
	host, err := readSOCKSHost(conn, request[3])
	if err != nil {
		return "", err
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBytes)))), nil
}

func readSOCKSHost(reader io.Reader, atyp byte) (string, error) {
	switch atyp {
	case 1:
		raw := make([]byte, 4)
		if _, err := io.ReadFull(reader, raw); err != nil {
			return "", err
		}
		return net.IP(raw).String(), nil
	case 4:
		raw := make([]byte, 16)
		if _, err := io.ReadFull(reader, raw); err != nil {
			return "", err
		}
		return net.IP(raw).String(), nil
	case 3:
		length := make([]byte, 1)
		if _, err := io.ReadFull(reader, length); err != nil {
			return "", err
		}
		raw := make([]byte, int(length[0]))
		if _, err := io.ReadFull(reader, raw); err != nil {
			return "", err
		}
		return string(raw), nil
	default:
		return "", errors.New("unsupported address type")
	}
}

func writeSOCKSReply(conn net.Conn, code byte, address net.Addr) error {
	ip := net.IPv4zero
	port := 0
	if tcp, ok := address.(*net.TCPAddr); ok {
		ip = tcp.IP
		port = tcp.Port
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		payload := []byte{5, code, 0, 1}
		payload = append(payload, ipv4...)
		payload = binary.BigEndian.AppendUint16(payload, uint16(port))
		_, err := conn.Write(payload)
		return err
	}
	payload := []byte{5, code, 0, 4}
	payload = append(payload, ip.To16()...)
	payload = binary.BigEndian.AppendUint16(payload, uint16(port))
	_, err := conn.Write(payload)
	return err
}

func dialSOCKS5(ctx context.Context, proxyAddr, targetAddr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			conn.Close()
		}
	}()
	deadline, ok := ctx.Deadline()
	if ok {
		_ = conn.SetDeadline(deadline)
	}
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return nil, err
	}
	response := make([]byte, 2)
	if _, err := io.ReadFull(conn, response); err != nil {
		return nil, err
	}
	if response[0] != 5 || response[1] != 0 {
		return nil, errors.New("SOCKS5 proxy rejected authentication")
	}
	host, portText, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, err
	}
	request := []byte{5, 1, 0}
	ip := net.ParseIP(host)
	if ipv4 := ip.To4(); ipv4 != nil {
		request = append(request, 1)
		request = append(request, ipv4...)
	} else if ipv6 := ip.To16(); ip != nil {
		request = append(request, 4)
		request = append(request, ipv6...)
	} else {
		if len(host) > 255 {
			return nil, errors.New("target hostname too long")
		}
		request = append(request, 3, byte(len(host)))
		request = append(request, host...)
	}
	request = binary.BigEndian.AppendUint16(request, uint16(port))
	if _, err := conn.Write(request); err != nil {
		return nil, err
	}
	reply := make([]byte, 4)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return nil, err
	}
	if reply[0] != 5 || reply[1] != 0 {
		return nil, fmt.Errorf("SOCKS5 connect failed with code %d", reply[1])
	}
	if _, err := readSOCKSHost(conn, reply[3]); err != nil {
		return nil, err
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	failed = false
	return conn, nil
}

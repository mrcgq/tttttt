package inbound

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// HTTPProxyStats holds server statistics.
type HTTPProxyStats struct {
	ActiveConns int64
	TotalConns  int64
}

// HTTPProxyServer implements an HTTP CONNECT proxy.
type HTTPProxyServer struct {
	Addr        string
	Logger      *zap.Logger
	OnConnect   TunnelFunc
	listener    net.Listener
	wg          sync.WaitGroup
	closeCh     chan struct{}
	closeOnce   sync.Once // 防止重复关闭 closeCh
	activeConns int64
	totalConns  int64
}

func NewHTTPProxyServer(addr string, logger *zap.Logger, onConnect TunnelFunc) *HTTPProxyServer {
	return &HTTPProxyServer{
		Addr:      addr,
		Logger:    logger,
		OnConnect: onConnect,
		closeCh:   make(chan struct{}),
	}
}

// Stats returns current server statistics.
func (s *HTTPProxyServer) Stats() HTTPProxyStats {
	return HTTPProxyStats{
		ActiveConns: atomic.LoadInt64(&s.activeConns),
		TotalConns:  atomic.LoadInt64(&s.totalConns),
	}
}

func (s *HTTPProxyServer) Start() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("httpproxy: listen %s: %w", s.Addr, err)
	}
	s.listener = ln
	s.Logger.Info("http proxy server started", zap.String("addr", s.Addr))

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-s.closeCh:
					return
				default:
					s.Logger.Warn("httpproxy: accept error", zap.Error(err))
					continue
				}
			}
			atomic.AddInt64(&s.totalConns, 1)
			atomic.AddInt64(&s.activeConns, 1)
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				defer atomic.AddInt64(&s.activeConns, -1)
				s.handleConn(conn)
			}()
		}
	}()
	return nil
}

// Stop 停止服务器 — 带 8 秒超时熔断，用 sync.Once 防止重复关闭
func (s *HTTPProxyServer) Stop() {
	s.closeOnce.Do(func() {
		close(s.closeCh)
	})

	if s.listener != nil {
		s.listener.Close()
	}

	// 带超时的等待，防止卡死
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 正常退出
	case <-time.After(8 * time.Second):
		s.Logger.Warn("httpproxy: stop timed out after 8s, forcing shutdown")
	}

	s.Logger.Info("http proxy server stopped",
		zap.Int64("total_connections", atomic.LoadInt64(&s.totalConns)))
}

func (s *HTTPProxyServer) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	br := bufio.NewReaderSize(conn, 4096)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}

	if req.Method != http.MethodConnect {
		_, _ = conn.Write([]byte("HTTP/1.1 405 Method Not Allowed\r\n" +
			"Proxy-Agent: tls-client\r\n" +
			"Content-Length: 0\r\n\r\n"))
		return
	}

	target := req.Host
	if _, _, err := net.SplitHostPort(target); err != nil {
		target = net.JoinHostPort(target, "443")
	}

	domain := req.URL.Hostname()
	s.Logger.Debug("http connect",
		zap.String("target", target),
		zap.String("domain", domain))

	_, _ = conn.Write([]byte("HTTP/1.1 200 Connection established\r\n" +
		"Proxy-Agent: tls-client\r\n\r\n"))
	_ = conn.SetDeadline(time.Time{})

	if s.OnConnect != nil {
		s.OnConnect(conn, target, domain)
	}
}

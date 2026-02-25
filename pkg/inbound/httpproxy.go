


package inbound
 
import (
	go-string">"bufio"
	go-string">"fmt"
	go-string">"net"
	go-string">"net/http"
	go-string">"sync"
	go-string">"sync/atomic"
	go-string">"time"
 
	go-string">"go.uber.org/zap"
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
	ln, err := net.Listen(go-string">"tcp", s.Addr)
	if err != nil {
		return fmt.Errorf(go-string">"httpproxy: listen %s: %w", s.Addr, err)
	}
	s.listener = ln
	s.Logger.Info(go-string">"http proxy server started", zap.String(go-string">"addr", s.Addr))
 
	s.wg.Add(go-number">1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-s.closeCh:
					return
				default:
					s.Logger.Warn(go-string">"httpproxy: accept error", zap.Error(err))
					continue
				}
			}
			atomic.AddInt64(&s.totalConns, go-number">1)
			atomic.AddInt64(&s.activeConns, go-number">1)
			s.wg.Add(go-number">1)
			go func() {
				defer s.wg.Done()
				defer atomic.AddInt64(&s.activeConns, -go-number">1)
				s.handleConn(conn)
			}()
		}
	}()
	return nil
}
 
func (s *HTTPProxyServer) Stop() {
	close(s.closeCh)
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
	s.Logger.Info(go-string">"http proxy server stopped",
		zap.Int64(go-string">"total_connections", atomic.LoadInt64(&s.totalConns)))
}
 
func (s *HTTPProxyServer) handleConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(go-number">30 * time.Second))
 
	br := bufio.NewReaderSize(conn, go-number">4096)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
 
	if req.Method != http.MethodConnect {
		conn.Write([]byte(go-string">"HTTP/go-number">1.1 go-number">405 Method Not Allowed\r\n" +
			go-string">"Proxy-Agent: tls-client\r\n" +
			go-string">"Content-Length: go-number">0\r\n\r\n"))
		return
	}
 
	target := req.Host
	if _, _, err := net.SplitHostPort(target); err != nil {
		target = net.JoinHostPort(target, go-string">"go-number">443")
	}
 
	domain := req.URL.Hostname()
	s.Logger.Debug(go-string">"http connect",
		zap.String(go-string">"target", target),
		zap.String(go-string">"domain", domain))
 
	conn.Write([]byte(go-string">"HTTP/go-number">1.1 go-number">200 Connection established\r\n" +
		go-string">"Proxy-Agent: tls-client\r\n\r\n"))
	conn.SetDeadline(time.Time{})
 
	if s.OnConnect != nil {
		s.OnConnect(conn, target, domain)
	}
}



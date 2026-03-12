package inbound

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

const (
	socks5Version      = 0x05
	socks5AuthNone     = 0x00
	socks5AuthUserPass = 0x02
	socks5AuthNoAccept = 0xFF
	socks5CmdConnect   = 0x01
	socks5CmdBind      = 0x02
	socks5CmdUDPAssoc  = 0x03
	socks5AtypIPv4     = 0x01
	socks5AtypDomain   = 0x03
	socks5AtypIPv6     = 0x04
	socks5RepSuccess   = 0x00
	socks5RepFail      = 0x01
	socks5RepCmdNotSup = 0x07

	userPassVersion = 0x01
)

// TunnelFunc is called when a CONNECT request is accepted.
type TunnelFunc func(clientConn net.Conn, target, domain string)

// UDPRelayFunc is called for UDP ASSOCIATE requests.
type UDPRelayFunc func(clientAddr net.Addr, data []byte, target string) ([]byte, error)

// SOCKS5Stats holds server statistics.
type SOCKS5Stats struct {
	ActiveConns int64
	TotalConns  int64
	TotalErrors int64
	AuthFails   int64
}

// SOCKS5AuthConfig 认证配置
type SOCKS5AuthConfig struct {
	Username string
	Password string
}

// SOCKS5Server implements a SOCKS5 proxy (RFC 1928).
type SOCKS5Server struct {
	Addr       string
	Logger     *zap.Logger
	OnConnect  TunnelFunc
	OnUDP      UDPRelayFunc
	AuthConfig *SOCKS5AuthConfig
	listener   net.Listener
	udpConn    *net.UDPConn
	wg         sync.WaitGroup
	closeCh    chan struct{}
	closeOnce  sync.Once // 防止重复关闭 closeCh

	// Metrics
	activeConns int64
	totalConns  int64
	totalErrors int64
	authFails   int64
}

// NewSOCKS5Server 创建 SOCKS5 服务器
func NewSOCKS5Server(addr string, logger *zap.Logger, onConnect TunnelFunc) *SOCKS5Server {
	return &SOCKS5Server{
		Addr:      addr,
		Logger:    logger,
		OnConnect: onConnect,
		closeCh:   make(chan struct{}),
	}
}

// NewSOCKS5ServerWithAuth 创建带认证的 SOCKS5 服务器
func NewSOCKS5ServerWithAuth(addr string, logger *zap.Logger, onConnect TunnelFunc, username, password string) *SOCKS5Server {
	s := NewSOCKS5Server(addr, logger, onConnect)
	if username != "" {
		s.AuthConfig = &SOCKS5AuthConfig{
			Username: username,
			Password: password,
		}
	}
	return s
}

// SetUDPHandler 设置 UDP 处理器
func (s *SOCKS5Server) SetUDPHandler(handler UDPRelayFunc) {
	s.OnUDP = handler
}

// Stats returns current server statistics.
func (s *SOCKS5Server) Stats() SOCKS5Stats {
	return SOCKS5Stats{
		ActiveConns: atomic.LoadInt64(&s.activeConns),
		TotalConns:  atomic.LoadInt64(&s.totalConns),
		TotalErrors: atomic.LoadInt64(&s.totalErrors),
		AuthFails:   atomic.LoadInt64(&s.authFails),
	}
}

// Start 启动服务器
func (s *SOCKS5Server) Start() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("socks5: listen %s: %w", s.Addr, err)
	}
	s.listener = ln

	authMode := "none"
	if s.AuthConfig != nil {
		authMode = "user/pass"
	}
	s.Logger.Info("socks5 server started",
		zap.String("addr", s.Addr),
		zap.String("auth", authMode),
	)

	if s.OnUDP != nil {
		if err := s.startUDPListener(); err != nil {
			s.Logger.Warn("socks5: failed to start UDP listener", zap.Error(err))
		}
	}

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
					s.Logger.Warn("socks5: accept error", zap.Error(err))
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

func (s *SOCKS5Server) startUDPListener() error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", s.Addr)
	if err != nil {
		return err
	}
	udpAddr := &net.UDPAddr{IP: tcpAddr.IP, Port: 0}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	s.udpConn = conn
	s.Logger.Info("socks5 UDP listener started",
		zap.String("addr", conn.LocalAddr().String()))
	s.wg.Add(1)
	go s.handleUDP()
	return nil
}

func (s *SOCKS5Server) handleUDP() {
	defer s.wg.Done()
	buf := make([]byte, 65535)

	for {
		select {
		case <-s.closeCh:
			return
		default:
		}
		_ = s.udpConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, clientAddr, err := s.udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-s.closeCh:
				return
			default:
				continue
			}
		}
		if n < 10 || buf[0] != 0 || buf[1] != 0 || buf[2] != 0 {
			continue
		}
		atyp := buf[3]
		var target string
		var dataOffset int
		switch atyp {
		case socks5AtypIPv4:
			if n < 10 {
				continue
			}
			ip := net.IP(buf[4:8])
			port := binary.BigEndian.Uint16(buf[8:10])
			target = fmt.Sprintf("%s:%d", ip.String(), port)
			dataOffset = 10
		case socks5AtypDomain:
			if n < 5 {
				continue
			}
			domainLen := int(buf[4])
			if n < 5+domainLen+2 {
				continue
			}
			domain := string(buf[5 : 5+domainLen])
			port := binary.BigEndian.Uint16(buf[5+domainLen : 5+domainLen+2])
			target = fmt.Sprintf("%s:%d", domain, port)
			dataOffset = 5 + domainLen + 2
		case socks5AtypIPv6:
			if n < 22 {
				continue
			}
			ip := net.IP(buf[4:20])
			port := binary.BigEndian.Uint16(buf[20:22])
			target = fmt.Sprintf("[%s]:%d", ip.String(), port)
			dataOffset = 22
		default:
			continue
		}
		data := buf[dataOffset:n]
		if s.OnUDP != nil {
			go func(addr *net.UDPAddr, payload []byte, dst string) {
				resp, err := s.OnUDP(addr, payload, dst)
				if err != nil {
					s.Logger.Debug("socks5 UDP relay error", zap.Error(err))
					return
				}
				if len(resp) > 0 {
					_, _ = s.udpConn.WriteToUDP(resp, addr)
				}
			}(clientAddr, append([]byte(nil), data...), target)
		}
	}
}

// Stop 停止服务器 — 带 8 秒超时熔断，用 sync.Once 防止重复关闭
func (s *SOCKS5Server) Stop() {
	s.closeOnce.Do(func() {
		close(s.closeCh)
	})

	if s.listener != nil {
		s.listener.Close()
	}
	if s.udpConn != nil {
		s.udpConn.Close()
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
		s.Logger.Warn("socks5: stop timed out after 8s, forcing shutdown")
	}

	s.Logger.Info("socks5 server stopped",
		zap.Int64("total_connections", atomic.LoadInt64(&s.totalConns)))
}

func (s *SOCKS5Server) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	// 读取版本和认证方法
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		atomic.AddInt64(&s.totalErrors, 1)
		return
	}
	if header[0] != socks5Version {
		atomic.AddInt64(&s.totalErrors, 1)
		return
	}
	methods := make([]byte, header[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}

	// 选择认证方式
	selectedAuth := s.selectAuthMethod(methods)
	if selectedAuth == socks5AuthNoAccept {
		_, _ = conn.Write([]byte{socks5Version, socks5AuthNoAccept})
		return
	}

	_, _ = conn.Write([]byte{socks5Version, selectedAuth})

	// 执行认证
	if selectedAuth == socks5AuthUserPass {
		if !s.doUserPassAuth(conn) {
			atomic.AddInt64(&s.authFails, 1)
			return
		}
	}

	// 读取请求
	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return
	}
	if req[0] != socks5Version {
		s.sendReply(conn, socks5RepFail, nil)
		return
	}
	cmd := req[1]
	atyp := req[3]
	target, domain, err := s.readAddress(conn, atyp)
	if err != nil {
		s.sendReply(conn, socks5RepFail, nil)
		return
	}

	switch cmd {
	case socks5CmdConnect:
		s.Logger.Debug("socks5 connect",
			zap.String("target", target),
			zap.String("domain", domain))
		s.sendReply(conn, socks5RepSuccess, nil)
		_ = conn.SetDeadline(time.Time{})
		if s.OnConnect != nil {
			s.OnConnect(conn, target, domain)
		}
	case socks5CmdUDPAssoc:
		if s.udpConn == nil {
			s.sendReply(conn, socks5RepCmdNotSup, nil)
			return
		}
		udpAddr := s.udpConn.LocalAddr().(*net.UDPAddr)
		s.sendReply(conn, socks5RepSuccess, udpAddr)
		_ = conn.SetDeadline(time.Time{})
		buf := make([]byte, 1)
		_, _ = conn.Read(buf)
	default:
		s.sendReply(conn, socks5RepCmdNotSup, nil)
	}
}

func (s *SOCKS5Server) selectAuthMethod(methods []byte) byte {
	needAuth := s.AuthConfig != nil && s.AuthConfig.Username != ""

	for _, m := range methods {
		if needAuth && m == socks5AuthUserPass {
			return socks5AuthUserPass
		}
		if !needAuth && m == socks5AuthNone {
			return socks5AuthNone
		}
	}

	if needAuth {
		return socks5AuthNoAccept
	}

	return socks5AuthNone
}

func (s *SOCKS5Server) doUserPassAuth(conn net.Conn) bool {
	ver := make([]byte, 1)
	if _, err := io.ReadFull(conn, ver); err != nil || ver[0] != userPassVersion {
		_, _ = conn.Write([]byte{userPassVersion, 0x01})
		return false
	}

	ulenBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, ulenBuf); err != nil {
		_, _ = conn.Write([]byte{userPassVersion, 0x01})
		return false
	}
	username := make([]byte, ulenBuf[0])
	if _, err := io.ReadFull(conn, username); err != nil {
		_, _ = conn.Write([]byte{userPassVersion, 0x01})
		return false
	}

	plenBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, plenBuf); err != nil {
		_, _ = conn.Write([]byte{userPassVersion, 0x01})
		return false
	}
	password := make([]byte, plenBuf[0])
	if _, err := io.ReadFull(conn, password); err != nil {
		_, _ = conn.Write([]byte{userPassVersion, 0x01})
		return false
	}

	if s.AuthConfig == nil ||
		string(username) != s.AuthConfig.Username ||
		string(password) != s.AuthConfig.Password {
		s.Logger.Warn("socks5: auth failed",
			zap.String("username", string(username)),
			zap.String("remote", conn.RemoteAddr().String()),
		)
		_, _ = conn.Write([]byte{userPassVersion, 0x01})
		return false
	}

	s.Logger.Debug("socks5: auth success", zap.String("username", string(username)))
	_, _ = conn.Write([]byte{userPassVersion, 0x00})
	return true
}

func (s *SOCKS5Server) readAddress(conn net.Conn, atyp byte) (target, domain string, err error) {
	var host string
	switch atyp {
	case socks5AtypIPv4:
		addr := make([]byte, 4)
		if _, err = io.ReadFull(conn, addr); err != nil {
			return
		}
		host = net.IP(addr).String()
	case socks5AtypDomain:
		lenBuf := make([]byte, 1)
		if _, err = io.ReadFull(conn, lenBuf); err != nil {
			return
		}
		domainBuf := make([]byte, lenBuf[0])
		if _, err = io.ReadFull(conn, domainBuf); err != nil {
			return
		}
		host = string(domainBuf)
		domain = host
	case socks5AtypIPv6:
		addr := make([]byte, 16)
		if _, err = io.ReadFull(conn, addr); err != nil {
			return
		}
		host = net.IP(addr).String()
	default:
		err = fmt.Errorf("socks5: unsupported atyp 0x%02x", atyp)
		return
	}
	portBuf := make([]byte, 2)
	if _, err = io.ReadFull(conn, portBuf); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBuf)
	target = net.JoinHostPort(host, fmt.Sprintf("%d", port))
	return
}

func (s *SOCKS5Server) sendReply(conn net.Conn, rep byte, bindAddr net.Addr) {
	reply := make([]byte, 0, 10)
	reply = append(reply, socks5Version, rep, 0x00)
	if bindAddr != nil {
		switch addr := bindAddr.(type) {
		case *net.UDPAddr:
			if ip4 := addr.IP.To4(); ip4 != nil {
				reply = append(reply, socks5AtypIPv4)
				reply = append(reply, ip4...)
			} else {
				reply = append(reply, socks5AtypIPv6)
				reply = append(reply, addr.IP...)
			}
			port := make([]byte, 2)
			binary.BigEndian.PutUint16(port, uint16(addr.Port))
			reply = append(reply, port...)
		default:
			reply = append(reply, socks5AtypIPv4, 0, 0, 0, 0, 0, 0)
		}
	} else {
		reply = append(reply, socks5AtypIPv4, 0, 0, 0, 0, 0, 0)
	}
	_, _ = conn.Write(reply)
}

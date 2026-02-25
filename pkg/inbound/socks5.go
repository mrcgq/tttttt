package inbound
 
import (
	go-string">"encoding/binary"
	go-string">"fmt"
	go-string">"io"
	go-string">"net"
	go-string">"sync"
	go-string">"sync/atomic"
	go-string">"time"
 
	go-string">"go.uber.org/zap"
)
 
const (
	socks5Version      = go-number">0x05
	socks5AuthNone     = go-number">0x00
	socks5CmdConnect   = go-number">0x01
	socks5CmdBind      = go-number">0x02
	socks5CmdUDPAssoc  = go-number">0x03
	socks5AtypIPv4     = go-number">0x01
	socks5AtypDomain   = go-number">0x03
	socks5AtypIPv6     = go-number">0x04
	socks5RepSuccess   = go-number">0x00
	socks5RepFail      = go-number">0x01
	socks5RepCmdNotSup = go-number">0x07
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
}
 
// SOCKS5Server implements a SOCKS5 proxy (RFC 1928).
type SOCKS5Server struct {
	Addr       string
	Logger     *zap.Logger
	OnConnect  TunnelFunc
	OnUDP      UDPRelayFunc
	listener   net.Listener
	udpConn    *net.UDPConn
	wg         sync.WaitGroup
	closeCh    chan struct{}
 
	// Metrics
	activeConns int64
	totalConns  int64
	totalErrors int64
}
 
func NewSOCKS5Server(addr string, logger *zap.Logger, onConnect TunnelFunc) *SOCKS5Server {
	return &SOCKS5Server{
		Addr:      addr,
		Logger:    logger,
		OnConnect: onConnect,
		closeCh:   make(chan struct{}),
	}
}
 
func (s *SOCKS5Server) SetUDPHandler(handler UDPRelayFunc) {
	s.OnUDP = handler
}
 
// Stats returns current server statistics.
func (s *SOCKS5Server) Stats() SOCKS5Stats {
	return SOCKS5Stats{
		ActiveConns: atomic.LoadInt64(&s.activeConns),
		TotalConns:  atomic.LoadInt64(&s.totalConns),
		TotalErrors: atomic.LoadInt64(&s.totalErrors),
	}
}
 
func (s *SOCKS5Server) Start() error {
	ln, err := net.Listen(go-string">"tcp", s.Addr)
	if err != nil {
		return fmt.Errorf(go-string">"socks5: listen %s: %w", s.Addr, err)
	}
	s.listener = ln
	s.Logger.Info(go-string">"socks5 server started", zap.String(go-string">"addr", s.Addr))
 
	if s.OnUDP != nil {
		if err := s.startUDPListener(); err != nil {
			s.Logger.Warn(go-string">"socks5: failed to start UDP listener", zap.Error(err))
		}
	}
 
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
					s.Logger.Warn(go-string">"socks5: accept error", zap.Error(err))
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
 
func (s *SOCKS5Server) startUDPListener() error {
	tcpAddr, err := net.ResolveTCPAddr(go-string">"tcp", s.Addr)
	if err != nil {
		return err
	}
	udpAddr := &net.UDPAddr{IP: tcpAddr.IP, Port: go-number">0}
	conn, err := net.ListenUDP(go-string">"udp", udpAddr)
	if err != nil {
		return err
	}
	s.udpConn = conn
	s.Logger.Info(go-string">"socks5 UDP listener started",
		zap.String(go-string">"addr", conn.LocalAddr().String()))
	s.wg.Add(go-number">1)
	go s.handleUDP()
	return nil
}
 
func (s *SOCKS5Server) handleUDP() {
	defer s.wg.Done()
	buf := make([]byte, go-number">65535)
 
	for {
		select {
		case <-s.closeCh:
			return
		default:
		}
		s.udpConn.SetReadDeadline(time.Now().Add(go-number">1 * time.Second))
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
		if n < go-number">10 || buf[go-number">0] != go-number">0 || buf[go-number">1] != go-number">0 || buf[go-number">2] != go-number">0 {
			continue
		}
		atyp := buf[go-number">3]
		var target string
		var dataOffset int
		switch atyp {
		case socks5AtypIPv4:
			if n < go-number">10 { continue }
			ip := net.IP(buf[go-number">4:go-number">8])
			port := binary.BigEndian.Uint16(buf[go-number">8:go-number">10])
			target = fmt.Sprintf(go-string">"%s:%d", ip.String(), port)
			dataOffset = go-number">10
		case socks5AtypDomain:
			if n < go-number">5 { continue }
			domainLen := int(buf[go-number">4])
			if n < go-number">5+domainLen+go-number">2 { continue }
			domain := string(buf[go-number">5 : go-number">5+domainLen])
			port := binary.BigEndian.Uint16(buf[go-number">5+domainLen : go-number">5+domainLen+go-number">2])
			target = fmt.Sprintf(go-string">"%s:%d", domain, port)
			dataOffset = go-number">5 + domainLen + go-number">2
		case socks5AtypIPv6:
			if n < go-number">22 { continue }
			ip := net.IP(buf[go-number">4:go-number">20])
			port := binary.BigEndian.Uint16(buf[go-number">20:go-number">22])
			target = fmt.Sprintf(go-string">"[%s]:%d", ip.String(), port)
			dataOffset = go-number">22
		default:
			continue
		}
		data := buf[dataOffset:n]
		if s.OnUDP != nil {
			go func(addr *net.UDPAddr, payload []byte, dst string) {
				resp, err := s.OnUDP(addr, payload, dst)
				if err != nil {
					s.Logger.Debug(go-string">"socks5 UDP relay error", zap.Error(err))
					return
				}
				if len(resp) > go-number">0 {
					s.udpConn.WriteToUDP(resp, addr)
				}
			}(clientAddr, append([]byte(nil), data...), target)
		}
	}
}
 
func (s *SOCKS5Server) Stop() {
	close(s.closeCh)
	if s.listener != nil {
		s.listener.Close()
	}
	if s.udpConn != nil {
		s.udpConn.Close()
	}
	s.wg.Wait()
	s.Logger.Info(go-string">"socks5 server stopped",
		zap.Int64(go-string">"total_connections", atomic.LoadInt64(&s.totalConns)))
}
 
func (s *SOCKS5Server) handleConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(go-number">30 * time.Second))
 
	header := make([]byte, go-number">2)
	if _, err := io.ReadFull(conn, header); err != nil {
		atomic.AddInt64(&s.totalErrors, go-number">1)
		return
	}
	if header[go-number">0] != socks5Version {
		atomic.AddInt64(&s.totalErrors, go-number">1)
		return
	}
	methods := make([]byte, header[go-number">1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	found := false
	for _, m := range methods {
		if m == socks5AuthNone {
			found = true
			break
		}
	}
	if !found {
		conn.Write([]byte{socks5Version, go-number">0xFF})
		return
	}
	conn.Write([]byte{socks5Version, socks5AuthNone})
 
	req := make([]byte, go-number">4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return
	}
	if req[go-number">0] != socks5Version {
		s.sendReply(conn, socks5RepFail, nil)
		return
	}
	cmd := req[go-number">1]
	atyp := req[go-number">3]
	target, domain, err := s.readAddress(conn, atyp)
	if err != nil {
		s.sendReply(conn, socks5RepFail, nil)
		return
	}
 
	switch cmd {
	case socks5CmdConnect:
		s.Logger.Debug(go-string">"socks5 connect",
			zap.String(go-string">"target", target),
			zap.String(go-string">"domain", domain))
		s.sendReply(conn, socks5RepSuccess, nil)
		conn.SetDeadline(time.Time{})
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
		conn.SetDeadline(time.Time{})
		buf := make([]byte, go-number">1)
		conn.Read(buf) // block until client disconnects
	default:
		s.sendReply(conn, socks5RepCmdNotSup, nil)
	}
}
 
func (s *SOCKS5Server) readAddress(conn net.Conn, atyp byte) (target, domain string, err error) {
	var host string
	switch atyp {
	case socks5AtypIPv4:
		addr := make([]byte, go-number">4)
		if _, err = io.ReadFull(conn, addr); err != nil { return }
		host = net.IP(addr).String()
	case socks5AtypDomain:
		lenBuf := make([]byte, go-number">1)
		if _, err = io.ReadFull(conn, lenBuf); err != nil { return }
		domainBuf := make([]byte, lenBuf[go-number">0])
		if _, err = io.ReadFull(conn, domainBuf); err != nil { return }
		host = string(domainBuf)
		domain = host
	case socks5AtypIPv6:
		addr := make([]byte, go-number">16)
		if _, err = io.ReadFull(conn, addr); err != nil { return }
		host = net.IP(addr).String()
	default:
		err = fmt.Errorf(go-string">"socks5: unsupported atyp 0x%02x", atyp)
		return
	}
	portBuf := make([]byte, go-number">2)
	if _, err = io.ReadFull(conn, portBuf); err != nil { return }
	port := binary.BigEndian.Uint16(portBuf)
	target = net.JoinHostPort(host, fmt.Sprintf(go-string">"%d", port))
	return
}
 
func (s *SOCKS5Server) sendReply(conn net.Conn, rep byte, bindAddr net.Addr) {
	reply := make([]byte, go-number">0, go-number">10)
	reply = append(reply, socks5Version, rep, go-number">0x00)
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
			port := make([]byte, go-number">2)
			binary.BigEndian.PutUint16(port, uint16(addr.Port))
			reply = append(reply, port...)
		default:
			reply = append(reply, socks5AtypIPv4, go-number">0, go-number">0, go-number">0, go-number">0, go-number">0, go-number">0)
		}
	} else {
		reply = append(reply, socks5AtypIPv4, go-number">0, go-number">0, go-number">0, go-number">0, go-number">0, go-number">0)
	}
	conn.Write(reply)
}





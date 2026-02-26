package outbound

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/user/tls-client/pkg/config"
	"github.com/user/tls-client/pkg/engine"
	"github.com/user/tls-client/pkg/fingerprint"
	"github.com/user/tls-client/pkg/transport"
	"github.com/user/tls-client/pkg/verify"
)

// NodeConfig describes a proxy node with transport settings.
type NodeConfig struct {
	Name         string
	Address      string
	SNI          string
	Profile      *fingerprint.BrowserProfile
	VerifyMode   verify.Mode
	Transport    transport.Transport
	TransportCfg *transport.Config
	Fallback     *transport.FallbackTransport
	Retry        *engine.RetryConfig
}

// NewNodeConfig builds a NodeConfig from parsed configuration.
func NewNodeConfig(
	nodeCfg *config.NodeConfig,
	profile *fingerprint.BrowserProfile,
	vmode verify.Mode,
	logger *zap.Logger,
) *NodeConfig {
	t := transport.Get(nodeCfg.Transport)

	tcfg := &transport.Config{
		Path:      nodeCfg.TransportOpts.WSPath,
		Host:      nodeCfg.TransportOpts.WSHost,
		UserAgent: profile.UserAgent,
		Headers:   nodeCfg.TransportOpts.WSHeaders,
	}
	if t.Name() == "h2" && nodeCfg.TransportOpts.H2Path != "" {
		tcfg.Path = nodeCfg.TransportOpts.H2Path
	}
	tcfg.Normalize()
	if tcfg.Host == "" {
		tcfg.Host = nodeCfg.SNI
	}

	nc := &NodeConfig{
		Name:         nodeCfg.Name,
		Address:      nodeCfg.Address,
		SNI:          nodeCfg.SNI,
		Profile:      profile,
		VerifyMode:   vmode,
		Transport:    t,
		TransportCfg: tcfg,
	}

	if nodeCfg.Retry.MaxAttempts > 1 {
		nc.Retry = &engine.RetryConfig{
			MaxAttempts: nodeCfg.Retry.MaxAttempts,
		}
	}

	if len(nodeCfg.Fallback) > 0 {
		nc.Fallback = transport.NewFallback(nodeCfg.Fallback, logger)
	}

	return nc
}

// TunnelStats holds tunnel operation metrics.
type TunnelStats struct {
	ActiveConns int64
	TotalConns  int64
	TotalBytes  int64
	TotalErrors int64
}

// TunnelManager handles outbound proxy tunnels.
type TunnelManager struct {
	Node   *NodeConfig
	Logger *zap.Logger
	Pool   *engine.ConnPool
	stats  TunnelStats
}

func NewTunnelManager(node *NodeConfig, logger *zap.Logger) *TunnelManager {
	return &TunnelManager{
		Node:   node,
		Logger: logger,
		Pool:   engine.NewConnPool(10, 90*time.Second),
	}
}

// Stats returns tunnel operation statistics.
func (t *TunnelManager) Stats() TunnelStats {
	return TunnelStats{
		ActiveConns: atomic.LoadInt64(&t.stats.ActiveConns),
		TotalConns:  atomic.LoadInt64(&t.stats.TotalConns),
		TotalBytes:  atomic.LoadInt64(&t.stats.TotalBytes),
		TotalErrors: atomic.LoadInt64(&t.stats.TotalErrors),
	}
}

// HandleConnect establishes a tunnel to the target through the proxy node.
func (t *TunnelManager) HandleConnect(clientConn net.Conn, target, domain string) {
	atomic.AddInt64(&t.stats.TotalConns, 1)
	atomic.AddInt64(&t.stats.ActiveConns, 1)
	defer atomic.AddInt64(&t.stats.ActiveConns, -1)

	t.Logger.Info("tunnel: new connection",
		zap.String("target", target),
		zap.String("domain", domain))

	var stream net.Conn
	var activeTransport transport.Transport
	var err error

	if t.Node.Fallback != nil {
		var usedTransport transport.Transport
		stream, usedTransport, err = t.Node.Fallback.WrapWithFallback(
			func(alpn []string) (net.Conn, error) {
				return t.dialNode(alpn)
			},
			t.Node.TransportCfg,
		)
		if err != nil {
			atomic.AddInt64(&t.stats.TotalErrors, 1)
			t.Logger.Error("tunnel: all transports failed",
				zap.String("node", t.Node.Name),
				zap.String("target", target),
				zap.Error(err))
			return
		}
		activeTransport = usedTransport
	} else {
		alpn := t.Node.Transport.ALPNProtos()
		t.Logger.Info("tunnel: dialing node",
			zap.String("node", t.Node.Name),
			zap.Strings("alpn", alpn))

		tlsConn, err2 := t.dialNode(alpn)
		if err2 != nil {
			atomic.AddInt64(&t.stats.TotalErrors, 1)
			t.Logger.Error("tunnel: dial failed",
				zap.String("node", t.Node.Name),
				zap.Error(err2))
			return
		}

		t.Logger.Info("tunnel: tls connected",
			zap.String("node", t.Node.Name),
			zap.String("target", target))

		activeTransport = t.Node.Transport
		transportName := activeTransport.Name()

		if transportName == "h2" && t.Node.TransportCfg.IsProxyMode() {
			h2Cfg := t.Node.TransportCfg.Clone()
			h2Cfg.Target = target

			stream, err = t.Node.Transport.Wrap(tlsConn, h2Cfg)
			if err != nil {
				tlsConn.Close()
				atomic.AddInt64(&t.stats.TotalErrors, 1)
				t.Logger.Error("tunnel: h2 proxy wrap failed",
					zap.String("node", t.Node.Name),
					zap.String("target", target),
					zap.Error(err))
				return
			}

			t.Logger.Info("tunnel: established (h2 proxy mode)",
				zap.String("node", t.Node.Name),
				zap.String("target", target))

			n := relay(clientConn, stream)
			atomic.AddInt64(&t.stats.TotalBytes, n)
			t.Logger.Info("tunnel: relay finished",
				zap.String("target", target),
				zap.Int64("bytes", n))
			stream.Close()
			return
		}

		stream, err = t.Node.Transport.Wrap(tlsConn, t.Node.TransportCfg)
		if err != nil {
			tlsConn.Close()
			atomic.AddInt64(&t.stats.TotalErrors, 1)
			t.Logger.Error("tunnel: transport wrap failed",
				zap.String("node", t.Node.Name),
				zap.String("transport", transportName),
				zap.Error(err))
			return
		}

		t.Logger.Info("tunnel: transport wrapped",
			zap.String("transport", transportName))
	}
	defer stream.Close()

	transportName := activeTransport.Name()
	switch transportName {
	case "ws":
		t.Logger.Info("tunnel: sending ws target",
			zap.String("target", target))
		if err := t.sendWSTarget(stream, target); err != nil {
			atomic.AddInt64(&t.stats.TotalErrors, 1)
			t.Logger.Error("tunnel: send ws target failed", zap.Error(err))
			return
		}
	case "h2":
		if err := t.sendH2Target(stream, target); err != nil {
			atomic.AddInt64(&t.stats.TotalErrors, 1)
			t.Logger.Error("tunnel: send h2 target failed", zap.Error(err))
			return
		}
	default:
		if err := t.sendHTTPConnect(stream, target); err != nil {
			atomic.AddInt64(&t.stats.TotalErrors, 1)
			t.Logger.Error("tunnel: send CONNECT failed", zap.Error(err))
			return
		}
	}

	t.Logger.Info("tunnel: relay started",
		zap.String("node", t.Node.Name),
		zap.String("target", target),
		zap.String("transport", transportName))

	n := relay(clientConn, stream)
	atomic.AddInt64(&t.stats.TotalBytes, n)

	t.Logger.Info("tunnel: relay finished",
		zap.String("target", target),
		zap.Int64("bytes", n))
}

func (t *TunnelManager) dialNode(alpn []string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	poolKey := fmt.Sprintf("%s:%s:%v", t.Node.Address, t.Node.SNI, alpn)
	conn, err := t.Pool.Get(ctx, poolKey, &engine.DialConfig{
		Address:    t.Node.Address,
		SNI:        t.Node.SNI,
		Profile:    t.Node.Profile,
		VerifyMode: t.Node.VerifyMode,
		ALPN:       alpn,
		Retry:      t.Node.Retry,
	})
	return conn, err
}

func (t *TunnelManager) sendWSTarget(conn net.Conn, target string) error {
	_, err := conn.Write([]byte(target))
	return err
}

func (t *TunnelManager) sendH2Target(conn net.Conn, target string) error {
	_, err := conn.Write([]byte(target + "\n"))
	return err
}

func (t *TunnelManager) sendHTTPConnect(stream net.Conn, target string) error {
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	if _, err := stream.Write([]byte(connectReq)); err != nil {
		return fmt.Errorf("send CONNECT: %w", err)
	}
	br := bufio.NewReader(stream)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		return fmt.Errorf("read CONNECT response: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CONNECT rejected: %s", resp.Status)
	}
	return nil
}

func relay(client net.Conn, proxy net.Conn) int64 {
	var wg sync.WaitGroup
	var totalBytes int64
	wg.Add(2)

	go func() {
		defer wg.Done()
		n, _ := io.Copy(proxy, client)
		atomic.AddInt64(&totalBytes, n)
		if tc, ok := proxy.(interface{ CloseWrite() error }); ok {
			_ = tc.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		n, _ := io.Copy(client, proxy)
		atomic.AddInt64(&totalBytes, n)
		if tc, ok := client.(interface{ CloseWrite() error }); ok {
			_ = tc.CloseWrite()
		}
	}()

	wg.Wait()
	return atomic.LoadInt64(&totalBytes)
}

func (t *TunnelManager) Close() {
	if t.Pool != nil {
		t.Pool.Close()
	}
}

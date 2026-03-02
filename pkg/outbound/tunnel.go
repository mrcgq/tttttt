
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

	// H2 路径处理
	if t.Name() == "h2" {
		if nodeCfg.TransportOpts.H2Path != "" {
			tcfg.Path = nodeCfg.TransportOpts.H2Path
		} else if tcfg.Path == "" {
			tcfg.Path = "/tunnel" // 默认 H2 路径
		}
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
	Node       *NodeConfig
	Logger     *zap.Logger
	Pool       *engine.ConnPool
	ProxyIPMgr *ProxyIPManager // 新增：ProxyIP 管理器
	stats      TunnelStats
}

// ProxyIPManager 简化的 ProxyIP 管理器接口
type ProxyIPManager interface {
	Select() *ProxyIPEntry
	MarkFailed(address string)
	MarkSuccess(address string)
}

// ProxyIPEntry 代表一个 ProxyIP
type ProxyIPEntry struct {
	Address string
	SNI     string
}

func NewTunnelManager(node *NodeConfig, logger *zap.Logger) *TunnelManager {
	return &TunnelManager{
		Node:   node,
		Logger: logger,
		Pool:   engine.NewConnPool(10, 90*time.Second),
	}
}

// SetProxyIPManager 设置 ProxyIP 管理器
func (t *TunnelManager) SetProxyIPManager(mgr ProxyIPManager) {
	t.ProxyIPMgr = mgr
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

	// 确定使用的节点地址和SNI
	nodeAddr := t.Node.Address
	nodeSNI := t.Node.SNI

	// 如果启用了 ProxyIP，使用 ProxyIP 管理器选择
	var selectedProxyIP *ProxyIPEntry
	if t.ProxyIPMgr != nil {
		selectedProxyIP = t.ProxyIPMgr.Select()
		if selectedProxyIP != nil {
			nodeAddr = selectedProxyIP.Address
			if selectedProxyIP.SNI != "" {
				nodeSNI = selectedProxyIP.SNI
			}
			t.Logger.Debug("tunnel: using proxyip",
				zap.String("address", nodeAddr),
				zap.String("sni", nodeSNI))
		}
	}

	var stream net.Conn
	var activeTransport transport.Transport
	var err error

	if t.Node.Fallback != nil {
		var usedTransport transport.Transport
		stream, usedTransport, err = t.Node.Fallback.WrapWithFallback(
			func(alpn []string) (net.Conn, error) {
				return t.dialNodeWithAddr(nodeAddr, nodeSNI, alpn)
			},
			t.Node.TransportCfg,
		)
		if err != nil {
			atomic.AddInt64(&t.stats.TotalErrors, 1)
			t.markProxyIPFailed(selectedProxyIP)
			t.Logger.Error("tunnel: all transports failed",
				zap.String("node", t.Node.Name),
				zap.String("target", target),
				zap.Error(err))
			return
		}
		activeTransport = usedTransport
	} else {
		alpn := t.Node.Transport.ALPNProtos()
		t.Logger.Debug("tunnel: dialing node",
			zap.String("node", t.Node.Name),
			zap.String("address", nodeAddr),
			zap.Strings("alpn", alpn))

		tlsConn, err2 := t.dialNodeWithAddr(nodeAddr, nodeSNI, alpn)
		if err2 != nil {
			atomic.AddInt64(&t.stats.TotalErrors, 1)
			t.markProxyIPFailed(selectedProxyIP)
			t.Logger.Error("tunnel: dial failed",
				zap.String("node", t.Node.Name),
				zap.Error(err2))
			return
		}

		t.Logger.Debug("tunnel: tls connected",
			zap.String("node", t.Node.Name),
			zap.String("target", target))

		activeTransport = t.Node.Transport
		transportName := activeTransport.Name()

		// 准备 transport 配置
		transportCfg := t.Node.TransportCfg.Clone()
		transportCfg.Target = target

		stream, err = activeTransport.Wrap(tlsConn, transportCfg)
		if err != nil {
			tlsConn.Close()
			atomic.AddInt64(&t.stats.TotalErrors, 1)
			t.markProxyIPFailed(selectedProxyIP)
			t.Logger.Error("tunnel: transport wrap failed",
				zap.String("node", t.Node.Name),
				zap.String("transport", transportName),
				zap.Error(err))
			return
		}

		t.Logger.Debug("tunnel: transport wrapped",
			zap.String("transport", transportName))
	}
	defer stream.Close()

	// 标记 ProxyIP 成功
	t.markProxyIPSuccess(selectedProxyIP)

	transportName := activeTransport.Name()

	// 根据传输类型发送目标信息
	switch transportName {
	case "ws":
		t.Logger.Debug("tunnel: sending ws target",
			zap.String("target", target))
		if err := t.sendWSTarget(stream, target); err != nil {
			atomic.AddInt64(&t.stats.TotalErrors, 1)
			t.Logger.Error("tunnel: send ws target failed", zap.Error(err))
			return
		}
	case "h2":
		// H2 模式：目标已在 Wrap 时通过 TransportCfg.Target 传递
		// 新的 h2.go 实现会在初始化时发送目标
		t.Logger.Debug("tunnel: h2 target sent via transport config",
			zap.String("target", target))
	case "raw":
		// RAW 模式：发送 HTTP CONNECT
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

func (t *TunnelManager) markProxyIPFailed(entry *ProxyIPEntry) {
	if entry != nil && t.ProxyIPMgr != nil {
		t.ProxyIPMgr.MarkFailed(entry.Address)
	}
}

func (t *TunnelManager) markProxyIPSuccess(entry *ProxyIPEntry) {
	if entry != nil && t.ProxyIPMgr != nil {
		t.ProxyIPMgr.MarkSuccess(entry.Address)
	}
}

func (t *TunnelManager) dialNode(alpn []string) (net.Conn, error) {
	return t.dialNodeWithAddr(t.Node.Address, t.Node.SNI, alpn)
}

func (t *TunnelManager) dialNodeWithAddr(address, sni string, alpn []string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	poolKey := fmt.Sprintf("%s:%s:%v", address, sni, alpn)
	conn, err := t.Pool.Get(ctx, poolKey, &engine.DialConfig{
		Address:    address,
		SNI:        sni,
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

	// client -> proxy
	go func() {
		defer wg.Done()
		n, _ := io.Copy(proxy, client)
		atomic.AddInt64(&totalBytes, n)
		if tc, ok := proxy.(interface{ CloseWrite() error }); ok {
			_ = tc.CloseWrite()
		}
	}()

	// proxy -> client
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


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

// NodeConfig 节点配置（内部使用）
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

	// 连接池配置
	PoolConfig *engine.PoolConfig

	// 远程代理配置 (Xlink 借力机制)
	RemoteSOCKS5   string
	RemoteFallback string
}

// NewNodeConfig 从配置创建节点
func NewNodeConfig(
	nodeCfg *config.NodeConfig,
	profile *fingerprint.BrowserProfile,
	vmode verify.Mode,
	logger *zap.Logger,
) *NodeConfig {
	var t transport.Transport
	if nodeCfg.Transport == "socks5-out" || nodeCfg.Transport == "socks5out" {
		t = transport.GetWithConfig(
			nodeCfg.Transport,
			nodeCfg.TransportOpts.SOCKS5Addr,
			nodeCfg.TransportOpts.SOCKS5Username,
			nodeCfg.TransportOpts.SOCKS5Password,
		)
	} else {
		t = transport.Get(nodeCfg.Transport)
	}

	tcfg := &transport.Config{
		Path:      nodeCfg.TransportOpts.WSPath,
		Host:      nodeCfg.TransportOpts.WSHost,
		UserAgent: profile.UserAgent,
		Headers:   nodeCfg.TransportOpts.WSHeaders,
		SOCKS5OutAddr:     nodeCfg.TransportOpts.SOCKS5Addr,
		SOCKS5OutUsername: nodeCfg.TransportOpts.SOCKS5Username,
		SOCKS5OutPassword: nodeCfg.TransportOpts.SOCKS5Password,
	}

	if t.Name() == "h2" || t.Name() == "ws" {
		if nodeCfg.TransportOpts.H2Path != "" {
			tcfg.Path = nodeCfg.TransportOpts.H2Path
		}
		if tcfg.Path == "" {
			tcfg.Path = "/"
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

	// 设置远程代理配置
	nc.RemoteSOCKS5 = nodeCfg.RemoteProxy.SOCKS5
	nc.RemoteFallback = nodeCfg.RemoteProxy.Fallback

	// 设置重试配置
	if nodeCfg.Retry.MaxAttempts > 1 {
		nc.Retry = &engine.RetryConfig{
			MaxAttempts: nodeCfg.Retry.MaxAttempts,
			BaseDelay:   nodeCfg.Retry.ParseBaseDelay(),
			MaxDelay:    nodeCfg.Retry.ParseMaxDelay(),
			Jitter:      nodeCfg.Retry.GetJitter(),
		}
	}

	// 设置连接池配置
	nc.PoolConfig = &engine.PoolConfig{
		MaxIdle:     nodeCfg.Pool.GetMaxIdle(),
		MaxPerKey:   nodeCfg.Pool.GetMaxPerKey(),
		IdleTimeout: nodeCfg.Pool.ParseIdleTimeout(),
		MaxLifetime: nodeCfg.Pool.ParseMaxLifetime(),
	}

	if len(nodeCfg.Fallback) > 0 {
		nc.Fallback = transport.NewFallback(nodeCfg.Fallback, logger)
	}

	return nc
}

// TunnelStats 隧道统计
type TunnelStats struct {
	ActiveConns int64
	TotalConns  int64
	TotalBytes  int64
	TotalErrors int64
}

// ProxyIPSelector ProxyIP选择器接口
type ProxyIPSelector interface {
	Select() *ProxyIPEntry
	MarkFailed(address string)
	MarkSuccess(address string)
}

// ProxyIPEntry ProxyIP条目
type ProxyIPEntry struct {
	Address string
	SNI     string
}

// TunnelManager 隧道管理器
type TunnelManager struct {
	Node            *NodeConfig
	Logger          *zap.Logger
	Pool            *engine.ConnPool
	ProxyIPSelector ProxyIPSelector

	// 原子计数器 — 实时统计
	activeConns int64
	totalConns  int64
	totalBytes  int64
	totalErrors int64
}

// NewTunnelManager 创建隧道管理器
func NewTunnelManager(node *NodeConfig, logger *zap.Logger) *TunnelManager {
	var pool *engine.ConnPool

	if node.PoolConfig != nil {
		pool = engine.NewConnPoolWithConfig(*node.PoolConfig)
	} else {
		pool = engine.NewConnPool(10, 90*time.Second)
	}

	return &TunnelManager{
		Node:   node,
		Logger: logger,
		Pool:   pool,
	}
}

// NewTunnelManagerWithProxyIP 创建带 ProxyIP 支持的隧道管理器
func NewTunnelManagerWithProxyIP(node *NodeConfig, logger *zap.Logger, selector ProxyIPSelector) *TunnelManager {
	tm := NewTunnelManager(node, logger)
	tm.ProxyIPSelector = selector
	return tm
}

// Stats 获取统计信息 — 原子读取
func (t *TunnelManager) Stats() TunnelStats {
	return TunnelStats{
		ActiveConns: atomic.LoadInt64(&t.activeConns),
		TotalConns:  atomic.LoadInt64(&t.totalConns),
		TotalBytes:  atomic.LoadInt64(&t.totalBytes),
		TotalErrors: atomic.LoadInt64(&t.totalErrors),
	}
}

// HandleConnect 处理连接请求
func (t *TunnelManager) HandleConnect(clientConn net.Conn, target, domain string) {
	atomic.AddInt64(&t.totalConns, 1)
	atomic.AddInt64(&t.activeConns, 1)
	defer atomic.AddInt64(&t.activeConns, -1)

	t.Logger.Info("tunnel: new connection",
		zap.String("target", target),
		zap.String("domain", domain),
	)

	// 如果配置了 ProxyIP 选择器，使用动态选择的地址
	nodeAddr := t.Node.Address
	nodeSNI := t.Node.SNI

	if t.ProxyIPSelector != nil {
		entry := t.ProxyIPSelector.Select()
		if entry != nil {
			nodeAddr = entry.Address
			if entry.SNI != "" {
				nodeSNI = entry.SNI
			}
			t.Logger.Debug("tunnel: using selected proxy ip",
				zap.String("address", nodeAddr),
				zap.String("sni", nodeSNI),
			)
		}
	}

	var stream net.Conn
	var activeTransport transport.Transport
	var err error

	if t.Node.Fallback != nil {
		var usedTransport transport.Transport
		stream, usedTransport, err = t.Node.Fallback.WrapWithFallback(
			func(alpn []string) (net.Conn, error) {
				return t.dialNode(nodeAddr, nodeSNI, alpn)
			},
			t.Node.TransportCfg,
		)
		if err != nil {
			atomic.AddInt64(&t.totalErrors, 1)
			t.markProxyIPFailed(nodeAddr)
			t.Logger.Error("tunnel: all transports failed",
				zap.String("node", t.Node.Name),
				zap.String("target", target),
				zap.Error(err),
			)
			return
		}
		activeTransport = usedTransport
	} else {
		alpn := t.Node.Transport.ALPNProtos()
		activeTransport = t.Node.Transport
		transportName := activeTransport.Name()

		t.Logger.Debug("tunnel: dialing node",
			zap.String("node", t.Node.Name),
			zap.String("address", nodeAddr),
			zap.String("transport", transportName),
			zap.Strings("alpn", alpn),
		)

		// 通过连接池拨号
		tlsConn, dialErr := t.dialNode(nodeAddr, nodeSNI, alpn)
		if dialErr != nil {
			atomic.AddInt64(&t.totalErrors, 1)
			t.markProxyIPFailed(nodeAddr)
			t.Logger.Error("tunnel: dial failed",
				zap.String("node", t.Node.Name),
				zap.Error(dialErr),
			)
			return
		}

		t.Logger.Debug("tunnel: tls connected",
			zap.String("node", t.Node.Name),
			zap.String("target", target),
		)

		// 克隆配置并注入 Xlink 借力参数
		transportCfg := t.Node.TransportCfg.Clone()
		transportCfg.Target = target
		transportCfg.SOCKS5Proxy = t.Node.RemoteSOCKS5
		transportCfg.Fallback = t.Node.RemoteFallback

		// 传输层包装
		stream, err = activeTransport.Wrap(tlsConn, transportCfg)
		if err != nil {
			tlsConn.Close()
			atomic.AddInt64(&t.totalErrors, 1)
			t.markProxyIPFailed(nodeAddr)
			t.Logger.Error("tunnel: transport wrap failed",
				zap.String("node", t.Node.Name),
				zap.String("transport", transportName),
				zap.Error(err),
			)
			return
		}

		t.Logger.Debug("tunnel: transport wrapped",
			zap.String("transport", transportName),
			zap.String("socks5_proxy", t.Node.RemoteSOCKS5),
			zap.String("fallback", t.Node.RemoteFallback),
		)
	}
	defer stream.Close()

	// 标记 ProxyIP 成功
	t.markProxyIPSuccess(nodeAddr)

	transportName := activeTransport.Name()

	// 日志记录
	switch transportName {
	case "ws":
		t.Logger.Debug("tunnel: ws xlink header sent",
			zap.String("target", target),
			zap.String("socks5", t.Node.RemoteSOCKS5),
			zap.String("fallback", t.Node.RemoteFallback),
		)
	case "h2":
		t.Logger.Debug("tunnel: h2 tunnel established (via ws)",
			zap.String("target", target),
		)
	case "socks5-out":
		t.Logger.Debug("tunnel: socks5-out tunnel established",
			zap.String("target", target),
			zap.String("proxy", t.Node.TransportCfg.SOCKS5OutAddr),
		)
	case "raw":
		if err := t.sendHTTPConnect(stream, target); err != nil {
			atomic.AddInt64(&t.totalErrors, 1)
			t.Logger.Error("tunnel: send CONNECT failed", zap.Error(err))
			return
		}
	}

	t.Logger.Info("tunnel: relay started",
		zap.String("node", t.Node.Name),
		zap.String("target", target),
		zap.String("transport", transportName),
	)

	n := t.relay(clientConn, stream)
	atomic.AddInt64(&t.totalBytes, n)

	t.Logger.Info("tunnel: relay finished",
		zap.String("target", target),
		zap.Int64("bytes", n),
	)
}

// markProxyIPFailed 标记 ProxyIP 失败
func (t *TunnelManager) markProxyIPFailed(address string) {
	if t.ProxyIPSelector != nil {
		t.ProxyIPSelector.MarkFailed(address)
	}
}

// markProxyIPSuccess 标记 ProxyIP 成功
func (t *TunnelManager) markProxyIPSuccess(address string) {
	if t.ProxyIPSelector != nil {
		t.ProxyIPSelector.MarkSuccess(address)
	}
}

// dialNode 通过连接池拨号 — 根据 address|sni|alpn 生成 poolKey 复用连接
func (t *TunnelManager) dialNode(address, sni string, alpn []string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 按 address|profile|alpn 生成 Key，确保连接池真正复用
	poolKey := fmt.Sprintf("%s|%s|%s|%v", address, sni, t.Node.Profile.Name, alpn)

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

func (t *TunnelManager) relay(client net.Conn, proxy net.Conn) int64 {
	var wg sync.WaitGroup
	var totalBytes int64
	wg.Add(2)

	go func() {
		defer wg.Done()
		n, err := io.Copy(proxy, client)
		if err != nil {
			t.Logger.Debug("relay: client→proxy error",
				zap.Int64("bytes", n),
				zap.Error(err),
			)
		}
		atomic.AddInt64(&totalBytes, n)
		if tc, ok := proxy.(interface{ CloseWrite() error }); ok {
			_ = tc.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		n, err := io.Copy(client, proxy)
		if err != nil {
			t.Logger.Debug("relay: proxy→client error",
				zap.Int64("bytes", n),
				zap.Error(err),
			)
		}
		atomic.AddInt64(&totalBytes, n)
		if tc, ok := client.(interface{ CloseWrite() error }); ok {
			_ = tc.CloseWrite()
		}
	}()

	wg.Wait()
	return atomic.LoadInt64(&totalBytes)
}

// Close 关闭隧道管理器
func (t *TunnelManager) Close() {
	if t.Pool != nil {
		t.Pool.Close()
	}
}

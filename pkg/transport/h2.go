package transport

import (
	"fmt"
	"net"
	"sync"

	"github.com/user/tls-client/internal/h2"
)

// H2Transport 通过真正的 HTTP/2 帧建立隧道。
//
// 工作流程：
//  1. 在 TLS 连接上发送 HTTP/2 connection preface（SETTINGS + WINDOW_UPDATE）
//  2. 发送 POST /tunnel 的 HEADERS 帧
//  3. 通过 DATA 帧发送目标地址（target + "\n"）
//  4. 等待 Worker 返回 200 OK
//  5. 后续数据通过 DATA 帧双向传输
type H2Transport struct{}

func (t *H2Transport) Name() string         { return "h2" }
func (t *H2Transport) ALPNProtos() []string { return []string{"h2", "http/1.1"} }

func (t *H2Transport) Info() TransportInfo {
	return TransportInfo{
		SupportsMultiplex: true,
		SupportsBinary:    true,
		RequiresUpgrade:   false,
		MaxFrameSize:      16384,
	}
}

func (t *H2Transport) Wrap(conn net.Conn, cfg *Config) (net.Conn, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	// 获取 H2 指纹配置
	var fp *h2.FingerprintConfig
	if cfg.H2Config != nil {
		if fpCfg, ok := cfg.H2Config.(*h2.FingerprintConfig); ok {
			fp = fpCfg
		}
	}
	if fp == nil {
		// 如果没有传入指纹，使用 Chrome 默认配置
		defaultFP := h2.ChromeDefaultConfig()
		fp = &defaultFP
	}

	host := cfg.Host
	if host == "" {
		host = "localhost"
	}

	path := cfg.Path
	if path == "" {
		path = "/tunnel"
	}

	ua := cfg.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
			"(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
	}

	target := cfg.Target

	// 步骤 1：创建 H2 Client
	// —— 发送 connection preface（SETTINGS + WINDOW_UPDATE），启动 readLoop
	client, err := h2.NewClient(conn, fp)
	if err != nil {
		return nil, fmt.Errorf("h2 transport: create client: %w", err)
	}

	// 步骤 2-4：打开隧道
	// —— 发送 POST 请求 HEADERS + 目标地址 DATA，等待 200 响应
	var initialData []byte
	if target != "" {
		initialData = []byte(target + "\n")
	}

	tunnel, err := client.OpenTunnel(host, path, ua, cfg.Headers, initialData)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("h2 transport: open tunnel: %w", err)
	}

	// 返回包装后的 net.Conn，关闭时同时关闭 H2 Client + TLS 连接
	return &h2ConnWrapper{
		H2TunnelConn: tunnel,
		client:       client,
	}, nil
}

// h2ConnWrapper 确保关闭 tunnel stream 时同时关闭底层 H2 Client。
type h2ConnWrapper struct {
	*h2.H2TunnelConn
	client *h2.Client
	once   sync.Once
}

func (w *h2ConnWrapper) Close() error {
	var err error
	w.once.Do(func() {
		// 先关闭 tunnel stream（发送 EndStream）
		_ = w.H2TunnelConn.Close()
		// 再关闭 H2 Client（关闭底层 TLS 连接，停止 readLoop）
		err = w.client.Close()
	})
	return err
}

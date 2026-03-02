package transport

import (
	"fmt"
	"net"
	"sync"

	"github.com/user/tls-client/internal/h2"
)

// H2Transport 通过真正的 HTTP/2 帧建立隧道。
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
	client, err := h2.NewClient(conn, fp)
	if err != nil {
		return nil, fmt.Errorf("h2 transport: create client: %w", err)
	}

	// 步骤 2：等待 SETTINGS 交换完成（关键修复！）
	if err := client.WaitReady(10_000_000_000); err != nil { // 10 秒
		client.Close()
		return nil, fmt.Errorf("h2 transport: settings exchange: %w", err)
	}

	// 步骤 3：打开隧道，target 通过 X-Target header 传递
	extraHeaders := make(map[string]string)
	if target != "" {
		extraHeaders["x-target"] = target
	}
	for k, v := range cfg.Headers {
		extraHeaders[k] = v
	}

	tunnel, err := client.OpenTunnel(host, path, ua, extraHeaders)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("h2 transport: open tunnel: %w", err)
	}

	return &h2ConnWrapper{
		H2TunnelConn: tunnel,
		client:       client,
	}, nil
}

type h2ConnWrapper struct {
	*h2.H2TunnelConn
	client *h2.Client
	once   sync.Once
}

func (w *h2ConnWrapper) Close() error {
	var err error
	w.once.Do(func() {
		_ = w.H2TunnelConn.Close()
		err = w.client.Close()
	})
	return err
}


package transport

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// H2Transport 使用 WebSocket 协议实现隧道
// 注意：虽然名为 "h2"，但实际改用 WebSocket 以确保与 CF Workers 兼容
// 原因：CF Workers 对普通 HTTP POST 有 CPU 时间限制，只有 WebSocket 允许长连接
type H2Transport struct{}

func (t *H2Transport) Name() string { return "h2" }

// ALPNProtos 声明 http/1.1，WebSocket 升级需要 HTTP/1.1
func (t *H2Transport) ALPNProtos() []string { return []string{"http/1.1"} }

func (t *H2Transport) Info() TransportInfo {
	return TransportInfo{
		SupportsMultiplex: false,
		SupportsBinary:    true,
		RequiresUpgrade:   true,
		MaxFrameSize:      16384,
	}
}

func (t *H2Transport) Wrap(conn net.Conn, cfg *Config) (net.Conn, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	host := cfg.Host
	if host == "" {
		host = "localhost"
	}

	path := cfg.Path
	if path == "" {
		path = "/"
	}

	ua := cfg.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
			"(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
	}

	target := cfg.Target

	// 使用 WebSocket 握手（与 WSTransport 相同的方式）
	wsT := &WSTransport{}
	wsConn, err := wsT.Wrap(conn, cfg)
	if err != nil {
		return nil, fmt.Errorf("h2: ws upgrade failed: %w", err)
	}

	// WebSocket 建立后，发送 target 地址作为第一个消息
	if target != "" {
		if _, err := wsConn.Write([]byte(target)); err != nil {
			wsConn.Close()
			return nil, fmt.Errorf("h2: write target: %w", err)
		}
	}

	return wsConn, nil
}




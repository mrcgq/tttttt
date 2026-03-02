package transport

import (
	"fmt"
	"net"
	"strings"
)

type Transport interface {
	Wrap(tlsConn net.Conn, cfg *Config) (net.Conn, error)
	Name() string
	ALPNProtos() []string
	Info() TransportInfo
}

type TransportInfo struct {
	SupportsMultiplex bool
	SupportsBinary    bool
	RequiresUpgrade   bool
	MaxFrameSize      int
}

type TransportStats struct {
	ConnectionsOpened int64
	ConnectionsClosed int64
	BytesRead         int64
	BytesWritten      int64
	Errors            int64
}

type Config struct {
	Path        string
	Host        string
	UserAgent   string
	Headers     map[string]string
	MaxIdleTime int
	Target      string

	// H2Config 保留用于未来可能的真正 HTTP/2 支持
	H2Config interface{}
}

func (c *Config) Validate() error {
	if c.Path != "" && c.Path[0] != '/' {
		return fmt.Errorf("transport: path must start with '/', got %q", c.Path)
	}
	if c.Target != "" {
		if !strings.Contains(c.Target, ":") {
			return fmt.Errorf("transport: target must be host:port, got %q", c.Target)
		}
	}
	return nil
}

func (c *Config) Clone() *Config {
	clone := *c
	if c.Headers != nil {
		clone.Headers = make(map[string]string, len(c.Headers))
		for k, v := range c.Headers {
			clone.Headers[k] = v
		}
	}
	return &clone
}

func (c *Config) Normalize() {
	if c.Path == "" {
		c.Path = "/"
	}
}

func (c *Config) IsProxyMode() bool {
	return c.Target != "" || c.Path == "" || c.Path == "/"
}

func Get(name string) Transport {
	switch name {
	case "ws", "websocket", "WebSocket":
		return &WSTransport{}
	case "h2", "http2", "HTTP2", "h2c":
		return &H2Transport{}
	case "raw", "direct", "tcp", "":
		return &RawTransport{}
	default:
		return &RawTransport{}
	}
}

func Names() []string {
	return []string{"raw", "ws", "h2"}
}

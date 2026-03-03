
package transport

import (
	"fmt"
	"net"
	"strings"
)

// Transport 定义传输层接口
type Transport interface {
	Wrap(tlsConn net.Conn, cfg *Config) (net.Conn, error)
	Name() string
	ALPNProtos() []string
	Info() TransportInfo
}

// TransportInfo 传输层信息
type TransportInfo struct {
	SupportsMultiplex bool
	SupportsBinary    bool
	RequiresUpgrade   bool
	MaxFrameSize      int
}

// TransportStats 传输层统计
type TransportStats struct {
	ConnectionsOpened int64
	ConnectionsClosed int64
	BytesRead         int64
	BytesWritten      int64
	Errors            int64
}

// Config 传输层配置
type Config struct {
	// 基础配置
	Path        string
	Host        string
	UserAgent   string
	Headers     map[string]string
	MaxIdleTime int
	Target      string // 最终目标地址 (host:port)

	// [核心新增] Xlink 借力配置
	SOCKS5Proxy string // 远程 Worker 使用的 SOCKS5 代理 (格式: user:pass@host:port)
	Fallback    string // 远程 Worker 连接失败时的备用地址 (格式: host:port)

	// H2 配置 (保留用于未来 HTTP/2 支持)
	H2Config interface{}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.Path != "" && c.Path[0] != '/' {
		return fmt.Errorf("transport: path must start with '/', got %q", c.Path)
	}
	if c.Target != "" {
		if !strings.Contains(c.Target, ":") {
			return fmt.Errorf("transport: target must be host:port, got %q", c.Target)
		}
	}
	// 验证 SOCKS5 格式
	if c.SOCKS5Proxy != "" {
		if err := validateSOCKS5Format(c.SOCKS5Proxy); err != nil {
			return fmt.Errorf("transport: invalid socks5_proxy: %w", err)
		}
	}
	// 验证 Fallback 格式
	if c.Fallback != "" {
		if !strings.Contains(c.Fallback, ":") {
			return fmt.Errorf("transport: fallback must be host:port, got %q", c.Fallback)
		}
	}
	return nil
}

// validateSOCKS5Format 验证 SOCKS5 地址格式
func validateSOCKS5Format(s string) error {
	// 支持格式:
	// - host:port
	// - user:pass@host:port
	if s == "" {
		return nil
	}

	addr := s
	if strings.Contains(s, "@") {
		parts := strings.SplitN(s, "@", 2)
		if len(parts) != 2 || parts[1] == "" {
			return fmt.Errorf("invalid format, expected user:pass@host:port")
		}
		addr = parts[1]
	}

	if !strings.Contains(addr, ":") {
		return fmt.Errorf("missing port in address")
	}

	return nil
}

// Clone 克隆配置
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

// Normalize 规范化配置
func (c *Config) Normalize() {
	if c.Path == "" {
		c.Path = "/"
	}
}

// IsProxyMode 检查是否为代理模式
func (c *Config) IsProxyMode() bool {
	return c.Target != "" || c.Path == "" || c.Path == "/"
}

// HasRemoteProxy 检查是否配置了远程代理
func (c *Config) HasRemoteProxy() bool {
	return c.SOCKS5Proxy != "" || c.Fallback != ""
}

// Get 根据名称获取传输层
func Get(name string) Transport {
	switch strings.ToLower(name) {
	case "ws", "websocket":
		return &WSTransport{}
	case "h2", "http2", "h2c":
		return &H2Transport{}
	case "raw", "direct", "tcp", "":
		return &RawTransport{}
	default:
		return &RawTransport{}
	}
}

// Names 返回所有支持的传输层名称
func Names() []string {
	return []string{"raw", "ws", "h2"}
}



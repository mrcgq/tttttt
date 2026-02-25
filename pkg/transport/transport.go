package transport

import (
	"fmt"
	"net"
	"strings"
)

// Transport wraps a TLS connection with a transport-layer protocol.
// The returned net.Conn is a bidirectional byte stream that upper layers
// use to send proxy protocol requests (e.g., HTTP CONNECT).
type Transport interface {
	// Wrap establishes the transport layer on an existing TLS connection.
	// cfg provides transport-specific parameters (path, host, headers).
	Wrap(tlsConn net.Conn, cfg *Config) (net.Conn, error)

	// Name returns the transport identifier ("raw", "ws", "h2").
	Name() string

	// ALPNProtos returns the preferred ALPN protocols for TLS negotiation.
	// WS needs ["http/1.1"], H2 needs ["h2"], Raw can use either.
	ALPNProtos() []string

	// Info returns transport capability information.
	Info() TransportInfo
}

// TransportInfo describes the capabilities of a transport.
type TransportInfo struct {
	// SupportsMultiplex indicates if the transport can carry multiple
	// streams on a single connection (e.g., H2).
	SupportsMultiplex bool

	// SupportsBinary indicates if binary data is natively supported
	// without encoding (e.g., WS binary frames).
	SupportsBinary bool

	// RequiresUpgrade indicates if an HTTP upgrade handshake is needed
	// before data transfer (e.g., WebSocket).
	RequiresUpgrade bool

	// MaxFrameSize is the maximum payload per frame (0 = unlimited/stream).
	MaxFrameSize int
}

// TransportStats tracks per-transport operational metrics.
type TransportStats struct {
	ConnectionsOpened int64
	ConnectionsClosed int64
	BytesRead         int64
	BytesWritten      int64
	Errors            int64
}

// Config holds transport-specific parameters.
type Config struct {
	// Path is the HTTP path for upgrade/stream requests (default "/").
	Path string

	// Host overrides the Host header in upgrade/stream requests.
	Host string

	// UserAgent is set on upgrade requests to match browser profile.
	UserAgent string

	// Headers are extra headers for the upgrade/stream request.
	Headers map[string]string

	// MaxIdleTime is the maximum idle time before the transport
	// considers the connection stale (0 = use transport default).
	MaxIdleTime int // seconds

	// [NEW] Target is the proxy destination address for H2 CONNECT mode.
	//
	// When set, H2 transport will send an HTTP/2 CONNECT request with
	// :authority set to this value. The CONNECT method is HPACK-compressed,
	// making it indistinguishable from normal browser HTTP/2 traffic.
	//
	// Format: "host:port" (e.g., "google.com:443")
	//
	// This field enables "proxy mode" vs "tunnel mode":
	//   - Proxy mode (Target set): H2 CONNECT, binary-encoded, anti-DPI
	//   - Tunnel mode (Target empty): H2 POST to Path, for Workers
	Target string
}

// Validate checks the config for basic correctness.
func (c *Config) Validate() error {
	if c.Path != "" && c.Path[0] != '/' {
		return fmt.Errorf("transport: path must start with '/', got %q", c.Path)
	}
	// [NEW] Validate Target format if set
	if c.Target != "" {
		if !strings.Contains(c.Target, ":") {
			return fmt.Errorf("transport: target must be host:port, got %q", c.Target)
		}
	}
	return nil
}

// Clone returns a deep copy of the config.
func (c *Config) Clone() *Config {
	clone := *c // copies all value fields including Target
	if c.Headers != nil {
		clone.Headers = make(map[string]string, len(c.Headers))
		for k, v := range c.Headers {
			clone.Headers[k] = v
		}
	}
	return &clone
}

// Normalize fills in defaults for missing fields.
func (c *Config) Normalize() {
	if c.Path == "" {
		c.Path = "/"
	}
}

// IsProxyMode returns true if this config enables H2 CONNECT proxy mode.
//
// Proxy mode is enabled when:
//   - Target is explicitly set, OR
//   - Path is empty or root "/" (indicating direct proxy, not Worker tunnel)
//
// [NEW] Helper method for H2 transport to determine operation mode.
func (c *Config) IsProxyMode() bool {
	return c.Target != "" || c.Path == "" || c.Path == "/"
}

// Get returns a Transport by name. Normalizes aliases.
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

// Names returns all supported transport names.
func Names() []string {
	return []string{"raw", "ws", "h2"}
}






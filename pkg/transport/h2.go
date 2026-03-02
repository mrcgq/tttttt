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

// H2Transport wraps a TLS connection using HTTP POST stream.
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
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
	}

	target := cfg.Target

	return &h2StreamConn{
		rawConn:  conn,
		host:     host,
		path:     path,
		target:   target,
		ua:       ua,
		headers:  cfg.Headers,
		initOnce: &sync.Once{},
	}, nil
}

type h2StreamConn struct {
	rawConn net.Conn
	host    string
	path    string
	target  string
	ua      string
	headers map[string]string

	initOnce   *sync.Once
	initErr    error
	respReader *bufio.Reader
	writeMu    sync.Mutex
	closed     bool
	closeMu    sync.Mutex
}

func (c *h2StreamConn) init() {
	c.initOnce.Do(func() {
		c.initErr = c.doInit()
	})
}

func (c *h2StreamConn) doInit() error {
	var reqBuf bytes.Buffer

	fmt.Fprintf(&reqBuf, "POST %s HTTP/1.1\r\n", c.path)
	fmt.Fprintf(&reqBuf, "Host: %s\r\n", c.host)
	fmt.Fprintf(&reqBuf, "User-Agent: %s\r\n", c.ua)
	fmt.Fprintf(&reqBuf, "Content-Type: application/octet-stream\r\n")
	fmt.Fprintf(&reqBuf, "Transfer-Encoding: chunked\r\n")
	fmt.Fprintf(&reqBuf, "Connection: keep-alive\r\n")

	for k, v := range c.headers {
		fmt.Fprintf(&reqBuf, "%s: %s\r\n", k, v)
	}
	fmt.Fprintf(&reqBuf, "\r\n")

	if _, err := c.rawConn.Write(reqBuf.Bytes()); err != nil {
		return fmt.Errorf("h2: write request headers: %w", err)
	}

	if c.target != "" {
		targetLine := c.target + "\n"
		chunk := fmt.Sprintf("%x\r\n%s\r\n", len(targetLine), targetLine)
		if _, err := c.rawConn.Write([]byte(chunk)); err != nil {
			return fmt.Errorf("h2: write target chunk: %w", err)
		}
	}

	c.respReader = bufio.NewReader(c.rawConn)

	resp, err := http.ReadResponse(c.respReader, nil)
	if err != nil {
		return fmt.Errorf("h2: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Body.Close()
		return fmt.Errorf("h2: server returned %s", resp.Status)
	}

	return nil
}

func (c *h2StreamConn) Read(p []byte) (int, error) {
	c.init()
	if c.initErr != nil {
		return 0, c.initErr
	}

	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return 0, io.EOF
	}
	c.closeMu.Unlock()

	return c.respReader.Read(p)
}

func (c *h2StreamConn) Write(p []byte) (int, error) {
	c.init()
	if c.initErr != nil {
		return 0, c.initErr
	}

	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return 0, io.ErrClosedPipe
	}
	c.closeMu.Unlock()

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	chunk := fmt.Sprintf("%x\r\n", len(p))
	if _, err := c.rawConn.Write([]byte(chunk)); err != nil {
		return 0, err
	}
	if _, err := c.rawConn.Write(p); err != nil {
		return 0, err
	}
	if _, err := c.rawConn.Write([]byte("\r\n")); err != nil {
		return 0, err
	}

	return len(p), nil
}

func (c *h2StreamConn) Close() error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return nil
	}
	c.closed = true
	c.closeMu.Unlock()

	c.writeMu.Lock()
	_, _ = c.rawConn.Write([]byte("0\r\n\r\n"))
	c.writeMu.Unlock()

	return c.rawConn.Close()
}

func (c *h2StreamConn) LocalAddr() net.Addr                { return c.rawConn.LocalAddr() }
func (c *h2StreamConn) RemoteAddr() net.Addr               { return c.rawConn.RemoteAddr() }
func (c *h2StreamConn) SetDeadline(t time.Time) error      { return c.rawConn.SetDeadline(t) }
func (c *h2StreamConn) SetReadDeadline(t time.Time) error  { return c.rawConn.SetReadDeadline(t) }
func (c *h2StreamConn) SetWriteDeadline(t time.Time) error { return c.rawConn.SetWriteDeadline(t) }

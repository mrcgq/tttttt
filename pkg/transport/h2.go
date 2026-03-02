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

// H2Transport 通过 HTTP/1.1 POST + Chunked 传输建立流式隧道。
// 虽然名为 "h2"，但实际使用 HTTP/1.1 协议以确保与 Cloudflare Workers 兼容。
// TLS 指纹控制仍通过 utls ClientHello 实现。
type H2Transport struct{}

func (t *H2Transport) Name() string { return "h2" }

// ALPNProtos 只声明 http/1.1，避免被协商为 h2 后发生协议不匹配
func (t *H2Transport) ALPNProtos() []string { return []string{"http/1.1"} }

func (t *H2Transport) Info() TransportInfo {
	return TransportInfo{
		SupportsMultiplex: false,
		SupportsBinary:    true,
		RequiresUpgrade:   false,
		MaxFrameSize:      65536,
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
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
			"(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
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

	// 构建 HTTP/1.1 POST 请求
	fmt.Fprintf(&reqBuf, "POST %s HTTP/1.1\r\n", c.path)
	fmt.Fprintf(&reqBuf, "Host: %s\r\n", c.host)
	fmt.Fprintf(&reqBuf, "User-Agent: %s\r\n", c.ua)
	fmt.Fprintf(&reqBuf, "Content-Type: application/octet-stream\r\n")
	fmt.Fprintf(&reqBuf, "Transfer-Encoding: chunked\r\n")
	fmt.Fprintf(&reqBuf, "Connection: keep-alive\r\n")
	fmt.Fprintf(&reqBuf, "Accept: */*\r\n")
	fmt.Fprintf(&reqBuf, "Cache-Control: no-cache\r\n")

	// 自定义头部
	for k, v := range c.headers {
		fmt.Fprintf(&reqBuf, "%s: %s\r\n", k, v)
	}
	fmt.Fprintf(&reqBuf, "\r\n")

	// 发送请求头
	if _, err := c.rawConn.Write(reqBuf.Bytes()); err != nil {
		return fmt.Errorf("h2: write request headers: %w", err)
	}

	// 发送目标地址作为第一个 chunk
	if c.target != "" {
		targetLine := c.target + "\n"
		chunk := fmt.Sprintf("%x\r\n%s\r\n", len(targetLine), targetLine)
		if _, err := c.rawConn.Write([]byte(chunk)); err != nil {
			return fmt.Errorf("h2: write target chunk: %w", err)
		}
	}

	// 创建响应读取器
	c.respReader = bufio.NewReaderSize(c.rawConn, 32768)

	// 读取响应头
	resp, err := http.ReadResponse(c.respReader, nil)
	if err != nil {
		return fmt.Errorf("h2: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("h2: server returned %s", resp.Status)
	}

	// 响应体会通过 respReader 继续读取
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

	// 写入 chunked 格式
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

	// 发送结束 chunk
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

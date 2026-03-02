package transport

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	// 必须引入项目内置的 H2 引擎，绝不能用标准库
	"github.com/user/tls-client/internal/h2"
)

type H2Transport struct{}

func (t *H2Transport) Name() string         { return "h2" }
func (t *H2Transport) ALPNProtos() []string { return[]string{"h2", "http/1.1"} }

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
	cfg.Normalize()

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

	// 吸收了你的优秀思路：检测实际协商的协议
	negotiatedProto := ""
	if tlsConn, ok := conn.(*tls.Conn); ok {
		negotiatedProto = tlsConn.ConnectionState().NegotiatedProtocol
	}

	if negotiatedProto == "h2" {
		return newH2StreamConn(conn, cfg, host, path, ua)
	}

	// 回退到 HTTP/1.1 chunked
	return newH1ChunkedConn(conn, cfg, host, path, ua)
}

// =============================================================================
// HTTP/2 真正实现 (融合了 prefixReader 和 internal/h2)
// =============================================================================

type h2StreamConn struct {
	rawConn  net.Conn
	pr       *io.PipeReader
	pw       *io.PipeWriter
	respBody io.ReadCloser

	readyCh   chan struct{}
	initErr   error
	closeOnce sync.Once
	closeMu   sync.Mutex
	closed    bool
}

func newH2StreamConn(conn net.Conn, cfg *Config, host, path, ua string) (*h2StreamConn, error) {
	pr, pw := io.Pipe()

	c := &h2StreamConn{
		rawConn: conn,
		pr:      pr,
		pw:      pw,
		readyCh: make(chan struct{}),
	}

	// 启动 HTTP/2 请求
	go c.doH2Request(cfg, host, path, ua)

	return c, nil
}

func (c *h2StreamConn) doH2Request(cfg *Config, host, path, ua string) {
	defer func() {
		select {
		case <-c.readyCh:
		default:
			close(c.readyCh)
		}
	}()

	// 1. 核心修复：使用项目自带的伪装 H2 客户端
	fp := h2.ChromeDefaultConfig() // 强行注入 Chrome 指纹
	client, err := h2.NewClient(c.rawConn, &fp)
	if err != nil {
		c.initErr = fmt.Errorf("h2: create custom client: %w", err)
		return
	}

	// 2. 你的优秀代码：使用 prefixReader 注入 Target
	var bodyReader io.Reader = c.pr
	if cfg.Target != "" {
		bodyReader = newPrefixReader(cfg.Target+"\n", c.pr)
	}

	url := fmt.Sprintf("https://%s%s", host, path)
	req, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		c.initErr = fmt.Errorf("h2: create request: %w", err)
		return
	}

	req.Header.Set("User-Agent", ua)
	req.Header.Set("Content-Type", "application/octet-stream")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	// 3. 执行指纹伪装的 H2 请求
	resp, err := client.Do(req)
	if err != nil {
		c.initErr = fmt.Errorf("h2: request failed: %w", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		c.initErr = fmt.Errorf("h2: server returned %s", resp.Status)
		return
	}

	c.respBody = resp.Body
	close(c.readyCh)
}

func (c *h2StreamConn) Read(p[]byte) (int, error) {
	<-c.readyCh
	if c.initErr != nil {
		return 0, c.initErr
	}
	if c.respBody == nil {
		return 0, io.EOF
	}
	return c.respBody.Read(p)
}

func (c *h2StreamConn) Write(p[]byte) (int, error) {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return 0, io.ErrClosedPipe
	}
	c.closeMu.Unlock()

	return c.pw.Write(p)
}

func (c *h2StreamConn) Close() error {
	c.closeOnce.Do(func() {
		c.closeMu.Lock()
		c.closed = true
		c.closeMu.Unlock()

		c.pw.Close()
		if c.respBody != nil {
			c.respBody.Close()
		}
		c.rawConn.Close()
	})
	return nil
}

func (c *h2StreamConn) LocalAddr() net.Addr                { return c.rawConn.LocalAddr() }
func (c *h2StreamConn) RemoteAddr() net.Addr               { return c.rawConn.RemoteAddr() }
func (c *h2StreamConn) SetDeadline(t time.Time) error      { return c.rawConn.SetDeadline(t) }
func (c *h2StreamConn) SetReadDeadline(t time.Time) error  { return c.rawConn.SetReadDeadline(t) }
func (c *h2StreamConn) SetWriteDeadline(t time.Time) error { return c.rawConn.SetWriteDeadline(t) }

// =============================================================================
// HTTP/1.1 Chunked 回退实现 (完全保留你的优秀代码)
// =============================================================================
type h1ChunkedConn struct {
	rawConn    net.Conn
	host       string
	path       string
	target     string
	ua         string
	headers    map[string]string
	initOnce   *sync.Once
	initErr    error
	respReader *bufio.Reader
	writeMu    sync.Mutex
	closed     bool
	closeMu    sync.Mutex
}

func newH1ChunkedConn(conn net.Conn, cfg *Config, host, path, ua string) (*h1ChunkedConn, error) {
	return &h1ChunkedConn{
		rawConn:  conn,
		host:     host,
		path:     path,
		target:   cfg.Target,
		ua:       ua,
		headers:  cfg.Headers,
		initOnce: &sync.Once{},
	}, nil
}

func (c *h1ChunkedConn) init() {
	c.initOnce.Do(func() {
		c.initErr = c.doInit()
	})
}

func (c *h1ChunkedConn) doInit() error {
	var reqBuf[]byte
	reqBuf = append(reqBuf, fmt.Sprintf("POST %s HTTP/1.1\r\n", c.path)...)
	reqBuf = append(reqBuf, fmt.Sprintf("Host: %s\r\n", c.host)...)
	reqBuf = append(reqBuf, fmt.Sprintf("User-Agent: %s\r\n", c.ua)...)
	reqBuf = append(reqBuf, "Content-Type: application/octet-stream\r\n"...)
	reqBuf = append(reqBuf, "Transfer-Encoding: chunked\r\n"...)
	reqBuf = append(reqBuf, "Connection: keep-alive\r\n"...)

	for k, v := range c.headers {
		reqBuf = append(reqBuf, fmt.Sprintf("%s: %s\r\n", k, v)...)
	}
	reqBuf = append(reqBuf, "\r\n"...)

	if _, err := c.rawConn.Write(reqBuf); err != nil {
		return fmt.Errorf("h1: write request headers: %w", err)
	}

	if c.target != "" {
		targetLine := c.target + "\n"
		chunk := fmt.Sprintf("%x\r\n%s\r\n", len(targetLine), targetLine)
		if _, err := c.rawConn.Write([]byte(chunk)); err != nil {
			return fmt.Errorf("h1: write target chunk: %w", err)
		}
	}

	c.respReader = bufio.NewReader(c.rawConn)
	resp, err := http.ReadResponse(c.respReader, nil)
	if err != nil {
		return fmt.Errorf("h1: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Body.Close()
		return fmt.Errorf("h1: server returned %s", resp.Status)
	}

	return nil
}

func (c *h1ChunkedConn) Read(p[]byte) (int, error) {
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

func (c *h1ChunkedConn) Write(p[]byte) (int, error) {
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

func (c *h1ChunkedConn) Close() error {
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

func (c *h1ChunkedConn) LocalAddr() net.Addr                { return c.rawConn.LocalAddr() }
func (c *h1ChunkedConn) RemoteAddr() net.Addr               { return c.rawConn.RemoteAddr() }
func (c *h1ChunkedConn) SetDeadline(t time.Time) error      { return c.rawConn.SetDeadline(t) }
func (c *h1ChunkedConn) SetReadDeadline(t time.Time) error  { return c.rawConn.SetReadDeadline(t) }
func (c *h1ChunkedConn) SetWriteDeadline(t time.Time) error { return c.rawConn.SetWriteDeadline(t) }

// =============================================================================
// 辅助工具 (完全保留)
// =============================================================================
type prefixReader struct {
	prefix[]byte
	pos    int
	reader io.Reader
}

func newPrefixReader(prefix string, r io.Reader) *prefixReader {
	return &prefixReader{
		prefix:[]byte(prefix),
		reader: r,
	}
}

func (p *prefixReader) Read(buf[]byte) (int, error) {
	if p.pos < len(p.prefix) {
		n := copy(buf, p.prefix[p.pos:])
		p.pos += n
		return n, nil
	}
	return p.reader.Read(buf)
}

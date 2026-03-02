package transport

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	// 必须引入原作者写好的高级防检测 H2 引擎
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

	// 核心修复：不要做任何 conn 的类型检测，直接相信系统并建立 H2 隧道
	return newH2StreamConn(conn, cfg, host, path, ua)
}

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

	// 强制注入 Chrome 浏览器指纹！
	fp := h2.ChromeDefaultConfig()
	client, err := h2.NewClient(c.rawConn, &fp)
	if err != nil {
		c.initErr = fmt.Errorf("h2: create custom client: %w", err)
		return
	}

	// 巧妙利用 prefixReader，在数据的最开头塞入目标地址(Target)
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

	// 发起带有指纹伪装的 H2 请求
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

// ==================
// 优雅注入 Target 头的核心工具
// ==================
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

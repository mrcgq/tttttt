package transport

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/user/tls-client/internal/h2"
)

var (
	h2Mu      sync.Mutex
	h2Clients = make(map[string]*h2.Client)
)

type H2Transport struct{}

func (t *H2Transport) Name() string         { return "h2" }
func (t *H2Transport) ALPNProtos() []string { return[]string{"h2"} } // 只保留 h2

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

	h2Mu.Lock()
	client, ok := h2Clients[host]
	if ok && !client.IsClosed() {
		go conn.Close()
	} else {
		fp := h2.ChromeDefaultConfig()
		var err error
		client, err = h2.NewClient(conn, &fp)
		if err != nil {
			h2Mu.Unlock()
			return nil, fmt.Errorf("h2 transport init failed: %w", err)
		}
		h2Clients[host] = client
	}
	h2Mu.Unlock()

	return newH2StreamConn(client, conn, cfg, host, path, ua)
}

type h2StreamConn struct {
	client    *h2.Client
	dummyAddr net.Addr
	pr        *io.PipeReader
	pw        *io.PipeWriter
	respBody  io.ReadCloser

	readyCh   chan struct{}
	initErr   error
	closeOnce sync.Once
	closeMu   sync.Mutex
	closed    bool
}

func newH2StreamConn(client *h2.Client, dummyConn net.Conn, cfg *Config, host, path, ua string) (*h2StreamConn, error) {
	pr, pw := io.Pipe()

	c := &h2StreamConn{
		client:    client,
		dummyAddr: dummyConn.RemoteAddr(),
		pr:        pr,
		pw:        pw,
		readyCh:   make(chan struct{}),
	}

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

	var bodyReader io.Reader = c.pr
	if cfg.Target != "" {
		// Base64 编码 Target 地址，防止被识别
		encodedTarget := base64.StdEncoding.EncodeToString([]byte(cfg.Target))
		bodyReader = newPrefixReader(encodedTarget+"\n", c.pr)
	}

	url := fmt.Sprintf("https://%s%s", host, path)
	req, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		c.initErr = fmt.Errorf("h2 stream req create: %w", err)
		return
	}
	req.Header.Set("User-Agent", ua)
	// 伪装成普通的文本上传
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.initErr = fmt.Errorf("h2 stream failed: %w", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		c.initErr = fmt.Errorf("h2 proxy returned status: %s", resp.Status)
		return
	}

	c.respBody = resp.Body
	close(c.readyCh)
}

// 👑 自动 Base64 编码/解码的 Read 和 Write
func (c *h2StreamConn) Read(p []byte) (int, error) {
	<-c.readyCh
	if c.initErr != nil {
		return 0, c.initErr
	}
	if c.respBody == nil {
		return 0, io.EOF
	}
	// 将从 CF 收到的 Base64 数据流，实时解码成原始二进制数据
	decoder := base64.NewDecoder(base64.StdEncoding, c.respBody)
	return decoder.Read(p)
}

func (c *h2StreamConn) Write(p []byte) (int, error) {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return 0, io.ErrClosedPipe
	}
	c.closeMu.Unlock()

	// 将原始二进制数据，编码成 Base64 字符串再写入管道
	encoded := base64.StdEncoding.EncodeToString(p)
	_, err := c.pw.Write([]byte(encoded))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *h2StreamConn) CloseWrite() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if !c.closed {
		return c.pw.Close()
	}
	return nil
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
	})
	return nil
}

func (c *h2StreamConn) LocalAddr() net.Addr                { return c.dummyAddr }
func (c *h2StreamConn) RemoteAddr() net.Addr               { return c.dummyAddr }
func (c *h2StreamConn) SetDeadline(t time.Time) error      { return nil }
func (c *h2StreamConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *h2StreamConn) SetWriteDeadline(t time.Time) error { return nil }

type prefixReader struct {
	prefix []byte
	pos    int
	reader io.Reader
}

func newPrefixReader(prefix string, r io.Reader) *prefixReader {
	return &prefixReader{
		prefix: []byte(prefix),
		reader: r,
	}
}

func (p *prefixReader) Read(buf []byte) (int, error) {
	if p.pos < len(p.prefix) {
		n := copy(buf, p.prefix[p.pos:])
		p.pos += n
		return n, nil
	}
	return p.reader.Read(buf)
}

package transport

import (
	"encoding/binary"
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
func (t *H2Transport) ALPNProtos() []string { return[]string{"h2"} }

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

	// 👑 终极二进制协议构造
	var bodyReader io.Reader
	if cfg.Target != "" {
		targetBytes := []byte(cfg.Target)
		lenBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBytes, uint16(len(targetBytes)))

		// 构造一个 Reader，它会先发送长度，再发送地址，最后发送真实数据流
		bodyReader = io.MultiReader(
			bytes.NewReader(lenBytes),
			bytes.NewReader(targetBytes),
			c.pr,
		)
	} else {
		bodyReader = c.pr
	}

	url := fmt.Sprintf("https://%s%s", host, path)
	req, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		c.initErr = fmt.Errorf("h2 stream req create: %w", err)
		return
	}

	req.Header.Set("User-Agent", ua)
	req.Header.Set("Content-Type", "application/octet-stream") // 改回二进制流
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
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

func (c *h2StreamConn) Read(p []byte) (int, error) {
	<-c.readyCh
	if c.initErr != nil {
		return 0, c.initErr
	}
	if c.respBody == nil {
		return 0, io.EOF
	}
	return c.respBody.Read(p)
}

func (c *h2StreamConn) Write(p []byte) (int, error) {
	return c.pw.Write(p)
}

func (c *h2StreamConn) CloseWrite() error {
	return c.pw.Close()
}

func (c *h2StreamConn) Close() error {
	c.closeOnce.Do(func() {
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

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

var (
	// 全局管理器：缓存每个节点 IP 对应的 H2 多路复用客户端
	h2Mu      sync.Mutex
	h2Clients = make(map[string]*h2.Client)
)

type H2Transport struct{}

func (t *H2Transport) Name() string         { return "h2" }
func (t *H2Transport) ALPNProtos() []string { return[]string{"h2", "http/1.1"} }

func (t *H2Transport) Info() TransportInfo {
	return TransportInfo{
		SupportsMultiplex: true, // 我们现在真正支持了多路复用！
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

	// 👑 核心修复：完美的 HTTP/2 连接复用机制
	h2Mu.Lock()
	client, ok := h2Clients[host]
	if ok && !client.IsClosed() {
		// 如果已经有一根通向 CF 的活跃 H2 连接，直接复用它！
		// 释放外层传入的冗余 TCP 连接，防止连接池爆炸
		go conn.Close()
	} else {
		// 如果没有，或者连接已断开，建立一条全新的隐蔽 H2 隧道
		fp := h2.ChromeDefaultConfig() // 强制注入 Chrome 指纹
		var err error
		client, err = h2.NewClient(conn, &fp)
		if err != nil {
			h2Mu.Unlock()
			return nil, fmt.Errorf("h2 transport init failed: %w", err)
		}
		h2Clients[host] = client
	}
	h2Mu.Unlock()

	// 把代理请求包装成这根 H2 隧道里的一个 Stream (流)
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
		dummyAddr: dummyConn.RemoteAddr(), // 仅用于实现接口，不实际使用
		pr:        pr,
		pw:        pw,
		readyCh:   make(chan struct{}),
	}

	// 异步启动 H2 数据流
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
		// 在请求体最开头注入目标地址
		bodyReader = newPrefixReader(cfg.Target+"\n", c.pr)
	}

	url := fmt.Sprintf("https://%s%s", host, path)
	req, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		c.initErr = fmt.Errorf("h2 stream req create: %w", err)
		return
	}

	req.Header.Set("User-Agent", ua)
	req.Header.Set("Content-Type", "application/octet-stream")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	// 发送 HTTP/2 数据帧
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

// 👑 完美支持 Telegram 等代理软件的“半关闭” (Half-Close) 特性
func (c *h2StreamConn) CloseWrite() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if !c.closed {
		// 发送 HTTP/2 的 END_STREAM 标志，通知 CF Worker 上传结束
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
		// ⚠️ 极其关键：绝对不能在这里关闭底层的 TCP 连接！
		// 因为这是 H2 的多路复用连接，其他代理请求还要继续用它！
	})
	return nil
}

func (c *h2StreamConn) LocalAddr() net.Addr                { return c.dummyAddr }
func (c *h2StreamConn) RemoteAddr() net.Addr               { return c.dummyAddr }
func (c *h2StreamConn) SetDeadline(t time.Time) error      { return nil }
func (c *h2StreamConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *h2StreamConn) SetWriteDeadline(t time.Time) error { return nil }

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

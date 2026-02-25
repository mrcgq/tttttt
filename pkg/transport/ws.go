
package transport

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const wsMagicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// WSTransport wraps a TLS connection with WebSocket framing.
//
// Use this transport when:
// - Connecting through Cloudflare Workers (primary use case)
// - The network path performs HTTP-level inspection
// - You need the connection to look like a WebSocket session
type WSTransport struct{}

func (t *WSTransport) Name() string         { return "ws" }
func (t *WSTransport) ALPNProtos() []string { return []string{"http/1.1"} }

func (t *WSTransport) Info() TransportInfo {
	return TransportInfo{
		SupportsMultiplex: false,
		SupportsBinary:    true,
		RequiresUpgrade:   true,
		MaxFrameSize:      16384,
	}
}

func (t *WSTransport) Wrap(conn net.Conn, cfg *Config) (net.Conn, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	cfg.Normalize()

	path := cfg.Path
	if path == "" {
		path = "/"
	}
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, fmt.Errorf("ws: generate key: %w", err)
	}
	wsKey := base64.StdEncoding.EncodeToString(keyBytes)

	// Build browser-realistic HTTP upgrade request
	reqStr := fmt.Sprintf("GET %s HTTP/1.1\r\n", path)
	reqStr += fmt.Sprintf("Host: %s\r\n", host)
	reqStr += "Upgrade: websocket\r\n"
	reqStr += "Connection: Upgrade\r\n"
	reqStr += fmt.Sprintf("Sec-WebSocket-Key: %s\r\n", wsKey)
	reqStr += "Sec-WebSocket-Version: 13\r\n"
	// Browser-realistic headers to avoid detection
	reqStr += "Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits\r\n"
	if cfg.UserAgent != "" {
		reqStr += fmt.Sprintf("User-Agent: %s\r\n", cfg.UserAgent)
	}
	reqStr += fmt.Sprintf("Origin: https://%s\r\n", host)
	reqStr += "Accept-Language: en-US,en;q=0.9\r\n"
	reqStr += "Accept-Encoding: gzip, deflate, br\r\n"
	reqStr += "Pragma: no-cache\r\n"
	reqStr += "Cache-Control: no-cache\r\n"
	for k, v := range cfg.Headers {
		reqStr += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	reqStr += "\r\n"

	if _, err := conn.Write([]byte(reqStr)); err != nil {
		return nil, fmt.Errorf("ws: send upgrade: %w", err)
	}

	br := bufio.NewReaderSize(conn, 4096)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		return nil, fmt.Errorf("ws: read upgrade response: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("ws: upgrade failed: %s", resp.Status)
	}

	expectedAccept := computeAcceptKey(wsKey)
	if resp.Header.Get("Sec-WebSocket-Accept") != expectedAccept {
		return nil, fmt.Errorf("ws: invalid Sec-WebSocket-Accept")
	}

	ws := newWSConn(conn, br)

	// 启动心跳
	go ws.keepAlive()

	return ws, nil
}

func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte(wsMagicGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// wsConn wraps a connection with WebSocket binary framing.
type wsConn struct {
	conn    net.Conn
	br      *bufio.Reader
	writeMu sync.Mutex

	// 分片帧状态
	fragmentBuf []byte
	fragmenting bool

	// 心跳
	lastPong  atomic.Int64
	closeCh   chan struct{}
	closeOnce sync.Once

	// Read state
	readBuf []byte
	readEOF bool
}

func newWSConn(conn net.Conn, br *bufio.Reader) *wsConn {
	c := &wsConn{
		conn:    conn,
		br:      br,
		closeCh: make(chan struct{}),
	}
	c.lastPong.Store(time.Now().UnixNano())
	return c
}

func (c *wsConn) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeCh:
			return
		case <-ticker.C:
			// 检查最后一次 pong 时间
			lastPong := time.Unix(0, c.lastPong.Load())
			if time.Since(lastPong) > 90*time.Second {
				c.Close()
				return
			}

			// 发送 ping
			c.writeMu.Lock()
			pingData := make([]byte, 8)
			rand.Read(pingData)
			writeFrame(c.conn, 0x09, pingData) // ping 是控制帧，始终 FIN=1
			c.writeMu.Unlock()
		}
	}
}

func (c *wsConn) Read(p []byte) (int, error) {
	if len(c.readBuf) > 0 {
		n := copy(p, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}
	if c.readEOF {
		return 0, io.EOF
	}

	for {
		// [BUG-1 FIX] 使用修复后的 readFrame，正确获取 FIN 位
		opcode, payload, fin, err := readFrame(c.br)
		if err != nil {
			c.readEOF = true
			return 0, err
		}

		switch opcode {
		case 0x00: // continuation frame
			if c.fragmenting {
				c.fragmentBuf = append(c.fragmentBuf, payload...)
				// [BUG-1 FIX] 只有 FIN=1 时才完成重组
				if fin {
					c.fragmenting = false
					payload = c.fragmentBuf
					c.fragmentBuf = nil
					// 继续到下方返回 payload
				} else {
					// 还有后续分片，继续读
					continue
				}
			} else {
				// 收到 continuation 但没在分片中，忽略
				continue
			}

		case 0x01, 0x02: // text, binary
			// [BUG-1 FIX] FIN=0 表示这是分片消息的第一帧
			if !fin {
				c.fragmenting = true
				c.fragmentBuf = append([]byte(nil), payload...)
				continue
			}
			// FIN=1，完整的单帧消息，直接返回

		case 0x08: // close
			c.readEOF = true
			// Echo close frame back (per RFC 6455)
			c.writeMu.Lock()
			WriteCloseFrame(c.conn, 1000)
			c.writeMu.Unlock()
			return 0, io.EOF

		case 0x09: // ping — 控制帧总是 FIN=1
			c.writeMu.Lock()
			writeFrame(c.conn, 0x0A, payload) // pong
			c.writeMu.Unlock()
			continue

		case 0x0A: // pong
			c.lastPong.Store(time.Now().UnixNano())
			continue

		default:
			continue
		}

		if len(payload) == 0 {
			continue
		}

		n := copy(p, payload)
		if n < len(payload) {
			c.readBuf = make([]byte, len(payload)-n)
			copy(c.readBuf, payload[n:])
		}
		return n, nil
	}
}

func (c *wsConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// [BUG-5 FIX] 完整修复分片发送的 FIN/opcode 控制
	//
	// RFC 6455 分片规则：
	//   第一帧：opcode=实际类型(0x02)，FIN=0（如果有后续帧）
	//   中间帧：opcode=0x00(continuation)，FIN=0
	//   最后帧：opcode=0x00(continuation)，FIN=1
	//   单帧消息：opcode=实际类型(0x02)，FIN=1

	const maxFrameSize = 16384
	total := 0
	remaining := p
	isFirstFrame := true

	for len(remaining) > 0 {
		chunk := remaining
		isLastFrame := true

		if len(chunk) > maxFrameSize {
			chunk = chunk[:maxFrameSize]
			isLastFrame = false
		}

		var opcode byte
		var fin bool

		if isFirstFrame && isLastFrame {
			// 单帧消息：opcode=0x02, FIN=1
			opcode = 0x02
			fin = true
		} else if isFirstFrame && !isLastFrame {
			// 分片第一帧：opcode=0x02, FIN=0
			opcode = 0x02
			fin = false
		} else if !isFirstFrame && isLastFrame {
			// 分片最后帧：opcode=0x00, FIN=1
			opcode = 0x00
			fin = true
		} else {
			// 分片中间帧：opcode=0x00, FIN=0
			opcode = 0x00
			fin = false
		}

		// [BUG-5 FIX] 使用 writeFrameBytes 独立控制 FIN 位
		n, err := writeFrameBytes(c.conn, fin, opcode, chunk)
		if err != nil {
			return total, err
		}
		total += n
		remaining = remaining[len(chunk):]
		isFirstFrame = false
	}

	return total, nil
}

func (c *wsConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closeCh)
		c.writeMu.Lock()
		WriteCloseFrame(c.conn, 1000) // 正常关闭
		c.writeMu.Unlock()
	})
	return c.conn.Close()
}

func (c *wsConn) LocalAddr() net.Addr                { return c.conn.LocalAddr() }
func (c *wsConn) RemoteAddr() net.Addr               { return c.conn.RemoteAddr() }
func (c *wsConn) SetDeadline(t time.Time) error      { return c.conn.SetDeadline(t) }
func (c *wsConn) SetReadDeadline(t time.Time) error  { return c.conn.SetReadDeadline(t) }
func (c *wsConn) SetWriteDeadline(t time.Time) error { return c.conn.SetWriteDeadline(t) }

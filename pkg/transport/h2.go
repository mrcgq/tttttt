package transport

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/user/tls-client/internal/h2"
	"golang.org/x/net/http2/hpack"
)

// H2Transport wraps a TLS connection using an HTTP/2 stream.
//
// Supports two modes:
//
//   - Proxy mode (CONNECT): When Target is set or Path is empty/root,
//     sends HTTP/2 CONNECT request. The CONNECT method is HPACK-compressed
//     into binary frames, making it indistinguishable from normal browser
//     HTTP/2 traffic. This defeats DPI keyword matching and length analysis.
//
//   - Tunnel mode (POST): When Path is set to a specific endpoint,
//     sends POST request (for Cloudflare Workers or similar).
type H2Transport struct{}

func (t *H2Transport) Name() string         { return "h2" }
func (t *H2Transport) ALPNProtos() []string { return []string{"h2"} }

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

	// [SECURITY FIX] 判断模式：
	//   - 有 Target 或 Path 为空/根路径 → 代理模式 (CONNECT)
	//   - 有具体 Path (如 /tunnel) → 隧道模式 (POST)
	//
	// 代理模式将 CONNECT 请求封装在 HPACK 压缩的 HEADERS 帧中，
	// 彻底消除明文 "CONNECT xxx:443 HTTP/1.1" 的流量特征
	isProxyMode := cfg.Target != "" || cfg.Path == "" || cfg.Path == "/"

	path := cfg.Path
	if !isProxyMode && path == "" {
		path = "/tunnel"
	}

	fpCfg := h2.GoDefaultConfig()
	preface := h2.BuildPreface(&fpCfg)
	if _, err := conn.Write(preface); err != nil {
		return nil, fmt.Errorf("h2transport: write preface: %w", err)
	}

	return &h2StreamConn{
		conn:           conn,
		host:           host,
		path:           path,
		target:         cfg.Target,
		ua:             cfg.UserAgent,
		isProxyMode:    isProxyMode,
		sendWindow:     65535,
		connSendWindow: 65535,
		streamID:       1,
	}, nil
}

// h2StreamConn is a simplified HTTP/2 bidirectional stream connection.
type h2StreamConn struct {
	conn      net.Conn
	host      string
	path      string
	target    string // 代理目标地址 (CONNECT 模式)
	ua        string
	writeMu   sync.Mutex
	readBuf   []byte
	initiated bool
	streamID  uint32

	// 模式标识
	isProxyMode bool

	// 流控
	sendWindow     int64
	connSendWindow int64
	windowMu       sync.Mutex
}

func (c *h2StreamConn) initStream() error {
	if c.initiated {
		return nil
	}
	c.initiated = true

	// 读取服务端 SETTINGS 帧
	serverFrame := make([]byte, 9)
	if _, err := io.ReadFull(c.conn, serverFrame); err != nil {
		return fmt.Errorf("h2transport: read server settings header: %w", err)
	}
	frameLen := int(serverFrame[0])<<16 | int(serverFrame[1])<<8 | int(serverFrame[2])
	if frameLen > 0 {
		settingsPayload := make([]byte, frameLen)
		if _, err := io.ReadFull(c.conn, settingsPayload); err != nil {
			return fmt.Errorf("h2transport: read server settings payload: %w", err)
		}
		c.parseSettings(settingsPayload)
	}

	// 发送 SETTINGS ACK
	ack := []byte{0, 0, 0, 0x04, 0x01, 0, 0, 0, 0}
	if _, err := c.conn.Write(ack); err != nil {
		return fmt.Errorf("h2transport: write settings ack: %w", err)
	}

	// 发送 HEADERS 帧
	if err := c.sendHeaders(); err != nil {
		return fmt.Errorf("h2transport: send headers: %w", err)
	}

	// [SECURITY FIX] 代理模式需要读取服务端的 2xx 响应
	if c.isProxyMode {
		if err := c.readConnectResponse(); err != nil {
			return fmt.Errorf("h2transport: connect response: %w", err)
		}
	}

	return nil
}

func (c *h2StreamConn) parseSettings(payload []byte) {
	for len(payload) >= 6 {
		id := uint16(payload[0])<<8 | uint16(payload[1])
		val := uint32(payload[2])<<24 | uint32(payload[3])<<16 | uint32(payload[4])<<8 | uint32(payload[5])
		payload = payload[6:]

		if id == 0x04 { // SETTINGS_INITIAL_WINDOW_SIZE
			c.windowMu.Lock()
			atomic.StoreInt64(&c.sendWindow, int64(val))
			atomic.StoreInt64(&c.connSendWindow, int64(val))
			c.windowMu.Unlock()
		}
	}
}

func (c *h2StreamConn) sendHeaders() error {
	var hpackBuf bytes.Buffer
	enc := hpack.NewEncoder(&hpackBuf)

	if c.isProxyMode {
		// [SECURITY FIX] --- 代理模式：标准的 HTTP/2 CONNECT ---
		//
		// RFC 7540 Section 8.3 规定：
		//   - CONNECT 方法的 :authority 应为目标地址（host:port）
		//   - 不应该包含 :scheme 和 :path 伪头部
		//
		// 关键安全优势：
		//   - "CONNECT google.com:443" 被 HPACK 压缩为二进制
		//   - 由于 HPACK 动态表，每次请求的字节序列可能不同
		//   - GFW 的 DPI 无法通过关键词匹配或长度特征识别
		authority := c.target
		if authority == "" {
			authority = c.host
		}
		enc.WriteField(hpack.HeaderField{Name: ":method", Value: "CONNECT"})
		enc.WriteField(hpack.HeaderField{Name: ":authority", Value: authority})
	} else {
		// --- 隧道模式：Worker POST ---
		enc.WriteField(hpack.HeaderField{Name: ":method", Value: "POST"})
		enc.WriteField(hpack.HeaderField{Name: ":authority", Value: c.host})
		enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "https"})
		enc.WriteField(hpack.HeaderField{Name: ":path", Value: c.path})
	}

	if c.ua != "" {
		enc.WriteField(hpack.HeaderField{Name: "user-agent", Value: c.ua})
	}

	// 隧道模式才需要这些头（模拟 gRPC）
	if !c.isProxyMode {
		enc.WriteField(hpack.HeaderField{Name: "content-type", Value: "application/grpc"})
		enc.WriteField(hpack.HeaderField{Name: "te", Value: "trailers"})
	}

	headerBlock := hpackBuf.Bytes()

	frame := make([]byte, 9+len(headerBlock))
	frame[0] = byte(len(headerBlock) >> 16)
	frame[1] = byte(len(headerBlock) >> 8)
	frame[2] = byte(len(headerBlock))
	frame[3] = 0x01 // type = HEADERS
	frame[4] = 0x04 // flags = END_HEADERS
	frame[5] = 0
	frame[6] = 0
	frame[7] = 0
	frame[8] = byte(c.streamID)

	copy(frame[9:], headerBlock)

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := c.conn.Write(frame)
	return err
}

// readConnectResponse 读取 CONNECT 请求的响应头
// HTTP/2 代理服务器应该返回一个 2xx 状态码的 HEADERS 帧
func (c *h2StreamConn) readConnectResponse() error {
	for {
		header := make([]byte, 9)
		if _, err := io.ReadFull(c.conn, header); err != nil {
			return fmt.Errorf("read response header: %w", err)
		}

		frameLen := int(header[0])<<16 | int(header[1])<<8 | int(header[2])
		frameType := header[3]
		streamID := uint32(header[5])<<24 | uint32(header[6])<<16 | uint32(header[7])<<8 | uint32(header[8])
		streamID &= 0x7FFFFFFF

		payload := make([]byte, frameLen)
		if frameLen > 0 {
			if _, err := io.ReadFull(c.conn, payload); err != nil {
				return fmt.Errorf("read response payload: %w", err)
			}
		}

		switch frameType {
		case 0x01: // HEADERS
			if streamID == c.streamID {
				// 解析状态码
				decoder := hpack.NewDecoder(4096, nil)
				fields, err := decoder.DecodeFull(payload)
				if err != nil {
					return fmt.Errorf("decode headers: %w", err)
				}

				for _, f := range fields {
					if f.Name == ":status" {
						if !strings.HasPrefix(f.Value, "2") {
							return fmt.Errorf("CONNECT rejected: %s", f.Value)
						}
						// 2xx 响应，隧道建立成功
						return nil
					}
				}
				return fmt.Errorf("no :status in response")
			}

		case 0x04: // SETTINGS
			if header[4]&0x01 == 0 { // 不是 ACK
				c.parseSettings(payload)
				ack := []byte{0, 0, 0, 0x04, 0x01, 0, 0, 0, 0}
				c.writeMu.Lock()
				c.conn.Write(ack)
				c.writeMu.Unlock()
			}

		case 0x06: // PING
			if header[4]&0x01 == 0 { // 不是 ACK
				pong := make([]byte, 9+8)
				copy(pong, []byte{0, 0, 8, 0x06, 0x01, 0, 0, 0, 0})
				copy(pong[9:], payload)
				c.writeMu.Lock()
				c.conn.Write(pong)
				c.writeMu.Unlock()
			}

		case 0x07: // GOAWAY
			return fmt.Errorf("received GOAWAY")

		case 0x03: // RST_STREAM
			return fmt.Errorf("stream reset")

		case 0x08: // WINDOW_UPDATE
			if frameLen >= 4 {
				increment := uint32(payload[0])<<24 | uint32(payload[1])<<16 | uint32(payload[2])<<8 | uint32(payload[3])
				increment &= 0x7FFFFFFF
				c.windowMu.Lock()
				if streamID == 0 {
					atomic.AddInt64(&c.connSendWindow, int64(increment))
				} else if streamID == c.streamID {
					atomic.AddInt64(&c.sendWindow, int64(increment))
				}
				c.windowMu.Unlock()
			}
		}
	}
}

func (c *h2StreamConn) Read(p []byte) (int, error) {
	if err := c.initStream(); err != nil {
		return 0, err
	}

	if len(c.readBuf) > 0 {
		n := copy(p, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}

	for {
		header := make([]byte, 9)
		if _, err := io.ReadFull(c.conn, header); err != nil {
			return 0, err
		}

		frameLen := int(header[0])<<16 | int(header[1])<<8 | int(header[2])
		frameType := header[3]
		streamID := uint32(header[5])<<24 | uint32(header[6])<<16 | uint32(header[7])<<8 | uint32(header[8])

		payload := make([]byte, frameLen)
		if frameLen > 0 {
			if _, err := io.ReadFull(c.conn, payload); err != nil {
				return 0, err
			}
		}

		switch frameType {
		case 0x00: // DATA
			if len(payload) == 0 {
				continue
			}
			n := copy(p, payload)
			if n < len(payload) {
				c.readBuf = make([]byte, len(payload)-n)
				copy(c.readBuf, payload[n:])
			}
			c.sendWindowUpdate(0, uint32(len(payload)))
			c.sendWindowUpdate(c.streamID, uint32(len(payload)))
			return n, nil

		case 0x01: // HEADERS (response headers, skip)
			continue

		case 0x04: // SETTINGS
			if header[4]&0x01 == 0 {
				c.parseSettings(payload)
				ack := []byte{0, 0, 0, 0x04, 0x01, 0, 0, 0, 0}
				c.writeMu.Lock()
				c.conn.Write(ack)
				c.writeMu.Unlock()
			}
			continue

		case 0x06: // PING
			if header[4]&0x01 == 0 {
				pong := make([]byte, 9+8)
				copy(pong, []byte{0, 0, 8, 0x06, 0x01, 0, 0, 0, 0})
				copy(pong[9:], payload)
				c.writeMu.Lock()
				c.conn.Write(pong)
				c.writeMu.Unlock()
			}
			continue

		case 0x07: // GOAWAY
			return 0, io.EOF

		case 0x03: // RST_STREAM
			return 0, io.EOF

		case 0x08: // WINDOW_UPDATE
			if frameLen >= 4 {
				increment := uint32(payload[0])<<24 | uint32(payload[1])<<16 | uint32(payload[2])<<8 | uint32(payload[3])
				increment &= 0x7FFFFFFF

				c.windowMu.Lock()
				if streamID == 0 {
					atomic.AddInt64(&c.connSendWindow, int64(increment))
				} else if streamID == c.streamID {
					atomic.AddInt64(&c.sendWindow, int64(increment))
				}
				c.windowMu.Unlock()
			}
			continue

		default:
			continue
		}
	}
}

// [BUG-3 FIX] Write 方法：用 time.Timer 轮询替代 goroutine 超时
func (c *h2StreamConn) Write(p []byte) (int, error) {
	if err := c.initStream(); err != nil {
		return 0, err
	}

	total := 0

	for len(p) > 0 {
		// [BUG-3 FIX] 用 deadline 轮询等待窗口可用，不启动 goroutine
		deadline := time.After(30 * time.Second)
		pollInterval := time.NewTicker(100 * time.Millisecond)

		var maxSend int64
		waitingForWindow := true

		for waitingForWindow {
			streamWin := atomic.LoadInt64(&c.sendWindow)
			connWin := atomic.LoadInt64(&c.connSendWindow)

			if streamWin > 0 && connWin > 0 {
				maxSend = streamWin
				if connWin < maxSend {
					maxSend = connWin
				}
				if maxSend > 16384 {
					maxSend = 16384
				}
				waitingForWindow = false
			} else {
				select {
				case <-deadline:
					pollInterval.Stop()
					return total, fmt.Errorf("h2transport: flow control timeout after 30s")
				case <-pollInterval.C:
					continue
				}
			}
		}

		pollInterval.Stop()

		chunk := p
		if int64(len(chunk)) > maxSend {
			chunk = chunk[:maxSend]
		}

		atomic.AddInt64(&c.sendWindow, -int64(len(chunk)))
		atomic.AddInt64(&c.connSendWindow, -int64(len(chunk)))

		header := make([]byte, 9)
		header[0] = byte(len(chunk) >> 16)
		header[1] = byte(len(chunk) >> 8)
		header[2] = byte(len(chunk))
		header[3] = 0x00 // type = DATA
		header[4] = 0x00 // flags = 0
		header[5] = 0
		header[6] = 0
		header[7] = 0
		header[8] = byte(c.streamID)

		c.writeMu.Lock()
		if _, err := c.conn.Write(header); err != nil {
			c.writeMu.Unlock()
			return total, err
		}
		if _, err := c.conn.Write(chunk); err != nil {
			c.writeMu.Unlock()
			return total, err
		}
		c.writeMu.Unlock()

		total += len(chunk)
		p = p[len(chunk):]
	}
	return total, nil
}

func (c *h2StreamConn) sendWindowUpdate(streamID, increment uint32) {
	if increment == 0 {
		return
	}
	frame := make([]byte, 9+4)
	frame[0] = 0
	frame[1] = 0
	frame[2] = 4
	frame[3] = 0x08 // WINDOW_UPDATE
	frame[4] = 0
	frame[5] = byte(streamID >> 24)
	frame[6] = byte(streamID >> 16)
	frame[7] = byte(streamID >> 8)
	frame[8] = byte(streamID)
	frame[9] = byte(increment >> 24)
	frame[10] = byte(increment >> 16)
	frame[11] = byte(increment >> 8)
	frame[12] = byte(increment)

	c.writeMu.Lock()
	c.conn.Write(frame)
	c.writeMu.Unlock()
}

func (c *h2StreamConn) Close() error                       { return c.conn.Close() }
func (c *h2StreamConn) LocalAddr() net.Addr                { return c.conn.LocalAddr() }
func (c *h2StreamConn) RemoteAddr() net.Addr               { return c.conn.RemoteAddr() }
func (c *h2StreamConn) SetDeadline(t time.Time) error      { return c.conn.SetDeadline(t) }
func (c *h2StreamConn) SetReadDeadline(t time.Time) error  { return c.conn.SetReadDeadline(t) }
func (c *h2StreamConn) SetWriteDeadline(t time.Time) error { return c.conn.SetWriteDeadline(t) }




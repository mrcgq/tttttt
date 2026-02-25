


package h2

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

var (
	ErrClientClosed = errors.New("h2: client closed")
	ErrStreamReset  = errors.New("h2: stream reset")
	ErrGoAway       = errors.New("h2: received GOAWAY")
	ErrFlowControl  = errors.New("h2: flow control violation")
)

const (
	stateActive = 0
	stateGoAway = 1
	stateClosed = 2
)

// ClientMetrics tracks connection-level metrics for observability.
type ClientMetrics struct {
	StreamsOpened  int64
	StreamsClosed  int64
	FramesSent     int64
	FramesReceived int64
	BytesRead      int64
	BytesWritten   int64
}

// Client is a custom HTTP/2 client that provides full control over
// SETTINGS, WINDOW_UPDATE, and pseudo-header ordering for fingerprinting.
type Client struct {
	conn   net.Conn
	framer *http2.Framer
	fp     *FingerprintConfig

	// HPACK encoder (for request headers)
	hpackBuf bytes.Buffer
	hpackEnc *hpack.Encoder

	// stream management
	nextStreamID uint32
	mu           sync.Mutex
	streams      map[uint32]*stream

	// write serialization
	writeMu sync.Mutex

	// connection state
	state        int32 // atomic: stateActive, stateGoAway, stateClosed
	closeOnce    sync.Once
	closeCh      chan struct{}
	lastStreamID uint32 // from GOAWAY frame

	// flow control
	connSendWindow int64
	serverInitWin  uint32 // server's INITIAL_WINDOW_SIZE

	// configuration
	responseTimeout time.Duration

	// metrics
	metrics ClientMetrics
}

type stream struct {
	id         uint32
	headers    chan *headerResult
	data       chan *dataChunk
	done       chan struct{}
	sendWindow int64 // per-stream send window

	// [BUG-2 FIX] sync.Once 保护 close(done)，防止 double close panic
	doneOnce sync.Once

	// [BUG-6 FIX] 标记 stream 是否已关闭，防止向已关闭的 channel 写入
	closed atomic.Bool
}

// closeDone safely closes the done channel exactly once.
// [BUG-2 FIX] 无论被调用多少次（handleResponseData / handleRSTStream / 超时），
// 都只会 close(done) 一次，永远不会 panic。
func (s *stream) closeDone() {
	s.doneOnce.Do(func() {
		s.closed.Store(true)
		close(s.done)
	})
}

// trySendHeaders attempts to send a header result to the stream.
// [BUG-6 FIX] 如果 stream 已关闭，静默丢弃，不会 panic。
func (s *stream) trySendHeaders(hr *headerResult) bool {
	if s.closed.Load() {
		return false
	}
	select {
	case s.headers <- hr:
		return true
	default:
		return false
	}
}

// trySendData attempts to send a data chunk to the stream.
// [BUG-6 FIX] 如果 stream 已关闭，静默丢弃，不会 panic。
func (s *stream) trySendData(chunk *dataChunk) bool {
	if s.closed.Load() {
		return false
	}
	select {
	case s.data <- chunk:
		return true
	default:
		return false
	}
}

type headerResult struct {
	status  int
	headers http.Header
	err     error
}

type dataChunk struct {
	data      []byte
	endStream bool
}

// NewClient creates a custom H2 client on an existing TLS connection.
func NewClient(conn net.Conn, fp *FingerprintConfig) (*Client, error) {
	c := &Client{
		conn:            conn,
		fp:              fp,
		nextStreamID:    1,
		streams:         make(map[uint32]*stream),
		closeCh:         make(chan struct{}),
		connSendWindow:  65535,
		serverInitWin:   65535,
		responseTimeout: 30 * time.Second,
	}

	// Send custom preface with fingerprint-matched parameters
	preface := BuildPreface(fp)
	if _, err := conn.Write(preface); err != nil {
		return nil, fmt.Errorf("h2: write preface: %w", err)
	}

	// Create framer for subsequent frames
	c.framer = http2.NewFramer(conn, conn)
	c.framer.SetMaxReadFrameSize(1 << 24)
	c.framer.ReadMetaHeaders = hpack.NewDecoder(4096, nil)
	c.framer.MaxHeaderListSize = 262144

	// Init HPACK encoder
	c.hpackEnc = hpack.NewEncoder(&c.hpackBuf)

	// Start read loop
	go c.readLoop()

	return c, nil
}

// SetResponseTimeout configures the maximum time to wait for response headers.
func (c *Client) SetResponseTimeout(d time.Duration) {
	c.responseTimeout = d
}

func (c *Client) readLoop() {
	defer c.closeInternal(nil)

	for {
		f, err := c.framer.ReadFrame()
		if err != nil {
			if atomic.LoadInt32(&c.state) == stateClosed {
				return
			}
			c.closeInternal(fmt.Errorf("h2: read frame: %w", err))
			return
		}

		atomic.AddInt64(&c.metrics.FramesReceived, 1)

		switch f := f.(type) {
		case *http2.SettingsFrame:
			if !f.IsAck() {
				c.handleServerSettings(f)
			}
		case *http2.PingFrame:
			if !f.IsAck() {
				c.writeMu.Lock()
				_ = c.framer.WritePing(true, f.Data)
				atomic.AddInt64(&c.metrics.FramesSent, 1)
				c.writeMu.Unlock()
			}
		case *http2.WindowUpdateFrame:
			c.handleWindowUpdate(f)
		case *http2.GoAwayFrame:
			atomic.StoreUint32(&c.lastStreamID, f.LastStreamID)
			atomic.StoreInt32(&c.state, stateGoAway)
			c.closeInternal(ErrGoAway)
			return
		case *http2.MetaHeadersFrame:
			c.handleResponseHeaders(f)
		case *http2.DataFrame:
			c.handleResponseData(f)
		case *http2.RSTStreamFrame:
			c.handleRSTStream(f)
		}
	}
}

func (c *Client) handleServerSettings(f *http2.SettingsFrame) {
	var newInitWin uint32
	hasNewInitWin := false

	_ = f.ForeachSetting(func(s http2.Setting) error {
		switch s.ID {
		case http2.SettingInitialWindowSize:
			newInitWin = s.Val
			hasNewInitWin = true
		case http2.SettingHeaderTableSize:
			c.mu.Lock()
			c.hpackEnc.SetMaxDynamicTableSizeLimit(s.Val)
			c.mu.Unlock()
		}
		return nil
	})

	// Update all existing stream windows when server changes INITIAL_WINDOW_SIZE
	if hasNewInitWin {
		c.mu.Lock()
		oldInitWin := c.serverInitWin
		c.serverInitWin = newInitWin
		delta := int64(newInitWin) - int64(oldInitWin)
		for _, s := range c.streams {
			atomic.AddInt64(&s.sendWindow, delta)
		}
		c.mu.Unlock()
	}

	// ACK the server settings
	c.writeMu.Lock()
	_ = c.framer.WriteSettingsAck()
	atomic.AddInt64(&c.metrics.FramesSent, 1)
	c.writeMu.Unlock()
}

func (c *Client) handleWindowUpdate(f *http2.WindowUpdateFrame) {
	if f.StreamID == 0 {
		atomic.AddInt64(&c.connSendWindow, int64(f.Increment))
		return
	}
	c.mu.Lock()
	s, ok := c.streams[f.StreamID]
	c.mu.Unlock()
	if ok {
		atomic.AddInt64(&s.sendWindow, int64(f.Increment))
	}
}

func (c *Client) handleResponseHeaders(f *http2.MetaHeadersFrame) {
	c.mu.Lock()
	s, ok := c.streams[f.StreamID]
	c.mu.Unlock()
	if !ok {
		return
	}

	hdr := make(http.Header)
	status := 200
	for _, field := range f.Fields {
		if field.Name == ":status" {
			status, _ = strconv.Atoi(field.Value)
		} else {
			hdr.Add(field.Name, field.Value)
		}
	}

	// [BUG-6 FIX] 使用 trySendHeaders 安全发送
	s.trySendHeaders(&headerResult{status: status, headers: hdr})

	if f.StreamEnded() {
		// [BUG-2 FIX] 使用 closeDone() 安全关闭
		s.closeDone()
		atomic.AddInt64(&c.metrics.StreamsClosed, 1)
	}
}

func (c *Client) handleResponseData(f *http2.DataFrame) {
	c.mu.Lock()
	s, ok := c.streams[f.StreamID]
	c.mu.Unlock()
	if !ok {
		return
	}

	// [BUG-6 FIX] 先检查 stream 是否已关闭
	if s.closed.Load() {
		return
	}

	data := make([]byte, len(f.Data()))
	copy(data, f.Data())

	atomic.AddInt64(&c.metrics.BytesRead, int64(len(data)))

	// Send WINDOW_UPDATE for flow control
	n := uint32(len(data))
	if n > 0 {
		c.writeMu.Lock()
		_ = c.framer.WriteWindowUpdate(0, n)
		_ = c.framer.WriteWindowUpdate(f.StreamID, n)
		atomic.AddInt64(&c.metrics.FramesSent, 2)
		c.writeMu.Unlock()
	}

	// [BUG-6 FIX] 使用 trySendData 安全发送
	s.trySendData(&dataChunk{data: data, endStream: f.StreamEnded()})

	if f.StreamEnded() {
		// [BUG-2 FIX] 使用 closeDone() 安全关闭
		s.closeDone()
		atomic.AddInt64(&c.metrics.StreamsClosed, 1)
	}
}

func (c *Client) handleRSTStream(f *http2.RSTStreamFrame) {
	c.mu.Lock()
	s, ok := c.streams[f.StreamID]
	c.mu.Unlock()
	if !ok {
		return
	}

	// [BUG-6 FIX] 使用 trySendHeaders 安全发送错误
	s.trySendHeaders(&headerResult{err: ErrStreamReset})

	// [BUG-2 FIX] 使用 closeDone() 安全关闭，不会重复 close
	s.closeDone()
	atomic.AddInt64(&c.metrics.StreamsClosed, 1)
}

// Do sends an HTTP request and returns the response.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if atomic.LoadInt32(&c.state) != stateActive {
		return nil, ErrClientClosed
	}

	// Allocate stream with server's initial window size
	c.mu.Lock()
	streamID := c.nextStreamID
	c.nextStreamID += 2
	s := &stream{
		id:         streamID,
		headers:    make(chan *headerResult, 1),
		data:       make(chan *dataChunk, 64),
		done:       make(chan struct{}),
		sendWindow: int64(c.serverInitWin),
	}
	c.streams[streamID] = s
	c.mu.Unlock()
	atomic.AddInt64(&c.metrics.StreamsOpened, 1)

	defer func() {
		c.mu.Lock()
		delete(c.streams, streamID)
		c.mu.Unlock()
	}()

	// Encode headers with custom pseudo-header order
	headerBlock, err := c.encodeHeaders(req)
	if err != nil {
		return nil, fmt.Errorf("h2: encode headers: %w", err)
	}

	hasBody := req.Body != nil && req.Body != http.NoBody

	c.writeMu.Lock()
	err = c.framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: headerBlock,
		EndStream:     !hasBody,
		EndHeaders:    true,
	})
	atomic.AddInt64(&c.metrics.FramesSent, 1)
	c.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("h2: write headers: %w", err)
	}

	if hasBody {
		if err := c.sendBody(streamID, req.Body); err != nil {
			return nil, fmt.Errorf("h2: write body: %w", err)
		}
	}

	// Wait for response headers with configurable timeout
	select {
	case hr := <-s.headers:
		if hr.err != nil {
			return nil, hr.err
		}
		resp := &http.Response{
			StatusCode:    hr.status,
			Status:        fmt.Sprintf("%d %s", hr.status, http.StatusText(hr.status)),
			Header:        hr.headers,
			Proto:         "HTTP/2.0",
			ProtoMajor:    2,
			ProtoMinor:    0,
			Request:       req,
			Body:          &streamBody{stream: s, closeCh: c.closeCh},
			ContentLength: -1,
		}
		if cl := hr.headers.Get("Content-Length"); cl != "" {
			resp.ContentLength, _ = strconv.ParseInt(cl, 10, 64)
		}
		return resp, nil
	case <-c.closeCh:
		return nil, ErrClientClosed
	case <-time.After(c.responseTimeout):
		return nil, fmt.Errorf("h2: response timeout after %v", c.responseTimeout)
	}
}

func (c *Client) encodeHeaders(req *http.Request) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.hpackBuf.Reset()

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	path := req.URL.RequestURI()
	if path == "" {
		path = "/"
	}

	pseudoValues := map[string]string{
		":method":    req.Method,
		":authority": host,
		":scheme":    "https",
		":path":      path,
	}

	// Write pseudo-headers in profile-specified order (fingerprint critical)
	for _, key := range c.fp.PseudoHeaderOrder {
		val, ok := pseudoValues[key]
		if !ok {
			continue
		}
		c.hpackEnc.WriteField(hpack.HeaderField{Name: key, Value: val})
	}

	// Regular headers (skip hop-by-hop and pseudo-headers)
	skipHeaders := map[string]bool{
		"host": true, "transfer-encoding": true, "connection": true,
		"keep-alive": true, "upgrade": true, "proxy-connection": true,
	}
	for key, vals := range req.Header {
		lk := strings.ToLower(key)
		if skipHeaders[lk] || strings.HasPrefix(lk, ":") {
			continue
		}
		for _, v := range vals {
			c.hpackEnc.WriteField(hpack.HeaderField{Name: lk, Value: v})
		}
	}

	out := make([]byte, c.hpackBuf.Len())
	copy(out, c.hpackBuf.Bytes())
	return out, nil
}

func (c *Client) sendBody(streamID uint32, body io.Reader) error {
	buf := make([]byte, c.fp.GetMaxFrameSize())
	for {
		n, err := body.Read(buf)
		if n > 0 {
			endStream := err == io.EOF
			c.writeMu.Lock()
			writeErr := c.framer.WriteData(streamID, endStream, buf[:n])
			atomic.AddInt64(&c.metrics.FramesSent, 1)
			atomic.AddInt64(&c.metrics.BytesWritten, int64(n))
			c.writeMu.Unlock()
			if writeErr != nil {
				return writeErr
			}
			if endStream {
				return nil
			}
		}
		if err != nil {
			if err == io.EOF {
				c.writeMu.Lock()
				writeErr := c.framer.WriteData(streamID, true, nil)
				atomic.AddInt64(&c.metrics.FramesSent, 1)
				c.writeMu.Unlock()
				return writeErr
			}
			return err
		}
	}
}

func (c *Client) closeInternal(err error) {
	c.closeOnce.Do(func() {
		atomic.StoreInt32(&c.state, stateClosed)
		close(c.closeCh)

		// [BUG-6 FIX] 安全通知所有 pending streams
		c.mu.Lock()
		for _, s := range c.streams {
			// 先发送错误，再标记关闭
			s.trySendHeaders(&headerResult{err: ErrClientClosed})
			s.closeDone()
		}
		c.mu.Unlock()

		c.conn.Close()
	})
}

// Close gracefully closes the H2 client.
func (c *Client) Close() error {
	c.closeInternal(nil)
	return nil
}

// IsClosed returns true if the client has been closed.
func (c *Client) IsClosed() bool {
	return atomic.LoadInt32(&c.state) == stateClosed
}

// Metrics returns a snapshot of connection metrics.
func (c *Client) Metrics() ClientMetrics {
	return ClientMetrics{
		StreamsOpened:  atomic.LoadInt64(&c.metrics.StreamsOpened),
		StreamsClosed:  atomic.LoadInt64(&c.metrics.StreamsClosed),
		FramesSent:     atomic.LoadInt64(&c.metrics.FramesSent),
		FramesReceived: atomic.LoadInt64(&c.metrics.FramesReceived),
		BytesRead:      atomic.LoadInt64(&c.metrics.BytesRead),
		BytesWritten:   atomic.LoadInt64(&c.metrics.BytesWritten),
	}
}

// streamBody implements io.ReadCloser for response body.
type streamBody struct {
	stream  *stream
	closeCh chan struct{}
	buf     []byte
	eof     bool
}

func (b *streamBody) Read(p []byte) (int, error) {
	if len(b.buf) > 0 {
		n := copy(p, b.buf)
		b.buf = b.buf[n:]
		return n, nil
	}
	if b.eof {
		return 0, io.EOF
	}

	select {
	case chunk, ok := <-b.stream.data:
		if !ok {
			return 0, io.EOF
		}
		if chunk.endStream {
			b.eof = true
		}
		n := copy(p, chunk.data)
		if n < len(chunk.data) {
			b.buf = chunk.data[n:]
		}
		if b.eof && len(b.buf) == 0 {
			return n, io.EOF
		}
		return n, nil
	case <-b.stream.done:
		return 0, io.EOF
	case <-b.closeCh:
		return 0, ErrClientClosed
	}
}

func (b *streamBody) Close() error {
	for {
		select {
		case _, ok := <-b.stream.data:
			if !ok {
				return nil
			}
		default:
			return nil
		}
	}
}








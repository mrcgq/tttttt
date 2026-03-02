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
	ErrNotReady     = errors.New("h2: settings exchange not completed")
)

const (
	stateActive = 0
	stateGoAway = 1
	stateClosed = 2
)

type ClientMetrics struct {
	StreamsOpened  int64
	StreamsClosed  int64
	FramesSent     int64
	FramesReceived int64
	BytesRead      int64
	BytesWritten   int64
}

type Client struct {
	conn   net.Conn
	framer *http2.Framer
	fp     *FingerprintConfig

	hpackBuf bytes.Buffer
	hpackEnc *hpack.Encoder

	nextStreamID uint32
	mu           sync.Mutex
	streams      map[uint32]*stream

	writeMu sync.Mutex

	state        int32
	closeOnce    sync.Once
	closeCh      chan struct{}
	lastStreamID uint32

	connSendWindow int64
	serverInitWin  uint32

	responseTimeout time.Duration

	// SETTINGS 交换信号
	settingsReady chan struct{}
	settingsOnce  sync.Once

	metrics ClientMetrics
}

type stream struct {
	id         uint32
	headers    chan *headerResult
	data       chan *dataChunk
	done       chan struct{}
	sendWindow int64

	doneOnce sync.Once
	closed   atomic.Bool
}

func (s *stream) closeDone() {
	s.doneOnce.Do(func() {
		s.closed.Store(true)
		close(s.done)
	})
}

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
		settingsReady:   make(chan struct{}),
	}

	preface := BuildPreface(fp)
	if _, err := conn.Write(preface); err != nil {
		return nil, fmt.Errorf("h2: write preface: %w", err)
	}

	c.framer = http2.NewFramer(conn, conn)
	c.framer.SetMaxReadFrameSize(1 << 24)
	c.framer.ReadMetaHeaders = hpack.NewDecoder(65536, nil)
	c.framer.MaxHeaderListSize = 262144

	c.hpackEnc = hpack.NewEncoder(&c.hpackBuf)

	go c.readLoop()

	return c, nil
}

// WaitReady 阻塞直到收到并确认服务器的第一个 SETTINGS 帧。
func (c *Client) WaitReady(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	select {
	case <-c.settingsReady:
		return nil
	case <-c.closeCh:
		return ErrClientClosed
	case <-time.After(timeout):
		return fmt.Errorf("h2: settings exchange timeout after %v", timeout)
	}
}

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

	c.writeMu.Lock()
	_ = c.framer.WriteSettingsAck()
	atomic.AddInt64(&c.metrics.FramesSent, 1)
	c.writeMu.Unlock()

	// 通知 WaitReady：SETTINGS 交换完成
	c.settingsOnce.Do(func() {
		close(c.settingsReady)
	})
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

	s.trySendHeaders(&headerResult{status: status, headers: hdr})

	if f.StreamEnded() {
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

	if s.closed.Load() {
		return
	}

	data := make([]byte, len(f.Data()))
	copy(data, f.Data())

	atomic.AddInt64(&c.metrics.BytesRead, int64(len(data)))

	n := uint32(len(data))
	if n > 0 {
		c.writeMu.Lock()
		_ = c.framer.WriteWindowUpdate(0, n)
		_ = c.framer.WriteWindowUpdate(f.StreamID, n)
		atomic.AddInt64(&c.metrics.FramesSent, 2)
		c.writeMu.Unlock()
	}

	s.trySendData(&dataChunk{data: data, endStream: f.StreamEnded()})

	if f.StreamEnded() {
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

	// 将 RST_STREAM 错误码包含在错误信息中
	rstErr := fmt.Errorf("h2: stream reset (code=%v, stream=%d)", f.ErrCode, f.StreamID)
	s.trySendHeaders(&headerResult{err: rstErr})
	s.closeDone()
	atomic.AddInt64(&c.metrics.StreamsClosed, 1)
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if atomic.LoadInt32(&c.state) != stateActive {
		return nil, ErrClientClosed
	}

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

	for _, key := range c.fp.PseudoHeaderOrder {
		val, ok := pseudoValues[key]
		if !ok {
			continue
		}
		_ = c.hpackEnc.WriteField(hpack.HeaderField{Name: key, Value: val})
	}

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
			_ = c.hpackEnc.WriteField(hpack.HeaderField{Name: lk, Value: v})
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

		c.settingsOnce.Do(func() {
			close(c.settingsReady)
		})

		c.mu.Lock()
		for _, s := range c.streams {
			s.trySendHeaders(&headerResult{err: ErrClientClosed})
			s.closeDone()
		}
		c.mu.Unlock()

		c.conn.Close()
	})
}

func (c *Client) Close() error {
	c.closeInternal(nil)
	return nil
}

func (c *Client) IsClosed() bool {
	return atomic.LoadInt32(&c.state) == stateClosed
}

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

// ============================================================
// ==================== 隧道支持 ==============================
// ============================================================

// H2TunnelConn 将 HTTP/2 stream 包装为 net.Conn。
type H2TunnelConn struct {
	client   *Client
	streamID uint32
	s        *stream

	readBuf []byte
	readEOF bool

	localAddr  net.Addr
	remoteAddr net.Addr

	closeOnce sync.Once
}

// OpenTunnel 打开一个 HTTP/2 POST stream 隧道。
// target 地址通过 X-Target header 发送给 Worker（避免 body 缓冲问题）。
func (c *Client) OpenTunnel(host, path, userAgent string, extraHeaders map[string]string) (*H2TunnelConn, error) {
	if atomic.LoadInt32(&c.state) != stateActive {
		return nil, ErrClientClosed
	}

	c.mu.Lock()
	streamID := c.nextStreamID
	c.nextStreamID += 2
	s := &stream{
		id:         streamID,
		headers:    make(chan *headerResult, 1),
		data:       make(chan *dataChunk, 256),
		done:       make(chan struct{}),
		sendWindow: int64(c.serverInitWin),
	}
	c.streams[streamID] = s
	c.mu.Unlock()
	atomic.AddInt64(&c.metrics.StreamsOpened, 1)

	cleanup := func() {
		c.mu.Lock()
		delete(c.streams, streamID)
		c.mu.Unlock()
	}

	headerBlock, err := c.encodeTunnelHeaders(host, path, userAgent, extraHeaders)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("h2: encode tunnel headers: %w", err)
	}

	// 发送 HEADERS 帧（EndStream=false），target 在 X-Target header 中
	c.writeMu.Lock()
	err = c.framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: headerBlock,
		EndStream:     false,
		EndHeaders:    true,
	})
	atomic.AddInt64(&c.metrics.FramesSent, 1)
	c.writeMu.Unlock()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("h2: write tunnel headers: %w", err)
	}

	// 等待 200 响应
	select {
	case hr := <-s.headers:
		if hr.err != nil {
			cleanup()
			return nil, fmt.Errorf("h2: tunnel response: %w", hr.err)
		}
		if hr.status != 200 {
			cleanup()
			return nil, fmt.Errorf("h2: tunnel server returned status %d", hr.status)
		}
	case <-c.closeCh:
		cleanup()
		return nil, ErrClientClosed
	case <-time.After(c.responseTimeout):
		cleanup()
		return nil, fmt.Errorf("h2: tunnel response timeout after %v", c.responseTimeout)
	}

	return &H2TunnelConn{
		client:     c,
		streamID:   streamID,
		s:          s,
		localAddr:  c.conn.LocalAddr(),
		remoteAddr: c.conn.RemoteAddr(),
	}, nil
}

func (c *Client) encodeTunnelHeaders(host, path, userAgent string, extra map[string]string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.hpackBuf.Reset()

	pseudoValues := map[string]string{
		":method":    "POST",
		":authority": host,
		":scheme":    "https",
		":path":      path,
	}

	for _, key := range c.fp.PseudoHeaderOrder {
		val, ok := pseudoValues[key]
		if !ok {
			continue
		}
		_ = c.hpackEnc.WriteField(hpack.HeaderField{Name: key, Value: val})
	}

	if userAgent != "" {
		_ = c.hpackEnc.WriteField(hpack.HeaderField{Name: "user-agent", Value: userAgent})
	}
	_ = c.hpackEnc.WriteField(hpack.HeaderField{Name: "content-type", Value: "application/octet-stream"})
	_ = c.hpackEnc.WriteField(hpack.HeaderField{Name: "accept", Value: "*/*"})

	// 写入额外头部（包括 x-target）
	for k, v := range extra {
		_ = c.hpackEnc.WriteField(hpack.HeaderField{Name: strings.ToLower(k), Value: v})
	}

	out := make([]byte, c.hpackBuf.Len())
	copy(out, c.hpackBuf.Bytes())
	return out, nil
}

// Read 从隧道读取数据
func (t *H2TunnelConn) Read(p []byte) (int, error) {
	if len(t.readBuf) > 0 {
		n := copy(p, t.readBuf)
		t.readBuf = t.readBuf[n:]
		return n, nil
	}
	if t.readEOF {
		return 0, io.EOF
	}

	select {
	case chunk, ok := <-t.s.data:
		if !ok {
			t.readEOF = true
			return 0, io.EOF
		}
		if chunk.endStream {
			t.readEOF = true
		}
		n := copy(p, chunk.data)
		if n < len(chunk.data) {
			t.readBuf = chunk.data[n:]
		}
		if t.readEOF && len(t.readBuf) == 0 {
			return n, io.EOF
		}
		return n, nil
	case <-t.s.done:
		t.readEOF = true
		return 0, io.EOF
	case <-t.client.closeCh:
		return 0, ErrClientClosed
	}
}

// Write 向隧道写入数据
func (t *H2TunnelConn) Write(p []byte) (int, error) {
	if t.s.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	if t.client.IsClosed() {
		return 0, ErrClientClosed
	}

	maxFrame := int(t.client.fp.GetMaxFrameSize())
	total := 0
	remaining := p

	for len(remaining) > 0 {
		chunk := remaining
		if len(chunk) > maxFrame {
			chunk = chunk[:maxFrame]
		}

		t.client.writeMu.Lock()
		err := t.client.framer.WriteData(t.streamID, false, chunk)
		atomic.AddInt64(&t.client.metrics.FramesSent, 1)
		atomic.AddInt64(&t.client.metrics.BytesWritten, int64(len(chunk)))
		t.client.writeMu.Unlock()
		if err != nil {
			return total, fmt.Errorf("h2: write tunnel data: %w", err)
		}

		total += len(chunk)
		remaining = remaining[len(chunk):]
	}

	return total, nil
}

// Close 关闭隧道
func (t *H2TunnelConn) Close() error {
	t.closeOnce.Do(func() {
		t.client.writeMu.Lock()
		_ = t.client.framer.WriteData(t.streamID, true, nil)
		atomic.AddInt64(&t.client.metrics.FramesSent, 1)
		t.client.writeMu.Unlock()

		t.s.closeDone()

		t.client.mu.Lock()
		delete(t.client.streams, t.streamID)
		t.client.mu.Unlock()
	})
	return nil
}

func (t *H2TunnelConn) LocalAddr() net.Addr                { return t.localAddr }
func (t *H2TunnelConn) RemoteAddr() net.Addr               { return t.remoteAddr }
func (t *H2TunnelConn) SetDeadline(d time.Time) error      { return t.client.conn.SetDeadline(d) }
func (t *H2TunnelConn) SetReadDeadline(d time.Time) error  { return t.client.conn.SetReadDeadline(d) }
func (t *H2TunnelConn) SetWriteDeadline(d time.Time) error { return t.client.conn.SetWriteDeadline(d) }

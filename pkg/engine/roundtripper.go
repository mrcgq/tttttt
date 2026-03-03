
package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/user/tls-client/internal/h2"
	"github.com/user/tls-client/pkg/fingerprint"
	"github.com/user/tls-client/pkg/verify"
)

// FingerprintTransport implements http.RoundTripper with full TLS+H2 fingerprint control.
type FingerprintTransport struct {
	// Selector picks a BrowserProfile for each request.
	Selector fingerprint.Selector

	// VerifyMode controls certificate verification.
	VerifyMode verify.Mode

	// VerifyOpts holds additional verification options.
	VerifyOpts *verify.Options

	// TargetAddr overrides DNS resolution (connect to this IP:Port directly).
	TargetAddr string

	// SNI overrides the SNI for all requests (domain fronting).
	SNI string

	// Retry configures retry behavior for TLS dial.
	Retry *RetryConfig

	// [新增] Cadence 时序控制
	Cadence *Cadence

	// [新增] Cookie 管理
	CookieManager *CookieManager

	// [新增] 是否启用自动重定向
	FollowRedirects bool
	MaxRedirects    int

	mu        sync.Mutex
	h2Clients map[string]*h2ClientEntry
	closed    bool

	// 统计
	requestCount int64
	successCount int64
	failCount    int64
}

type h2ClientEntry struct {
	client  *h2.Client
	profile *fingerprint.BrowserProfile
}

// NewFingerprintTransport 创建指纹传输层
func NewFingerprintTransport(selector fingerprint.Selector) *FingerprintTransport {
	return &FingerprintTransport{
		Selector:        selector,
		VerifyMode:      verify.ModeSNISkip,
		h2Clients:       make(map[string]*h2ClientEntry),
		MaxRedirects:    10,
		FollowRedirects: true,
	}
}

// WithCadence 设置时序控制
func (t *FingerprintTransport) WithCadence(cadence *Cadence) *FingerprintTransport {
	t.Cadence = cadence
	return t
}

// WithCookieManager 设置 Cookie 管理
func (t *FingerprintTransport) WithCookieManager(cm *CookieManager) *FingerprintTransport {
	t.CookieManager = cm
	return t
}

// RoundTrip implements http.RoundTripper
func (t *FingerprintTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	atomic.AddInt64(&t.requestCount, 1)

	// 时序控制
	if t.Cadence != nil {
		t.Cadence.Wait()
	}

	// 应用 Cookie
	if t.CookieManager != nil {
		t.CookieManager.ApplyToRequest(req)
	}

	// 执行请求
	resp, err := t.doRoundTrip(req)

	if err != nil {
		atomic.AddInt64(&t.failCount, 1)
		return nil, err
	}

	atomic.AddInt64(&t.successCount, 1)

	// 保存 Cookie
	if t.CookieManager != nil {
		t.CookieManager.SaveFromResponse(resp)
	}

	return resp, nil
}

func (t *FingerprintTransport) doRoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("engine: only https is supported, got %s", req.URL.Scheme)
	}

	host := req.URL.Host
	profile := t.Selector.Select(host)
	if profile == nil {
		return nil, fmt.Errorf("engine: no profile selected for %s", host)
	}

	// Determine actual connection target
	addr := t.TargetAddr
	if addr == "" {
		addr = host
		if _, _, err := net.SplitHostPort(addr); err != nil {
			addr = net.JoinHostPort(addr, "443")
		}
	}

	sni := t.SNI
	if sni == "" {
		sni = req.URL.Hostname()
	}

	// Set User-Agent from profile if not already set
	if req.Header.Get("User-Agent") == "" {
		if req.Header == nil {
			req.Header = make(http.Header)
		}
		req.Header.Set("User-Agent", profile.UserAgent)
	}

	// Try cached H2 client first
	if client := t.getCachedH2Client(host); client != nil {
		resp, err := client.Do(req)
		if err == nil {
			return resp, nil
		}
		// Client failed, remove from cache and establish new connection
		t.removeCachedH2Client(host)
	}

	// Dial TLS
	result, err := Dial(req.Context(), &DialConfig{
		Address:    addr,
		SNI:        sni,
		Profile:    profile,
		VerifyMode: t.VerifyMode,
		VerifyOpts: t.VerifyOpts,
		Retry:      t.Retry,
	})
	if err != nil {
		return nil, err
	}

	if result.NegProto == "h2" {
		return t.roundTripH2(host, result.Conn, profile, req)
	}
	return t.roundTripH1(result.Conn, req)
}

func (t *FingerprintTransport) getCachedH2Client(host string) *h2.Client {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.h2Clients == nil {
		return nil
	}
	entry, ok := t.h2Clients[host]
	if !ok {
		return nil
	}
	if entry.client.IsClosed() {
		delete(t.h2Clients, host)
		return nil
	}
	return entry.client
}

func (t *FingerprintTransport) removeCachedH2Client(host string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry, ok := t.h2Clients[host]; ok {
		entry.client.Close()
		delete(t.h2Clients, host)
	}
}

func (t *FingerprintTransport) roundTripH2(host string, conn net.Conn, profile *fingerprint.BrowserProfile, req *http.Request) (*http.Response, error) {
	client, err := h2.NewClient(conn, &profile.H2)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("engine: h2 client: %w", err)
	}

	// 等待 SETTINGS 交换完成
	if err := client.WaitReady(10 * time.Second); err != nil {
		client.Close()
		return nil, fmt.Errorf("engine: h2 ready: %w", err)
	}

	// Cache the H2 client for connection reuse
	t.mu.Lock()
	if t.h2Clients == nil {
		t.h2Clients = make(map[string]*h2ClientEntry)
	}
	t.h2Clients[host] = &h2ClientEntry{client: client, profile: profile}
	t.mu.Unlock()

	resp, err := client.Do(req)
	if err != nil {
		t.removeCachedH2Client(host)
		return nil, err
	}
	return resp, nil
}

func (t *FingerprintTransport) roundTripH1(conn net.Conn, req *http.Request) (*http.Response, error) {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return conn, nil
		},
		DisableKeepAlives: true,
		ForceAttemptHTTP2: false,
	}
	return transport.RoundTrip(req)
}

// CloseIdleConnections closes all cached H2 client connections.
func (t *FingerprintTransport) CloseIdleConnections() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	for host, entry := range t.h2Clients {
		entry.client.Close()
		delete(t.h2Clients, host)
	}
}

// Stats 返回统计信息
func (t *FingerprintTransport) Stats() map[string]int64 {
	return map[string]int64{
		"requests":  atomic.LoadInt64(&t.requestCount),
		"successes": atomic.LoadInt64(&t.successCount),
		"failures":  atomic.LoadInt64(&t.failCount),
	}
}

// CreateAntiDetectClient 创建反检测 HTTP 客户端（便捷函数）
func CreateAntiDetectClient(profileName string, opts ...func(*FingerprintTransport)) *http.Client {
	profile := fingerprint.Get(profileName)
	if profile == nil {
		profile = fingerprint.MustGet(fingerprint.DefaultProfile())
	}

	selector := &fingerprint.FixedSelector{Profile: profile}
	transport := NewFingerprintTransport(selector)

	// 应用选项
	for _, opt := range opts {
		opt(transport)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

// WithBrowsingCadence 添加浏览模式时序控制
func WithBrowsingCadence() func(*FingerprintTransport) {
	return func(t *FingerprintTransport) {
		t.Cadence = NewCadence(DefaultBrowsingCadence())
	}
}

// WithFastCadence 添加快速模式时序控制
func WithFastCadence() func(*FingerprintTransport) {
	return func(t *FingerprintTransport) {
		t.Cadence = NewCadence(DefaultFastCadence())
	}
}

// WithCookies 启用 Cookie 管理
func WithCookies() func(*FingerprintTransport) {
	return func(t *FingerprintTransport) {
		cm, _ := NewCookieManager()
		t.CookieManager = cm
	}
}

// WithDomainFronting 配置域前置
func WithDomainFronting(targetAddr, sni string) func(*FingerprintTransport) {
	return func(t *FingerprintTransport) {
		t.TargetAddr = targetAddr
		t.SNI = sni
	}
}









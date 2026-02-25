package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

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

	mu        sync.Mutex
	h2Clients map[string]*h2ClientEntry
	closed    bool
}

type h2ClientEntry struct {
	client  *h2.Client
	profile *fingerprint.BrowserProfile
}

func (t *FingerprintTransport) RoundTrip(req *http.Request) (*http.Response, error) {
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

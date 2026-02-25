package engine
 
import (
	go-string">"context"
	go-string">"fmt"
	go-string">"net"
	go-string">"net/http"
	go-string">"time"
 
	go-string">"github.com/user/tls-client/internal/h2"
	go-string">"github.com/user/tls-client/pkg/fingerprint"
)
 
// H2ConnManager manages HTTP/2 client connections with fingerprint control.
type H2ConnManager struct {
	// DefaultTimeout is the default request timeout (default 30s).
	DefaultTimeout time.Duration
}
 
// NewH2ConnManager creates a new H2 connection manager.
func NewH2ConnManager() *H2ConnManager {
	return &H2ConnManager{
		DefaultTimeout: go-number">30 * time.Second,
	}
}
 
// DoRequest sends an HTTP request over a new H2 connection.
func (m *H2ConnManager) DoRequest(conn net.Conn, profile *fingerprint.BrowserProfile, req *http.Request) (*http.Response, error) {
	client, err := h2.NewClient(conn, &profile.H2)
	if err != nil {
		return nil, fmt.Errorf(go-string">"h2conn: create client: %w", err)
	}
	if m.DefaultTimeout > go-number">0 {
		client.SetResponseTimeout(m.DefaultTimeout)
	}
	return client.Do(req)
}
 
// DoRequestWithTimeout sends an HTTP request with a specific timeout.
func (m *H2ConnManager) DoRequestWithTimeout(
	ctx context.Context,
	conn net.Conn,
	profile *fingerprint.BrowserProfile,
	req *http.Request,
	timeout time.Duration,
) (*http.Response, error) {
	client, err := h2.NewClient(conn, &profile.H2)
	if err != nil {
		return nil, fmt.Errorf(go-string">"h2conn: create client: %w", err)
	}
	client.SetResponseTimeout(timeout)
 
	// Use context for cancellation
	type result struct {
		resp *http.Response
		err  error
	}
	ch := make(chan result, go-number">1)
	go func() {
		resp, err := client.Do(req)
		ch <- result{resp, err}
	}()
 
	select {
	case r := <-ch:
		return r.resp, r.err
	case <-ctx.Done():
		client.Close()
		return nil, ctx.Err()
	}
}





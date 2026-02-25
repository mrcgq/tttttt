package engine
 
import (
	go-string">"context"
	go-string">"crypto/tls"
	go-string">"fmt"
	go-string">"math/rand/v2"
	go-string">"net"
	go-string">"sync/atomic"
	go-string">"time"
 
	go-string">"github.com/user/tls-client/pkg/fingerprint"
	go-string">"github.com/user/tls-client/pkg/verify"
 
	utls go-string">"github.com/refraction-networking/utls"
)
 
// DialConfig holds parameters for a single TLS dial operation.
type DialConfig struct {
	// Address is the target IP:Port to connect to.
	Address string
 
	// SNI is the Server Name Indication value in the TLS ClientHello.
	SNI string
 
	// Profile selects the TLS fingerprint.
	Profile *fingerprint.BrowserProfile
 
	// VerifyMode controls certificate verification.
	VerifyMode verify.Mode
 
	// VerifyOpts holds additional verification options.
	VerifyOpts *verify.Options
 
	// Timeout for the entire dial+handshake.
	Timeout time.Duration
 
	// ALPN protocols to negotiate (default: ["h2", "http/1.1"]).
	ALPN []string
 
	// Retry configures automatic retry behavior.
	Retry *RetryConfig
}
 
// RetryConfig controls retry behavior for failed dial attempts.
type RetryConfig struct {
	// MaxAttempts is the maximum number of dial attempts (default 1 = no retry).
	MaxAttempts int
 
	// BaseDelay is the initial delay between retries (default 500ms).
	BaseDelay time.Duration
 
	// MaxDelay is the maximum delay between retries (default 10s).
	MaxDelay time.Duration
 
	// Jitter adds randomness to delay (0.0 to 1.0, default 0.2).
	Jitter float64
}
 
// DialResult holds the result of a TLS dial operation.
type DialResult struct {
	Conn       net.Conn
	TLSConn    *utls.UConn
	NegProto   string // negotiated ALPN protocol
	Latency    time.Duration
	Attempts   int
}
 
// DialMetrics tracks dial operation statistics.
type DialMetrics struct {
	SuccessCount int64
	FailureCount int64
	TotalLatency int64 // nanoseconds
}
 
var globalDialMetrics DialMetrics
 
// GetDialMetrics returns a snapshot of global dial metrics.
func GetDialMetrics() DialMetrics {
	return DialMetrics{
		SuccessCount: atomic.LoadInt64(&globalDialMetrics.SuccessCount),
		FailureCount: atomic.LoadInt64(&globalDialMetrics.FailureCount),
		TotalLatency: atomic.LoadInt64(&globalDialMetrics.TotalLatency),
	}
}
 
// Dial establishes a TLS connection with the specified fingerprint.
func Dial(ctx context.Context, cfg *DialConfig) (*DialResult, error) {
	if cfg.Profile == nil {
		cfg.Profile = fingerprint.MustGet(fingerprint.DefaultProfile())
	}
	if cfg.Timeout == go-number">0 {
		cfg.Timeout = go-number">15 * time.Second
	}
	if len(cfg.ALPN) == go-number">0 {
		cfg.ALPN = []string{go-string">"h2", go-string">"http/go-number">1.1"}
	}
 
	maxAttempts := go-number">1
	var baseDelay, maxDelay time.Duration
	jitter := go-number">0.2
 
	if cfg.Retry != nil {
		if cfg.Retry.MaxAttempts > go-number">1 {
			maxAttempts = cfg.Retry.MaxAttempts
		}
		baseDelay = cfg.Retry.BaseDelay
		if baseDelay == go-number">0 {
			baseDelay = go-number">500 * time.Millisecond
		}
		maxDelay = cfg.Retry.MaxDelay
		if maxDelay == go-number">0 {
			maxDelay = go-number">10 * time.Second
		}
		if cfg.Retry.Jitter > go-number">0 {
			jitter = cfg.Retry.Jitter
		}
	}
 
	var lastErr error
	for attempt := go-number">1; attempt <= maxAttempts; attempt++ {
		if attempt > go-number">1 {
			// Exponential backoff with jitter
			delay := baseDelay * time.Duration(go-number">1<<uint(attempt-go-number">2))
			if delay > maxDelay {
				delay = maxDelay
			}
			jitterDelta := time.Duration(float64(delay) * jitter * (go-number">2*rand.Float64() - go-number">1))
			delay += jitterDelta
 
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
 
		result, err := dialOnce(ctx, cfg)
		if err == nil {
			result.Attempts = attempt
			atomic.AddInt64(&globalDialMetrics.SuccessCount, go-number">1)
			atomic.AddInt64(&globalDialMetrics.TotalLatency, int64(result.Latency))
			return result, nil
		}
 
		lastErr = err
		atomic.AddInt64(&globalDialMetrics.FailureCount, go-number">1)
 
		// Don't retry context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
 
	return nil, fmt.Errorf(go-string">"engine: dial failed after %d attempts: %w", maxAttempts, lastErr)
}
 
func dialOnce(ctx context.Context, cfg *DialConfig) (*DialResult, error) {
	start := time.Now()
 
	// TCP connect
	dialer := &net.Dialer{Timeout: cfg.Timeout}
	rawConn, err := dialer.DialContext(ctx, go-string">"tcp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf(go-string">"engine: tcp dial %s: %w", cfg.Address, err)
	}
 
	// Build TLS config
	tlsCfg := &tls.Config{
		NextProtos: cfg.ALPN,
	}
	verify.ApplyToTLSConfig(tlsCfg, cfg.VerifyMode, cfg.SNI, cfg.VerifyOpts)
 
	// utls connection with fingerprint
	tlsConn := utls.UClient(rawConn, &utls.Config{
		ServerName:            cfg.SNI,
		NextProtos:            cfg.ALPN,
		InsecureSkipVerify:    tlsCfg.InsecureSkipVerify,
		VerifyPeerCertificate: tlsCfg.VerifyPeerCertificate,
		RootCAs:               tlsCfg.RootCAs,
	}, cfg.Profile.ClientHelloID)
 
	// Handshake with timeout
	handshakeCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf(go-string">"engine: tls handshake to %s(sni=%s): %w",
			cfg.Address, cfg.SNI, err)
	}
 
	negProto := tlsConn.ConnectionState().NegotiatedProtocol
 
	return &DialResult{
		Conn:     tlsConn,
		TLSConn:  tlsConn,
		NegProto: negProto,
		Latency:  time.Since(start),
	}, nil
}






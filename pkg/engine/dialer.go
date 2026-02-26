package engine

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand/v2"
	"net"
	"sync/atomic"
	"time"

	"github.com/user/tls-client/pkg/fingerprint"
	"github.com/user/tls-client/pkg/verify"

	utls "github.com/refraction-networking/utls"
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
	Conn     net.Conn
	TLSConn  *utls.UConn
	NegProto string // negotiated ALPN protocol
	Latency  time.Duration
	Attempts int
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
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	if len(cfg.ALPN) == 0 {
		cfg.ALPN = []string{"h2", "http/1.1"}
	}

	maxAttempts := 1
	var baseDelay, maxDelay time.Duration
	jitter := 0.2

	if cfg.Retry != nil {
		if cfg.Retry.MaxAttempts > 1 {
			maxAttempts = cfg.Retry.MaxAttempts
		}
		baseDelay = cfg.Retry.BaseDelay
		if baseDelay == 0 {
			baseDelay = 500 * time.Millisecond
		}
		maxDelay = cfg.Retry.MaxDelay
		if maxDelay == 0 {
			maxDelay = 10 * time.Second
		}
		if cfg.Retry.Jitter > 0 {
			jitter = cfg.Retry.Jitter
		}
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			delay := baseDelay * time.Duration(1<<uint(attempt-2))
			if delay > maxDelay {
				delay = maxDelay
			}
			jitterDelta := time.Duration(float64(delay) * jitter * (2*rand.Float64() - 1))
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
			atomic.AddInt64(&globalDialMetrics.SuccessCount, 1)
			atomic.AddInt64(&globalDialMetrics.TotalLatency, int64(result.Latency))
			return result, nil
		}

		lastErr = err
		atomic.AddInt64(&globalDialMetrics.FailureCount, 1)

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("engine: dial failed after %d attempts: %w", maxAttempts, lastErr)
}

func dialOnce(ctx context.Context, cfg *DialConfig) (*DialResult, error) {
	start := time.Now()

	// 1. TCP Connect
	dialer := &net.Dialer{Timeout: cfg.Timeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("engine: tcp dial %s: %w", cfg.Address, err)
	}

	// 2. Build TLS Config
	tlsCfg := &tls.Config{
		NextProtos: cfg.ALPN,
	}
	verify.ApplyToTLSConfig(tlsCfg, cfg.VerifyMode, cfg.SNI, cfg.VerifyOpts)

	// 3. Create uTLS Client with fingerprint
	tlsConn := utls.UClient(rawConn, &utls.Config{
		ServerName:            cfg.SNI,
		NextProtos:            cfg.ALPN,
		InsecureSkipVerify:    tlsCfg.InsecureSkipVerify,
		VerifyPeerCertificate: tlsCfg.VerifyPeerCertificate,
		RootCAs:               tlsCfg.RootCAs,
	}, cfg.Profile.ClientHelloID)

	// ========================================================================
	// [CRITICAL FIX] 强制同步 ALPN (全平台修复)
	//
	// 此段代码的作用是防止 uTLS 的浏览器指纹模板强行添加它自带的协议（如 h2）。
	// 它会强制使 TLS 握手发送的 ALPN 列表与我们 Transport 层要求的完全一致。
	// ========================================================================
	if err := tlsConn.BuildHandshakeState(); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("engine: build handshake state: %w", err)
	}

	for _, ext := range tlsConn.Extensions {
		if alpnExt, ok := ext.(*utls.ALPNExtension); ok {
			// 强制覆盖：如果是 WS 模式这里就是 ["http/1.1"]，如果是 H2 模式这里就是 ["h2"]
			alpnExt.AlpnProtocols = cfg.ALPN
			break
		}
	}
	// ========================================================================

	// 4. Handshake with timeout
	handshakeCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
		_ = rawConn.Close()
		return nil, fmt.Errorf("engine: tls handshake to %s (sni=%s): %w",
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

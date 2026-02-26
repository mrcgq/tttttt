


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
			// Exponential backoff with jitter
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

		// Don't retry context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("engine: dial failed after %d attempts: %w", maxAttempts, lastErr)
}

func dialOnce(ctx context.Context, cfg *DialConfig) (*DialResult, error) {
	start := time.Now()

	// TCP connect
	dialer := &net.Dialer{Timeout: cfg.Timeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("engine: tcp dial %s: %w", cfg.Address, err)
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

	// ========================================================================
	// [CRITICAL FIX] 强制 ALPN 锁定 (全平台通用修复)
	//
	// 问题：uTLS 应用 Chrome 指纹时，会强制覆盖 ALPN 为 ["h2", "http/1.1"]
	// 后果：Cloudflare 选择 H2，但 WebSocket 需要 HTTP/1.1，导致协议冲突
	// 解决：在握手前，强制将指纹中的 ALPN 扩展替换为我们传输层要求的协议
	//
	// 这样做的好处：
	// 1. TLS 指纹（加密套件、扩展顺序等）依然保持 Chrome 特征
	// 2. 仅修改 ALPN 列表，模拟"禁用 H2 的 Chrome"（真实存在的场景）
	// 3. Cloudflare 被迫降级到 HTTP/1.1，WebSocket 握手成功
	// ========================================================================
	if err := tlsConn.BuildHandshakeState(); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("engine: build handshake state: %w", err)
	}

	// 遍历所有 TLS 扩展，找到 ALPN 扩展并强制覆盖
	for _, ext := range tlsConn.Extensions {
		if alpnExt, ok := ext.(*utls.ALPNExtension); ok {
			// cfg.ALPN 来自传输层（如 ws.go 返回 ["http/1.1"]）
			alpnExt.AlpnProtocols = cfg.ALPN
			break
		}
	}
	
	
    if err := tlsConn.MarshalClientHello(); err != nil {
    rawConn.Close()
    return nil, fmt.Errorf("engine: marshal client hello: %w", err)
    }
	// ========================================================================

	// Handshake with timeout
	handshakeCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("engine: tls handshake to %s (sni=%s): %w",
			cfg.Address, cfg.SNI, err)
	}

	negProto := tlsConn.ConnectionState().NegotiatedProtocol
	
	
    if len(cfg.ALPN) == 1 && cfg.ALPN[0] == "http/1.1" && negProto == "h2" {
    rawConn.Close()
    return nil, fmt.Errorf(
        "engine: ALPN mismatch: requested [http/1.1] but negotiated h2")
    }

	return &DialResult{
		Conn:     tlsConn,
		TLSConn:  tlsConn,
		NegProto: negProto,
		Latency:  time.Since(start),
	}, nil
}




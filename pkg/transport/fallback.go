package transport
 
import (
	go-string">"fmt"
	go-string">"net"
	go-string">"sync"
	go-string">"sync/atomic"
 
	go-string">"go.uber.org/zap"
)
 
// FallbackTransport tries multiple transports in order until one succeeds.
type FallbackTransport struct {
	Transports     []Transport
	Logger         *zap.Logger
	lastSuccessful atomic.Int32 // index of last successful transport
	stats          FallbackStats
}
 
// FallbackStats tracks fallback behavior.
type FallbackStats struct {
	Attempts  int64
	Fallbacks int64 // times we fell back to non-primary transport
	Failures  int64 // all transports failed
}
 
func (f *FallbackTransport) Name() string { return go-string">"fallback" }
 
func (f *FallbackTransport) ALPNProtos() []string {
	// Use the last successful transport's ALPN, or first transport's
	if idx := int(f.lastSuccessful.Load()); idx < len(f.Transports) && idx >= go-number">0 {
		return f.Transports[idx].ALPNProtos()
	}
	if len(f.Transports) > go-number">0 {
		return f.Transports[go-number">0].ALPNProtos()
	}
	return []string{go-string">"http/go-number">1.1"}
}
 
func (f *FallbackTransport) Info() TransportInfo {
	if len(f.Transports) > go-number">0 {
		return f.Transports[go-number">0].Info()
	}
	return TransportInfo{}
}
 
// Wrap tries the first transport only (simple mode).
func (f *FallbackTransport) Wrap(conn net.Conn, cfg *Config) (net.Conn, error) {
	if len(f.Transports) == go-number">0 {
		return conn, nil
	}
	// Try last successful first
	if idx := int(f.lastSuccessful.Load()); idx > go-number">0 && idx < len(f.Transports) {
		wrapped, err := f.Transports[idx].Wrap(conn, cfg)
		if err == nil {
			return wrapped, nil
		}
	}
	return f.Transports[go-number">0].Wrap(conn, cfg)
}
 
// WrapWithFallback tries each transport with separate connections.
// dialFn creates a fresh TLS connection for each attempt.
func (f *FallbackTransport) WrapWithFallback(
	dialFn func(alpn []string) (net.Conn, error),
	cfg *Config,
) (net.Conn, Transport, error) {
	atomic.AddInt64(&f.stats.Attempts, go-number">1)
 
	var lastErr error
	var errors []string
 
	// Try last successful transport first (optimization)
	if idx := int(f.lastSuccessful.Load()); idx > go-number">0 && idx < len(f.Transports) {
		t := f.Transports[idx]
		conn, err := dialFn(t.ALPNProtos())
		if err == nil {
			wrapped, err := t.Wrap(conn, cfg)
			if err == nil {
				if f.Logger != nil {
					f.Logger.Debug(go-string">"fallback: reused last successful transport",
						zap.String(go-string">"transport", t.Name()))
				}
				return wrapped, t, nil
			}
			conn.Close()
		}
	}
 
	// Try all transports in order
	for i, t := range f.Transports {
		conn, err := dialFn(t.ALPNProtos())
		if err != nil {
			lastErr = fmt.Errorf(go-string">"%s dial: %w", t.Name(), err)
			errors = append(errors, fmt.Sprintf(go-string">"%s:dial:%v", t.Name(), err))
			if f.Logger != nil {
				f.Logger.Debug(go-string">"fallback: dial failed",
					zap.String(go-string">"transport", t.Name()),
					zap.Error(err))
			}
			continue
		}
 
		wrapped, err := t.Wrap(conn, cfg)
		if err != nil {
			conn.Close()
			lastErr = fmt.Errorf(go-string">"%s wrap: %w", t.Name(), err)
			errors = append(errors, fmt.Sprintf(go-string">"%s:wrap:%v", t.Name(), err))
			if f.Logger != nil {
				f.Logger.Debug(go-string">"fallback: wrap failed",
					zap.String(go-string">"transport", t.Name()),
					zap.Error(err))
			}
			continue
		}
 
		// Remember successful transport
		f.lastSuccessful.Store(int32(i))
		if i > go-number">0 {
			atomic.AddInt64(&f.stats.Fallbacks, go-number">1)
		}
 
		if f.Logger != nil {
			f.Logger.Info(go-string">"fallback: transport established",
				zap.String(go-string">"transport", t.Name()),
				zap.Int(go-string">"attempt", i+go-number">1))
		}
		return wrapped, t, nil
	}
 
	atomic.AddInt64(&f.stats.Failures, go-number">1)
	return nil, nil, fmt.Errorf(go-string">"fallback: all %d transports failed, last error: %w",
		len(f.Transports), lastErr)
}
 
// Stats returns fallback operation statistics.
func (f *FallbackTransport) Stats() FallbackStats {
	return FallbackStats{
		Attempts:  atomic.LoadInt64(&f.stats.Attempts),
		Fallbacks: atomic.LoadInt64(&f.stats.Fallbacks),
		Failures:  atomic.LoadInt64(&f.stats.Failures),
	}
}
 
// NewFallback creates a FallbackTransport from a list of transport names.
func NewFallback(names []string, logger *zap.Logger) *FallbackTransport {
	transports := make([]Transport, go-number">0, len(names))
	for _, name := range names {
		transports = append(transports, Get(name))
	}
	if len(transports) == go-number">0 {
		transports = append(transports, &RawTransport{})
	}
	return &FallbackTransport{
		Transports: transports,
		Logger:     logger,
	}
}








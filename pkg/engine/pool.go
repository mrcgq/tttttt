
package engine

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/user/tls-client/pkg/fingerprint"
	"github.com/user/tls-client/pkg/verify"
)

// PoolConfig configures the connection pool.
type PoolConfig struct {
	MaxIdle     int
	MaxPerKey   int
	IdleTimeout time.Duration
	MaxLifetime time.Duration
}

// DefaultPoolConfig returns sensible defaults for the connection pool.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxIdle:     10,
		MaxPerKey:   3,
		IdleTimeout: 120 * time.Second,
		MaxLifetime: 10 * time.Minute,
	}
}

// ConnPool manages reusable TLS connections with health checking.
type ConnPool struct {
	mu          sync.Mutex
	conns       map[string][]*poolEntry
	cfg         PoolConfig
	closed      bool

	// metrics
	expiredCount int64
}

type poolEntry struct {
	conn      net.Conn
	createdAt time.Time
	lastUsed  time.Time
	inUse     bool
}

// PoolStats holds pool statistics.
type PoolStats struct {
	Total   int
	Idle    int
	InUse   int
	Expired int64
	Keys    int
}

// NewConnPool creates a connection pool with the given config.
func NewConnPool(maxIdle int, idleTimeout time.Duration) *ConnPool {
	cfg := DefaultPoolConfig()
	if maxIdle > 0 {
		cfg.MaxIdle = maxIdle
	}
	if idleTimeout > 0 {
		cfg.IdleTimeout = idleTimeout
	}
	return NewConnPoolWithConfig(cfg)
}

// NewConnPoolWithConfig creates a connection pool with full configuration.
func NewConnPoolWithConfig(cfg PoolConfig) *ConnPool {
	p := &ConnPool{
		conns: make(map[string][]*poolEntry),
		cfg:   cfg,
	}
	go p.cleanup()
	return p
}

// Get retrieves or creates a TLS connection.
func (p *ConnPool) Get(ctx context.Context, key string, cfg *DialConfig) (net.Conn, error) {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()
		return p.dial(ctx, cfg)
	}

	entries := p.conns[key]
	now := time.Now()

	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.inUse {
			continue
		}

		// Check if connection is expired (idle or max lifetime)
		if now.Sub(entry.lastUsed) > p.cfg.IdleTimeout ||
			now.Sub(entry.createdAt) > p.cfg.MaxLifetime {
			entry.conn.Close()
			entries = append(entries[:i], entries[i+1:]...)
			atomic.AddInt64(&p.expiredCount, 1)
			continue
		}

		// [BUG-4 FIX] 真实的健康探测，替代原来的 SetReadDeadline 假检查
		//
		// 原始代码的问题：
		//   SetReadDeadline 只检查了 conn 对象是否有效（非 nil，未 close）
		//   但无法检测：
		//   - 对端已关闭连接（TCP FIN/RST 已到达但未被读取）
		//   - 网络中断（路由变化、NAT 超时）
		//   - TLS 连接已被服务端超时关闭
		//
		// 修复方案：
		//   1. 设置极短的读超时（1ms）
		//   2. 尝试读取 1 字节
		//   3. 如果返回 EOF / 非超时错误 → 连接已死
		//   4. 如果返回超时错误 → 连接仍然存活（没有待读数据是正常的）
		//   5. 如果成功读到数据 → 异常（不应该有未请求的数据），视为不健康
		if !p.probeConnection(entry.conn) {
			entry.conn.Close()
			entries = append(entries[:i], entries[i+1:]...)
			continue
		}

		entry.inUse = true
		entry.lastUsed = now
		p.conns[key] = entries
		p.mu.Unlock()

		return &pooledConn{
			Conn:  entry.conn,
			pool:  p,
			key:   key,
			entry: entry,
		}, nil
	}

	p.conns[key] = entries
	p.mu.Unlock()

	return p.dial(ctx, cfg)
}

// probeConnection performs a real health check on a connection.
//
// [BUG-4 FIX] 核心探测逻辑：
//
//	对于 TLS/TCP 连接，尝试短超时读取来检测对端状态：
//	- 超时错误 = 连接正常（对端没有发送数据是预期行为）
//	- EOF = 对端已关闭
//	- 其他错误 = 连接异常
//	- 读到数据 = 意外数据，视为不健康
func (p *ConnPool) probeConnection(conn net.Conn) bool {
	probe := make([]byte, 1)

	// 设置极短的读超时
	if err := conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond)); err != nil {
		// SetReadDeadline 失败 → conn 对象已损坏
		return false
	}

	n, err := conn.Read(probe)

	// 立即恢复无限制的 deadline
	conn.SetReadDeadline(time.Time{})

	if n > 0 {
		// 读到了意外数据（在连接池中不应该有未请求的数据）
		// 这通常意味着服务端发送了 close notify 或 GOAWAY
		return false
	}

	if err == nil {
		// 读了 0 字节且无错误，理论上不会发生
		return true
	}

	// 判断错误类型
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		// 超时 = 没有待读数据 = 连接正常存活
		return true
	}

	// EOF / connection reset / 其他错误 = 连接已死
	return false
}

func (p *ConnPool) dial(ctx context.Context, cfg *DialConfig) (net.Conn, error) {
	result, err := Dial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return result.Conn, nil
}

// Put returns a connection to the pool for reuse.
func (p *ConnPool) Put(key string, conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		conn.Close()
		return
	}

	entries := p.conns[key]
	idleCount := 0
	for _, e := range entries {
		if !e.inUse {
			idleCount++
		}
	}

	if idleCount >= p.cfg.MaxPerKey || len(entries) >= p.cfg.MaxIdle {
		conn.Close()
		return
	}

	p.conns[key] = append(entries, &poolEntry{
		conn:      conn,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
		inUse:     false,
	})
}

// Release marks a pooled connection as available.
func (p *ConnPool) Release(key string, entry *poolEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		entry.conn.Close()
		return
	}
	entry.inUse = false
	entry.lastUsed = time.Now()
}

// Remove removes a connection from the pool (e.g., on error).
func (p *ConnPool) Remove(key string, entry *poolEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entries := p.conns[key]
	for i, e := range entries {
		if e == entry {
			e.conn.Close()
			p.conns[key] = append(entries[:i], entries[i+1:]...)
			return
		}
	}
}

func (p *ConnPool) cleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}

		now := time.Now()
		for key, entries := range p.conns {
			var active []*poolEntry
			for _, entry := range entries {
				if entry.inUse {
					active = append(active, entry)
					continue
				}
				if now.Sub(entry.lastUsed) > p.cfg.IdleTimeout ||
					now.Sub(entry.createdAt) > p.cfg.MaxLifetime {
					entry.conn.Close()
					atomic.AddInt64(&p.expiredCount, 1)
					continue
				}
				active = append(active, entry)
			}
			if len(active) > 0 {
				p.conns[key] = active
			} else {
				delete(p.conns, key)
			}
		}
		p.mu.Unlock()
	}
}

// Close closes all pooled connections.
func (p *ConnPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for _, entries := range p.conns {
		for _, entry := range entries {
			entry.conn.Close()
		}
	}
	p.conns = make(map[string][]*poolEntry)
}

// Stats returns comprehensive pool statistics.
func (p *ConnPool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	var stats PoolStats
	stats.Keys = len(p.conns)
	stats.Expired = atomic.LoadInt64(&p.expiredCount)
	for _, entries := range p.conns {
		for _, entry := range entries {
			stats.Total++
			if entry.inUse {
				stats.InUse++
			} else {
				stats.Idle++
			}
		}
	}
	return stats
}

// pooledConn wraps a connection to return it to pool on close.
type pooledConn struct {
	net.Conn
	pool  *ConnPool
	key   string
	entry *poolEntry
	// [BUG-4 附带修复] 用 atomic 保护 released，防止并发 Close
	released atomic.Bool
}

func (c *pooledConn) Close() error {
	// [BUG-4 附带修复] 原始代码用 bool 无并发保护
	// 两个 goroutine 同时 Close 可能导致 double release
	if !c.released.CompareAndSwap(false, true) {
		return nil
	}
	c.pool.Release(c.key, c.entry)
	return nil
}

// CloseWithError closes the connection and removes it from pool.
func (c *pooledConn) CloseWithError() error {
	if !c.released.CompareAndSwap(false, true) {
		return nil
	}
	c.pool.Remove(c.key, c.entry)
	return nil
}

// DialForProxy dials a TLS connection for the proxy outbound path.
func DialForProxy(ctx context.Context, address, sni string, profile *fingerprint.BrowserProfile, verifyMode verify.Mode) (net.Conn, string, error) {
	result, err := Dial(ctx, &DialConfig{
		Address:    address,
		SNI:        sni,
		Profile:    profile,
		VerifyMode: verifyMode,
		ALPN:       []string{"http/1.1"},
	})
	if err != nil {
		return nil, "", err
	}
	return result.Conn, result.NegProto, nil
}



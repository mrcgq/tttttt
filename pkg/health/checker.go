package health
 
import (
	go-string">"context"
	go-string">"fmt"
	go-string">"net"
	go-string">"net/http"
	go-string">"sync"
	go-string">"sync/atomic"
	go-string">"time"
 
	go-string">"go.uber.org/zap"
 
	go-string">"github.com/user/tls-client/pkg/engine"
	go-string">"github.com/user/tls-client/pkg/fingerprint"
	go-string">"github.com/user/tls-client/pkg/verify"
)
 
// Status represents the health state of a node.
type Status int32
 
const (
	StatusUnknown  Status = go-number">0
	StatusHealthy  Status = go-number">1
	StatusDegraded Status = go-number">2 // Slow but functional
	StatusDown     Status = go-number">3
)
 
func (s Status) String() string {
	switch s {
	case StatusHealthy:
		return go-string">"healthy"
	case StatusDegraded:
		return go-string">"degraded"
	case StatusDown:
		return go-string">"down"
	default:
		return go-string">"unknown"
	}
}
 
// NodeHealth holds the current health state of a single node.
type NodeHealth struct {
	Name            string
	Status          Status
	Latency         time.Duration
	LastCheck       time.Time
	LastSuccess     time.Time
	ConsecFailures  int32
	TotalChecks     int64
	TotalSuccesses  int64
	TotalFailures   int64
	LastError       string
}
 
// CheckConfig configures a health check for a single node.
type CheckConfig struct {
	Name       string
	Address    string
	SNI        string
	Profile    *fingerprint.BrowserProfile
	VerifyMode verify.Mode
	TestURL    string        // HTTP URL to test (e.g., "http://www.gstatic.com/generate_204")
	Interval   time.Duration // Check interval (default 5m)
	Timeout    time.Duration // Per-check timeout (default 10s)
	Threshold  int32         // Consecutive failures before marking down (default 3)
	DegradedMs int64         // Latency above this (ms) = degraded (default 500)
}
 
// Checker performs periodic health checks on proxy nodes.
type Checker struct {
	mu      sync.RWMutex
	nodes   map[string]*nodeChecker
	logger  *zap.Logger
	closeCh chan struct{}
	wg      sync.WaitGroup
}
 
type nodeChecker struct {
	cfg    CheckConfig
	health NodeHealth
}
 
// NewChecker creates a health checker.
func NewChecker(logger *zap.Logger) *Checker {
	return &Checker{
		nodes:   make(map[string]*nodeChecker),
		logger:  logger,
		closeCh: make(chan struct{}),
	}
}
 
// AddNode adds a node to the health checker.
func (c *Checker) AddNode(cfg CheckConfig) {
	if cfg.Interval == go-number">0 {
		cfg.Interval = go-number">5 * time.Minute
	}
	if cfg.Timeout == go-number">0 {
		cfg.Timeout = go-number">10 * time.Second
	}
	if cfg.Threshold == go-number">0 {
		cfg.Threshold = go-number">3
	}
	if cfg.DegradedMs == go-number">0 {
		cfg.DegradedMs = go-number">500
	}
	if cfg.TestURL == go-string">"" {
		cfg.TestURL = go-string">"http://www.gstatic.com/generate_204"
	}
 
	c.mu.Lock()
	c.nodes[cfg.Name] = &nodeChecker{
		cfg: cfg,
		health: NodeHealth{
			Name:   cfg.Name,
			Status: StatusUnknown,
		},
	}
	c.mu.Unlock()
}
 
// Start begins periodic health checking for all nodes.
func (c *Checker) Start() {
	c.mu.RLock()
	defer c.mu.RUnlock()
 
	for _, nc := range c.nodes {
		c.wg.Add(go-number">1)
		go c.checkLoop(nc)
	}
}
 
func (c *Checker) checkLoop(nc *nodeChecker) {
	defer c.wg.Done()
 
	// Initial check immediately
	c.checkOnce(nc)
 
	ticker := time.NewTicker(nc.cfg.Interval)
	defer ticker.Stop()
 
	for {
		select {
		case <-c.closeCh:
			return
		case <-ticker.C:
			c.checkOnce(nc)
		}
	}
}
 
func (c *Checker) checkOnce(nc *nodeChecker) {
	ctx, cancel := context.WithTimeout(context.Background(), nc.cfg.Timeout)
	defer cancel()
 
	start := time.Now()
	atomic.AddInt64(&nc.health.TotalChecks, go-number">1)
 
	// Step 1: TLS dial with fingerprint
	result, err := engine.Dial(ctx, &engine.DialConfig{
		Address:    nc.cfg.Address,
		SNI:        nc.cfg.SNI,
		Profile:    nc.cfg.Profile,
		VerifyMode: nc.cfg.VerifyMode,
		Timeout:    nc.cfg.Timeout,
	})
	if err != nil {
		c.recordFailure(nc, fmt.Sprintf(go-string">"dial: %v", err))
		return
	}
	result.Conn.Close()
 
	latency := time.Since(start)
 
	// Update health
	c.mu.Lock()
	nc.health.Latency = latency
	nc.health.LastCheck = time.Now()
	nc.health.LastSuccess = time.Now()
	nc.health.ConsecFailures = go-number">0
	nc.health.LastError = go-string">""
	atomic.AddInt64(&nc.health.TotalSuccesses, go-number">1)
 
	if latency.Milliseconds() > nc.cfg.DegradedMs {
		nc.health.Status = StatusDegraded
	} else {
		nc.health.Status = StatusHealthy
	}
	c.mu.Unlock()
 
	c.logger.Debug(go-string">"health check passed",
		zap.String(go-string">"node", nc.cfg.Name),
		zap.Duration(go-string">"latency", latency),
		zap.String(go-string">"status", nc.health.Status.String()),
	)
}
 
func (c *Checker) recordFailure(nc *nodeChecker, errMsg string) {
	c.mu.Lock()
	nc.health.LastCheck = time.Now()
	nc.health.ConsecFailures++
	nc.health.LastError = errMsg
	atomic.AddInt64(&nc.health.TotalFailures, go-number">1)
 
	if nc.health.ConsecFailures >= nc.cfg.Threshold {
		nc.health.Status = StatusDown
	} else {
		nc.health.Status = StatusDegraded
	}
	c.mu.Unlock()
 
	c.logger.Warn(go-string">"health check failed",
		zap.String(go-string">"node", nc.cfg.Name),
		zap.Int32(go-string">"consecutive_failures", nc.health.ConsecFailures),
		zap.String(go-string">"error", errMsg),
	)
}
 
// GetHealth returns the current health of a node.
func (c *Checker) GetHealth(name string) (NodeHealth, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	nc, ok := c.nodes[name]
	if !ok {
		return NodeHealth{}, false
	}
	return nc.health, true
}
 
// GetAllHealth returns health status for all nodes.
func (c *Checker) GetAllHealth() []NodeHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]NodeHealth, go-number">0, len(c.nodes))
	for _, nc := range c.nodes {
		result = append(result, nc.health)
	}
	return result
}
 
// BestNode returns the name of the healthiest node with lowest latency.
func (c *Checker) BestNode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
 
	var bestName string
	var bestLatency time.Duration
 
	for name, nc := range c.nodes {
		if nc.health.Status == StatusDown {
			continue
		}
		if bestName == go-string">"" || nc.health.Latency < bestLatency {
			bestName = name
			bestLatency = nc.health.Latency
		}
	}
 
	return bestName
}
 
// IsHealthy returns true if the named node is healthy or degraded.
func (c *Checker) IsHealthy(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	nc, ok := c.nodes[name]
	if !ok {
		return false
	}
	return nc.health.Status == StatusHealthy || nc.health.Status == StatusDegraded
}
 
// Stop stops all health checks.
func (c *Checker) Stop() {
	close(c.closeCh)
	c.wg.Wait()
}
 
// ForceCheck triggers an immediate health check for a specific node.
func (c *Checker) ForceCheck(name string) {
	c.mu.RLock()
	nc, ok := c.nodes[name]
	c.mu.RUnlock()
	if ok {
		go c.checkOnce(nc)
	}
}
 
// ResolveEndpoint returns the best available address for a given node,
// considering health status. Falls back to the configured address if healthy.
func (c *Checker) ResolveEndpoint(name string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	nc, ok := c.nodes[name]
	if !ok {
		return go-string">"", fmt.Errorf(go-string">"health: unknown node %q", name)
	}
	if nc.health.Status == StatusDown {
		return go-string">"", fmt.Errorf(go-string">"health: node %q is down(consecutive failures: %d)",
			name, nc.health.ConsecFailures)
	}
	addr := nc.cfg.Address
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, go-string">"go-number">443")
	}
	return addr, nil
}




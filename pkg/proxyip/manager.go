
package proxyip

import (
	"context"
	"math/rand"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/user/tls-client/pkg/config"
)

// Entry 代表一个可用的 ProxyIP
type Entry struct {
	Address     string
	SNI         string
	Weight      int
	Region      string
	Provider    string
	Latency     time.Duration
	LastCheck   time.Time
	FailCount   int32
	SuccessRate float64
	Available   bool
}

// Manager 管理 ProxyIP 池
type Manager struct {
	mu          sync.RWMutex
	entries     []*Entry
	current     int
	mode        SelectMode
	logger      *zap.Logger
	checkPeriod time.Duration
	timeout     time.Duration
	maxFails    int32
	stopCh      chan struct{}
	wg          sync.WaitGroup

	// 统计
	totalChecks  int64
	totalSuccess int64
	totalFail    int64
}

// SelectMode 选择模式
type SelectMode string

const (
	ModeRoundRobin SelectMode = "round-robin"
	ModeRandom     SelectMode = "random"
	ModeLatency    SelectMode = "latency"
	ModeWeighted   SelectMode = "weighted"
	ModeFailover   SelectMode = "failover"
)

// Config 管理器配置
type Config struct {
	Entries     []*Entry
	Mode        SelectMode
	CheckPeriod time.Duration
	Timeout     time.Duration
	MaxFails    int32
	Logger      *zap.Logger
}

// NewManager 创建 ProxyIP 管理器
func NewManager(cfg Config) *Manager {
	if cfg.CheckPeriod == 0 {
		cfg.CheckPeriod = 5 * time.Minute
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxFails == 0 {
		cfg.MaxFails = 3
	}
	if cfg.Mode == "" {
		cfg.Mode = ModeLatency
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	m := &Manager{
		entries:     cfg.Entries,
		mode:        cfg.Mode,
		logger:      cfg.Logger,
		checkPeriod: cfg.CheckPeriod,
		timeout:     cfg.Timeout,
		maxFails:    cfg.MaxFails,
		stopCh:      make(chan struct{}),
	}

	// 标记所有 IP 为初始可用
	for _, e := range m.entries {
		e.Available = true
	}

	return m
}

// NewManagerFromConfig 从配置创建管理器
func NewManagerFromConfig(cfgList []config.ProxyIPConfig, opts config.ProxyIPOptions, logger *zap.Logger) *Manager {
	entries := make([]*Entry, 0, len(cfgList))
	for _, cfg := range cfgList {
		if !cfg.Enabled {
			continue
		}
		entries = append(entries, &Entry{
			Address:   cfg.Address,
			SNI:       cfg.SNI,
			Weight:    cfg.Weight,
			Region:    cfg.Region,
			Provider:  cfg.Provider,
			Available: true,
		})
	}

	checkPeriod, _ := time.ParseDuration(opts.CheckPeriod)
	if checkPeriod == 0 {
		checkPeriod = 5 * time.Minute
	}

	timeout, _ := time.ParseDuration(opts.Timeout)
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	maxFails := int32(opts.MaxFails)
	if maxFails == 0 {
		maxFails = 3
	}

	mode := SelectMode(opts.Mode)
	if mode == "" {
		mode = ModeLatency
	}

	return NewManager(Config{
		Entries:     entries,
		Mode:        mode,
		CheckPeriod: checkPeriod,
		Timeout:     timeout,
		MaxFails:    maxFails,
		Logger:      logger,
	})
}

// Start 启动健康检查
func (m *Manager) Start() {
	m.wg.Add(1)
	go m.checkLoop()
}

// Stop 停止管理器
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

func (m *Manager) checkLoop() {
	defer m.wg.Done()

	// 立即执行一次检测
	m.CheckAll()

	ticker := time.NewTicker(m.checkPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.CheckAll()
		}
	}
}

// CheckAll 检测所有 IP
func (m *Manager) CheckAll() {
	m.mu.RLock()
	entries := make([]*Entry, len(m.entries))
	copy(entries, m.entries)
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, e := range entries {
		wg.Add(1)
		go func(entry *Entry) {
			defer wg.Done()
			m.checkOne(entry)
		}(e)
	}
	wg.Wait()

	m.logger.Info("proxyip: health check complete",
		zap.Int("total", len(entries)),
		zap.Int("available", m.AvailableCount()),
	)
}

func (m *Manager) checkOne(entry *Entry) {
	atomic.AddInt64(&m.totalChecks, 1)

	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", entry.Address)
	latency := time.Since(start)

	m.mu.Lock()
	defer m.mu.Unlock()

	entry.LastCheck = time.Now()

	if err != nil {
		entry.FailCount++
		atomic.AddInt64(&m.totalFail, 1)
		if entry.FailCount >= m.maxFails {
			entry.Available = false
		}
		m.logger.Debug("proxyip: check failed",
			zap.String("address", entry.Address),
			zap.Int32("fails", entry.FailCount),
			zap.Error(err),
		)
		return
	}

	conn.Close()
	atomic.AddInt64(&m.totalSuccess, 1)

	entry.Latency = latency
	entry.FailCount = 0
	entry.Available = true

	// 更新成功率
	total := float64(atomic.LoadInt64(&m.totalChecks))
	success := float64(atomic.LoadInt64(&m.totalSuccess))
	if total > 0 {
		entry.SuccessRate = success / total
	}

	m.logger.Debug("proxyip: check success",
		zap.String("address", entry.Address),
		zap.Duration("latency", latency),
	)
}

// Select 选择一个可用的 ProxyIP
func (m *Manager) Select() *Entry {
	m.mu.Lock()
	defer m.mu.Unlock()

	available := m.availableEntries()
	if len(available) == 0 {
		// 没有可用的，随机返回一个（可能已失败）
		if len(m.entries) > 0 {
			return m.entries[rand.Intn(len(m.entries))]
		}
		return nil
	}

	switch m.mode {
	case ModeRoundRobin:
		return m.selectRoundRobin(available)
	case ModeRandom:
		return available[rand.Intn(len(available))]
	case ModeLatency:
		return m.selectLowestLatency(available)
	case ModeWeighted:
		return m.selectWeighted(available)
	case ModeFailover:
		return available[0]
	default:
		return available[0]
	}
}

func (m *Manager) selectRoundRobin(available []*Entry) *Entry {
	m.current = (m.current + 1) % len(available)
	return available[m.current]
}

func (m *Manager) selectLowestLatency(available []*Entry) *Entry {
	sort.Slice(available, func(i, j int) bool {
		return available[i].Latency < available[j].Latency
	})
	return available[0]
}

func (m *Manager) selectWeighted(available []*Entry) *Entry {
	totalWeight := 0
	for _, e := range available {
		if e.Weight <= 0 {
			e.Weight = 1
		}
		totalWeight += e.Weight
	}

	r := rand.Intn(totalWeight)
	for _, e := range available {
		r -= e.Weight
		if r < 0 {
			return e
		}
	}
	return available[len(available)-1]
}

func (m *Manager) availableEntries() []*Entry {
	var result []*Entry
	for _, e := range m.entries {
		if e.Available {
			result = append(result, e)
		}
	}
	return result
}

// MarkFailed 标记 IP 失败
func (m *Manager) MarkFailed(address string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range m.entries {
		if e.Address == address {
			e.FailCount++
			if e.FailCount >= m.maxFails {
				e.Available = false
				m.logger.Warn("proxyip: marked unavailable",
					zap.String("address", address),
					zap.Int32("fails", e.FailCount),
				)
			}
			return
		}
	}
}

// MarkSuccess 标记 IP 成功
func (m *Manager) MarkSuccess(address string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range m.entries {
		if e.Address == address {
			e.FailCount = 0
			e.Available = true
			return
		}
	}
}

// AddEntry 动态添加 IP
func (m *Manager) AddEntry(entry *Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry.Available = true
	m.entries = append(m.entries, entry)
}

// RemoveEntry 移除 IP
func (m *Manager) RemoveEntry(address string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.entries {
		if e.Address == address {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			return
		}
	}
}

// List 获取所有 IP
func (m *Manager) List() []*Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Entry, len(m.entries))
	copy(result, m.entries)
	return result
}

// AvailableCount 可用 IP 数量
func (m *Manager) AvailableCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, e := range m.entries {
		if e.Available {
			count++
		}
	}
	return count
}

// Stats 统计信息
func (m *Manager) Stats() map[string]interface{} {
	return map[string]interface{}{
		"total":        len(m.entries),
		"available":    m.AvailableCount(),
		"checks":       atomic.LoadInt64(&m.totalChecks),
		"success":      atomic.LoadInt64(&m.totalSuccess),
		"fail":         atomic.LoadInt64(&m.totalFail),
		"mode":         m.mode,
		"check_period": m.checkPeriod.String(),
	}
}






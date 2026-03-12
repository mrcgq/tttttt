package config

import "time"

// Config 是主配置结构 - 完全体版本
type Config struct {
	Global         GlobalConfig         `yaml:"global" json:"global"`
	Inbound        InboundConfig        `yaml:"inbound" json:"inbound"`
	Fingerprint    FingerprintConfig    `yaml:"fingerprint" json:"fingerprint"`
	TLS            TLSConfig            `yaml:"tls" json:"tls"`
	Nodes          []NodeConfig         `yaml:"nodes" json:"nodes"`
	ClientBehavior ClientBehaviorConfig `yaml:"client_behavior" json:"client_behavior"`
	API            APIConfig            `yaml:"api" json:"api"`
	Health         HealthConfig         `yaml:"health" json:"health"`
	ProxyIPs       ProxyIPPoolConfig    `yaml:"proxy_ips" json:"proxy_ips"`
}

// ActiveNode 返回第一个 active=true 的节点
func (c *Config) ActiveNode() *NodeConfig {
	for i := range c.Nodes {
		if c.Nodes[i].Active {
			return &c.Nodes[i]
		}
	}
	if len(c.Nodes) > 0 {
		return &c.Nodes[0]
	}
	return nil
}

// ActiveNodes 返回所有 active=true 的节点
func (c *Config) ActiveNodes() []NodeConfig {
	var result []NodeConfig
	for _, n := range c.Nodes {
		if n.Active {
			result = append(result, n)
		}
	}
	return result
}

// GlobalConfig 全局配置
type GlobalConfig struct {
	LogLevel  string        `yaml:"log_level" json:"log_level"`
	LogOutput string        `yaml:"log_output" json:"log_output"`
	Metrics   MetricsConfig `yaml:"metrics" json:"metrics"`
}

// MetricsConfig 指标配置
type MetricsConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	Endpoint string `yaml:"endpoint" json:"endpoint"`
}

// InboundConfig 入站配置
type InboundConfig struct {
	SOCKS5 ListenConfig `yaml:"socks5" json:"socks5"`
	HTTP   ListenConfig `yaml:"http" json:"http"`
}

// ListenConfig 监听配置
type ListenConfig struct {
	Listen   string `yaml:"listen" json:"listen"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
}

// FingerprintConfig 指纹配置
type FingerprintConfig struct {
	Rotation RotationConfig `yaml:"rotation" json:"rotation"`
}

// RotationConfig 轮换配置
type RotationConfig struct {
	Mode     string   `yaml:"mode" json:"mode"`
	Profile  string   `yaml:"profile" json:"profile"`
	Profiles []string `yaml:"profiles" json:"profiles"`
	Interval string   `yaml:"interval" json:"interval"`
	Weights  []int    `yaml:"weights" json:"weights"`
}

// TLSConfig TLS 配置
type TLSConfig struct {
	VerifyMode string     `yaml:"verify_mode" json:"verify_mode"`
	VerifyOpts VerifyOpts `yaml:"verify_opts" json:"verify_opts"`
}

// VerifyOpts 验证选项
type VerifyOpts struct {
	CertPin  string `yaml:"cert_pin" json:"cert_pin"`
	CustomCA string `yaml:"custom_ca" json:"custom_ca"`
}

// NodeConfig 节点配置
type NodeConfig struct {
	Name          string        `yaml:"name" json:"name"`
	Address       string        `yaml:"address" json:"address"`
	SNI           string        `yaml:"sni" json:"sni"`
	Fingerprint   string        `yaml:"fingerprint" json:"fingerprint"`
	Active        bool          `yaml:"active" json:"active"`
	Transport     string        `yaml:"transport" json:"transport"`
	TransportOpts TransportOpts `yaml:"transport_opts" json:"transport_opts"`
	Fallback      []string      `yaml:"transport_fallback" json:"transport_fallback"`
	Retry         RetryOpts     `yaml:"retry" json:"retry"`
	Pool          PoolOpts      `yaml:"pool" json:"pool"`

	// 远程代理配置 (Xlink 借力机制)
	RemoteProxy RemoteProxyConfig `yaml:"remote_proxy" json:"remote_proxy"`
}

// TransportOpts 传输选项
type TransportOpts struct {
	WSPath         string            `yaml:"ws_path" json:"ws_path"`
	WSHost         string            `yaml:"ws_host" json:"ws_host"`
	WSHeaders      map[string]string `yaml:"ws_headers" json:"ws_headers"`
	H2Path         string            `yaml:"h2_path" json:"h2_path"`
	SOCKS5Addr     string            `yaml:"socks5_addr" json:"socks5_addr"`
	SOCKS5Username string            `yaml:"socks5_username" json:"socks5_username"`
	SOCKS5Password string            `yaml:"socks5_password" json:"socks5_password"`
}

// RetryOpts 重试选项
type RetryOpts struct {
	MaxAttempts int     `yaml:"max_attempts" json:"max_attempts"`
	BaseDelay   string  `yaml:"base_delay" json:"base_delay"`
	MaxDelay    string  `yaml:"max_delay" json:"max_delay"`
	Jitter      float64 `yaml:"jitter" json:"jitter"`
}

// PoolOpts 连接池选项
type PoolOpts struct {
	MaxIdle     int    `yaml:"max_idle" json:"max_idle"`
	MaxPerKey   int    `yaml:"max_per_key" json:"max_per_key"`
	IdleTimeout string `yaml:"idle_timeout" json:"idle_timeout"`
	MaxLifetime string `yaml:"max_lifetime" json:"max_lifetime"`
}

// RemoteProxyConfig 远程代理配置 (Xlink 借力机制)
type RemoteProxyConfig struct {
	SOCKS5   string `yaml:"socks5" json:"socks5"`
	Fallback string `yaml:"fallback" json:"fallback"`
}

// HasRemoteProxy 检查是否配置了远程代理
func (c *NodeConfig) HasRemoteProxy() bool {
	return c.RemoteProxy.SOCKS5 != "" || c.RemoteProxy.Fallback != ""
}

// GetSOCKS5Proxy 获取 SOCKS5 代理配置
func (c *NodeConfig) GetSOCKS5Proxy() string {
	return c.RemoteProxy.SOCKS5
}

// GetFallback 获取 Fallback 配置
func (c *NodeConfig) GetFallback() string {
	return c.RemoteProxy.Fallback
}

// ============================================================
// ClientBehaviorConfig - 客户端行为配置
// ============================================================

// ClientBehaviorConfig 客户端行为配置
type ClientBehaviorConfig struct {
	Cadence         CadenceConfig `yaml:"cadence" json:"cadence"`
	Cookies         CookiesConfig `yaml:"cookies" json:"cookies"`
	FollowRedirects bool          `yaml:"follow_redirects" json:"follow_redirects"`
	MaxRedirects    int           `yaml:"max_redirects" json:"max_redirects"`
}

// CadenceConfig 时序节奏配置
type CadenceConfig struct {
	Mode     string   `yaml:"mode" json:"mode"`
	MinDelay string   `yaml:"min_delay" json:"min_delay"`
	MaxDelay string   `yaml:"max_delay" json:"max_delay"`
	Sequence []string `yaml:"sequence" json:"sequence"`
	Jitter   float64  `yaml:"jitter" json:"jitter"`
}

// CookiesConfig Cookie管理配置
type CookiesConfig struct {
	Enabled         bool `yaml:"enabled" json:"enabled"`
	ClearOnRotation bool `yaml:"clear_on_rotation" json:"clear_on_rotation"`
}

// ParseCadenceMode 解析时序模式配置为引擎可用的格式
func (c *CadenceConfig) ParseCadenceMode() string {
	if c.Mode == "" {
		return "none"
	}
	return c.Mode
}

// ParseMinDelay 解析最小延迟
func (c *CadenceConfig) ParseMinDelay() time.Duration {
	if c.MinDelay == "" {
		return 0
	}
	d, _ := time.ParseDuration(c.MinDelay)
	return d
}

// ParseMaxDelay 解析最大延迟
func (c *CadenceConfig) ParseMaxDelay() time.Duration {
	if c.MaxDelay == "" {
		return 0
	}
	d, _ := time.ParseDuration(c.MaxDelay)
	return d
}

// ParseSequence 解析延迟序列
func (c *CadenceConfig) ParseSequence() []time.Duration {
	result := make([]time.Duration, 0, len(c.Sequence))
	for _, s := range c.Sequence {
		if d, err := time.ParseDuration(s); err == nil {
			result = append(result, d)
		}
	}
	return result
}

// ============================================================
// APIConfig 和 HealthConfig
// ============================================================

// APIConfig API服务器配置
type APIConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Listen  string `yaml:"listen" json:"listen"`
	Token   string `yaml:"token" json:"token"`
}

// HealthConfig 健康检查配置
type HealthConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	Interval   string `yaml:"interval" json:"interval"`
	Timeout    string `yaml:"timeout" json:"timeout"`
	Threshold  int    `yaml:"threshold" json:"threshold"`
	DegradedMs int64  `yaml:"degraded_ms" json:"degraded_ms"`
	TestURL    string `yaml:"test_url" json:"test_url"`
}

// ParseInterval 解析健康检查间隔
func (h *HealthConfig) ParseInterval() time.Duration {
	if h.Interval == "" {
		return 5 * time.Minute
	}
	d, _ := time.ParseDuration(h.Interval)
	if d == 0 {
		return 5 * time.Minute
	}
	return d
}

// ParseTimeout 解析健康检查超时
func (h *HealthConfig) ParseTimeout() time.Duration {
	if h.Timeout == "" {
		return 10 * time.Second
	}
	d, _ := time.ParseDuration(h.Timeout)
	if d == 0 {
		return 10 * time.Second
	}
	return d
}

// ============================================================
// ProxyIPPoolConfig
// ============================================================

// ProxyIPPoolConfig ProxyIP池配置
type ProxyIPPoolConfig struct {
	Enabled bool           `yaml:"enabled" json:"enabled"`
	Mode    string         `yaml:"mode" json:"mode"`
	Options ProxyIPOptions `yaml:"options" json:"options"`
	Entries []ProxyIPEntry `yaml:"entries" json:"entries"`
}

// ProxyIPOptions ProxyIP选项
type ProxyIPOptions struct {
	CheckPeriod string `yaml:"check_period" json:"check_period"`
	Timeout     string `yaml:"timeout" json:"timeout"`
	MaxFails    int    `yaml:"max_fails" json:"max_fails"`
}

// ProxyIPEntry ProxyIP条目
type ProxyIPEntry struct {
	Address  string `yaml:"address" json:"address"`
	SNI      string `yaml:"sni" json:"sni"`
	Weight   int    `yaml:"weight" json:"weight"`
	Region   string `yaml:"region" json:"region"`
	Provider string `yaml:"provider" json:"provider"`
	Enabled  bool   `yaml:"enabled" json:"enabled"`
}

// ============================================================
// 辅助方法
// ============================================================

// ParseBaseDelay 解析重试基础延迟
func (r *RetryOpts) ParseBaseDelay() time.Duration {
	if r.BaseDelay == "" {
		return 500 * time.Millisecond
	}
	d, _ := time.ParseDuration(r.BaseDelay)
	if d == 0 {
		return 500 * time.Millisecond
	}
	return d
}

// ParseMaxDelay 解析重试最大延迟
func (r *RetryOpts) ParseMaxDelay() time.Duration {
	if r.MaxDelay == "" {
		return 10 * time.Second
	}
	d, _ := time.ParseDuration(r.MaxDelay)
	if d == 0 {
		return 10 * time.Second
	}
	return d
}

// GetJitter 获取抖动系数，默认0.2
func (r *RetryOpts) GetJitter() float64 {
	if r.Jitter <= 0 {
		return 0.2
	}
	if r.Jitter > 1 {
		return 1.0
	}
	return r.Jitter
}

// ParseIdleTimeout 解析连接池空闲超时
func (p *PoolOpts) ParseIdleTimeout() time.Duration {
	if p.IdleTimeout == "" {
		return 120 * time.Second
	}
	d, _ := time.ParseDuration(p.IdleTimeout)
	if d == 0 {
		return 120 * time.Second
	}
	return d
}

// ParseMaxLifetime 解析连接池最大生命周期
func (p *PoolOpts) ParseMaxLifetime() time.Duration {
	if p.MaxLifetime == "" {
		return 10 * time.Minute
	}
	d, _ := time.ParseDuration(p.MaxLifetime)
	if d == 0 {
		return 10 * time.Minute
	}
	return d
}

// GetMaxIdle 获取最大空闲连接数
func (p *PoolOpts) GetMaxIdle() int {
	if p.MaxIdle <= 0 {
		return 10
	}
	return p.MaxIdle
}

// GetMaxPerKey 获取每个key的最大连接数
func (p *PoolOpts) GetMaxPerKey() int {
	if p.MaxPerKey <= 0 {
		return 3
	}
	return p.MaxPerKey
}

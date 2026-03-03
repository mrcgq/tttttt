package config

// Config 是主配置结构
type Config struct {
	Global      GlobalConfig      `yaml:"global"`
	Inbound     InboundConfig     `yaml:"inbound"`
	Fingerprint FingerprintConfig `yaml:"fingerprint"`
	TLS         TLSConfig         `yaml:"tls"`
	Nodes       []NodeConfig      `yaml:"nodes"`
}

// GlobalConfig 全局配置
type GlobalConfig struct {
	LogLevel  string        `yaml:"log_level"`
	LogOutput string        `yaml:"log_output"`
	Metrics   MetricsConfig `yaml:"metrics"`
}

// MetricsConfig 指标配置
type MetricsConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint"`
}

// InboundConfig 入站配置
type InboundConfig struct {
	SOCKS5 ListenConfig `yaml:"socks5"`
	HTTP   ListenConfig `yaml:"http"`
}

// ListenConfig 监听配置
type ListenConfig struct {
	Listen   string `yaml:"listen"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// FingerprintConfig 指纹配置
type FingerprintConfig struct {
	Rotation RotationConfig `yaml:"rotation"`
}

// RotationConfig 轮换配置
type RotationConfig struct {
	Mode     string   `yaml:"mode"`
	Profile  string   `yaml:"profile"`
	Profiles []string `yaml:"profiles"`
	Interval string   `yaml:"interval"`
	Weights  []int    `yaml:"weights"`
}

// TLSConfig TLS 配置
type TLSConfig struct {
	VerifyMode string     `yaml:"verify_mode"`
	VerifyOpts VerifyOpts `yaml:"verify_opts"`
}

// VerifyOpts 验证选项
type VerifyOpts struct {
	CertPin  string `yaml:"cert_pin"`
	CustomCA string `yaml:"custom_ca"`
}

// NodeConfig 节点配置
type NodeConfig struct {
	Name          string        `yaml:"name"`
	Address       string        `yaml:"address"`
	SNI           string        `yaml:"sni"`
	Fingerprint   string        `yaml:"fingerprint"`
	Active        bool          `yaml:"active"`
	Transport     string        `yaml:"transport"`
	TransportOpts TransportOpts `yaml:"transport_opts"`
	Fallback      []string      `yaml:"transport_fallback"`
	Retry         RetryOpts     `yaml:"retry"`
	Pool          PoolOpts      `yaml:"pool"`

	// 远程代理配置 (Xlink 借力机制)
	RemoteProxy RemoteProxyConfig `yaml:"remote_proxy"`
}

// TransportOpts 传输选项
type TransportOpts struct {
	WSPath         string            `yaml:"ws_path"`
	WSHost         string            `yaml:"ws_host"`
	WSHeaders      map[string]string `yaml:"ws_headers"`
	H2Path         string            `yaml:"h2_path"`
	SOCKS5Addr     string            `yaml:"socks5_addr"`
	SOCKS5Username string            `yaml:"socks5_username"`
	SOCKS5Password string            `yaml:"socks5_password"`
}

// RetryOpts 重试选项
type RetryOpts struct {
	MaxAttempts int    `yaml:"max_attempts"`
	BaseDelay   string `yaml:"base_delay"`
	MaxDelay    string `yaml:"max_delay"`
}

// PoolOpts 连接池选项
type PoolOpts struct {
	MaxIdle     int    `yaml:"max_idle"`
	MaxPerKey   int    `yaml:"max_per_key"`
	IdleTimeout string `yaml:"idle_timeout"`
	MaxLifetime string `yaml:"max_lifetime"`
}

// RemoteProxyConfig 远程代理配置 (Xlink 借力机制)
type RemoteProxyConfig struct {
	// SOCKS5 代理地址，Worker 会通过此代理连接目标
	// 格式: user:pass@host:port 或 host:port
	SOCKS5 string `yaml:"socks5"`

	// Fallback 地址，当直连失败时 Worker 会使用此地址
	// 格式: host:port
	Fallback string `yaml:"fallback"`
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

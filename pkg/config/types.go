
package config

// Config 是主配置结构
type Config struct {
	Global      GlobalConfig      `yaml:"global"`
	Inbound     InboundConfig     `yaml:"inbound"`
	Fingerprint FingerprintConfig `yaml:"fingerprint"`
	TLS         TLSConfig         `yaml:"tls"`
	Nodes       []NodeConfig      `yaml:"nodes"`
	ProxyIPs    []ProxyIPConfig   `yaml:"proxy_ips"`    // 新增：ProxyIP 列表
	ProxyIPOpts ProxyIPOptions    `yaml:"proxyip_opts"` // 新增：ProxyIP 选项
}

type GlobalConfig struct {
	LogLevel  string        `yaml:"log_level"`
	LogOutput string        `yaml:"log_output"`
	Metrics   MetricsConfig `yaml:"metrics"`
}

type MetricsConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint"`
}

type InboundConfig struct {
	SOCKS5 ListenConfig `yaml:"socks5"`
	HTTP   ListenConfig `yaml:"http"`
}

type ListenConfig struct {
	Listen   string `yaml:"listen"`
	Username string `yaml:"username"` // 新增：认证用户名
	Password string `yaml:"password"` // 新增：认证密码
}

type FingerprintConfig struct {
	Rotation RotationConfig `yaml:"rotation"`
}

type RotationConfig struct {
	Mode     string   `yaml:"mode"`
	Profile  string   `yaml:"profile"`
	Profiles []string `yaml:"profiles"`
	Interval string   `yaml:"interval"`
	Weights  []int    `yaml:"weights"` // 新增：加权模式的权重
}

type TLSConfig struct {
	VerifyMode string     `yaml:"verify_mode"`
	VerifyOpts VerifyOpts `yaml:"verify_opts"`
}

type VerifyOpts struct {
	CertPin  string `yaml:"cert_pin"`
	CustomCA string `yaml:"custom_ca"`
}

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
}

type TransportOpts struct {
	WSPath    string            `yaml:"ws_path"`
	WSHost    string            `yaml:"ws_host"`
	WSHeaders map[string]string `yaml:"ws_headers"`
	H2Path    string            `yaml:"h2_path"`
	// 新增：SOCKS5 出站配置
	SOCKS5Addr     string `yaml:"socks5_addr"`
	SOCKS5Username string `yaml:"socks5_username"`
	SOCKS5Password string `yaml:"socks5_password"`
}

type RetryOpts struct {
	MaxAttempts int    `yaml:"max_attempts"`
	BaseDelay   string `yaml:"base_delay"`
	MaxDelay    string `yaml:"max_delay"`
}

type PoolOpts struct {
	MaxIdle     int    `yaml:"max_idle"`
	MaxPerKey   int    `yaml:"max_per_key"`
	IdleTimeout string `yaml:"idle_timeout"`
	MaxLifetime string `yaml:"max_lifetime"`
}

// ============================================================
// 新增：ProxyIP 配置结构
// ============================================================

// ProxyIPConfig 单个 ProxyIP 配置
type ProxyIPConfig struct {
	Address  string `yaml:"address"`  // IP:Port 格式
	SNI      string `yaml:"sni"`      // 伪装的 SNI（用于夺舍）
	Weight   int    `yaml:"weight"`   // 权重（用于加权选择）
	Region   string `yaml:"region"`   // 地区标识
	Provider string `yaml:"provider"` // 提供商标识
	Enabled  bool   `yaml:"enabled"`  // 是否启用
}

// ProxyIPOptions ProxyIP 管理选项
type ProxyIPOptions struct {
	Enabled     bool   `yaml:"enabled"`      // 是否启用 ProxyIP 功能
	Mode        string `yaml:"mode"`         // 选择模式: round-robin, random, latency, weighted, failover
	CheckPeriod string `yaml:"check_period"` // 健康检查周期，如 "5m"
	Timeout     string `yaml:"timeout"`      // 连接超时，如 "10s"
	MaxFails    int    `yaml:"max_fails"`    // 最大失败次数后标记不可用
	AutoFetch   bool   `yaml:"auto_fetch"`   // 是否自动从远程获取 ProxyIP 列表
	FetchURL    string `yaml:"fetch_url"`    // 远程 ProxyIP 列表 URL
	FetchPeriod string `yaml:"fetch_period"` // 自动获取周期
}

// ============================================================
// 新增：SOCKS5 出站配置
// ============================================================

// SOCKS5OutConfig SOCKS5 出站代理配置
type SOCKS5OutConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Address  string `yaml:"address"`  // 上游 SOCKS5 代理地址
	Username string `yaml:"username"` // 用户名
	Password string `yaml:"password"` // 密码
}





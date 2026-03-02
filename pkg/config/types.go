package config

// Config 是主配置结构
type Config struct {
	Global      GlobalConfig      `yaml:"global"`
	Inbound     InboundConfig     `yaml:"inbound"`
	Fingerprint FingerprintConfig `yaml:"fingerprint"`
	TLS         TLSConfig         `yaml:"tls"`
	Nodes       []NodeConfig      `yaml:"nodes"`
	ProxyIPs    []ProxyIPConfig   `yaml:"proxy_ips"`
	ProxyIPOpts ProxyIPOptions    `yaml:"proxyip_opts"`
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
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type FingerprintConfig struct {
	Rotation RotationConfig `yaml:"rotation"`
}

type RotationConfig struct {
	Mode     string   `yaml:"mode"`
	Profile  string   `yaml:"profile"`
	Profiles []string `yaml:"profiles"`
	Interval string   `yaml:"interval"`
	Weights  []int    `yaml:"weights"`
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
	WSPath         string            `yaml:"ws_path"`
	WSHost         string            `yaml:"ws_host"`
	WSHeaders      map[string]string `yaml:"ws_headers"`
	H2Path         string            `yaml:"h2_path"`
	SOCKS5Addr     string            `yaml:"socks5_addr"`
	SOCKS5Username string            `yaml:"socks5_username"`
	SOCKS5Password string            `yaml:"socks5_password"`
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

// ProxyIPConfig 单个 ProxyIP 配置
type ProxyIPConfig struct {
	Address  string `yaml:"address"`
	SNI      string `yaml:"sni"`
	Weight   int    `yaml:"weight"`
	Region   string `yaml:"region"`
	Provider string `yaml:"provider"`
	Enabled  bool   `yaml:"enabled"`
}

// ProxyIPOptions ProxyIP 管理选项
type ProxyIPOptions struct {
	Enabled     bool   `yaml:"enabled"`
	Mode        string `yaml:"mode"`
	CheckPeriod string `yaml:"check_period"`
	Timeout     string `yaml:"timeout"`
	MaxFails    int    `yaml:"max_fails"`
	AutoFetch   bool   `yaml:"auto_fetch"`
	FetchURL    string `yaml:"fetch_url"`
	FetchPeriod string `yaml:"fetch_period"`
}

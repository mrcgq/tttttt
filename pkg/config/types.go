package config
 
// Config is the top-level configuration structure.
type Config struct {
	Global      GlobalConfig      `yaml:go-string">"global"`
	Inbound     InboundConfig     `yaml:go-string">"inbound"`
	Fingerprint FingerprintConfig `yaml:go-string">"fingerprint"`
	TLS         TLSConfig         `yaml:go-string">"tls"`
	Nodes       []NodeConfig      `yaml:go-string">"nodes"`
}
 
type GlobalConfig struct {
	LogLevel  string        `yaml:go-string">"log_level"`
	LogOutput string        `yaml:go-string">"log_output"`
	Metrics   MetricsConfig `yaml:go-string">"metrics"`
}
 
// MetricsConfig controls internal metrics collection.
type MetricsConfig struct {
	Enabled  bool   `yaml:go-string">"enabled"`
	Endpoint string `yaml:go-string">"endpoint"` // e.g. "127.0.0.1:9091"
}
 
type InboundConfig struct {
	SOCKS5 ListenConfig `yaml:go-string">"socks5"`
	HTTP   ListenConfig `yaml:go-string">"http"`
}
 
type ListenConfig struct {
	Listen string `yaml:go-string">"listen"`
}
 
type FingerprintConfig struct {
	Rotation RotationConfig `yaml:go-string">"rotation"`
}
 
type RotationConfig struct {
	Mode     string   `yaml:go-string">"mode"`     // fixed, random, per-domain, weighted, timed
	Profile  string   `yaml:go-string">"profile"`  // default profile for fixed mode
	Profiles []string `yaml:go-string">"profiles"` // profile list for rotation modes
	Interval string   `yaml:go-string">"interval"` // for timed mode, e.g. "5m"
}
 
type TLSConfig struct {
	VerifyMode string     `yaml:go-string">"verify_mode"`
	VerifyOpts VerifyOpts `yaml:go-string">"verify_opts"`
}
 
// VerifyOpts holds extra TLS verification options.
type VerifyOpts struct {
	CertPin    string `yaml:go-string">"cert_pin"`    // SHA256 hex of leaf cert for "pin" mode
	CustomCA   string `yaml:go-string">"custom_ca"`   // path to custom CA bundle
}
 
type NodeConfig struct {
	Name          string        `yaml:go-string">"name"`
	Address       string        `yaml:go-string">"address"`
	SNI           string        `yaml:go-string">"sni"`
	Fingerprint   string        `yaml:go-string">"fingerprint"`
	Active        bool          `yaml:go-string">"active"`
	Transport     string        `yaml:go-string">"transport"`
	TransportOpts TransportOpts `yaml:go-string">"transport_opts"`
	Fallback      []string      `yaml:go-string">"transport_fallback"`
	Retry         RetryOpts     `yaml:go-string">"retry"`
	Pool          PoolOpts      `yaml:go-string">"pool"`
}
 
type TransportOpts struct {
	WSPath    string            `yaml:go-string">"ws_path"`
	WSHost    string            `yaml:go-string">"ws_host"`
	WSHeaders map[string]string `yaml:go-string">"ws_headers"`
	H2Path    string            `yaml:go-string">"h2_path"`
}
 
// RetryOpts configures retry behavior per node.
type RetryOpts struct {
	MaxAttempts int    `yaml:go-string">"max_attempts"` // default 1 (no retry)
	BaseDelay   string `yaml:go-string">"base_delay"`   // e.g. "500ms"
	MaxDelay    string `yaml:go-string">"max_delay"`    // e.g. "10s"
}
 
// PoolOpts configures connection pooling per node.
type PoolOpts struct {
	MaxIdle     int    `yaml:go-string">"max_idle"`
	MaxPerKey   int    `yaml:go-string">"max_per_key"`
	IdleTimeout string `yaml:go-string">"idle_timeout"` // e.g. "120s"
	MaxLifetime string `yaml:go-string">"max_lifetime"` // e.g. "10m"
}



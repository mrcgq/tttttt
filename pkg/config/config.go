package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var envVarRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

func Load(path string) (*Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("config: stat %s: %w", path, err)
	}
	if info.Mode().Perm()&0077 != 0 {
		fmt.Fprintf(os.Stderr, "WARNING: config file %s has permissions %o, recommend 0600\n",
			path, info.Mode().Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	expanded := expandEnvVars(string(data))

	cfg := &Config{}
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("config: validate: %w", err)
	}

	return cfg, nil
}

func expandEnvVars(s string) string {
	return envVarRegex.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return match
	})
}

func applyDefaults(cfg *Config) {
	// Global 默认值
	if cfg.Global.LogLevel == "" {
		cfg.Global.LogLevel = "info"
	}
	if cfg.Global.LogOutput == "" {
		cfg.Global.LogOutput = "stderr"
	}

	// Inbound 默认值
	if cfg.Inbound.SOCKS5.Listen == "" {
		cfg.Inbound.SOCKS5.Listen = "127.0.0.1:1080"
	}

	// Fingerprint 默认值
	if cfg.Fingerprint.Rotation.Mode == "" {
		cfg.Fingerprint.Rotation.Mode = "fixed"
	}
	if cfg.Fingerprint.Rotation.Profile == "" {
		cfg.Fingerprint.Rotation.Profile = "chrome-126-win"
	}

	// TLS 默认值
	if cfg.TLS.VerifyMode == "" {
		cfg.TLS.VerifyMode = "sni-skip"
	}

	// ClientBehavior 默认值
	if cfg.ClientBehavior.Cadence.Mode == "" {
		cfg.ClientBehavior.Cadence.Mode = "none"
	}
	if cfg.ClientBehavior.Cadence.Jitter == 0 {
		cfg.ClientBehavior.Cadence.Jitter = 0.3
	}
	if cfg.ClientBehavior.MaxRedirects == 0 {
		cfg.ClientBehavior.MaxRedirects = 10
	}

	// API 默认值
	if cfg.API.Listen == "" {
		cfg.API.Listen = "127.0.0.1:9090"
	}

	// Health 默认值
	if cfg.Health.Interval == "" {
		cfg.Health.Interval = "5m"
	}
	if cfg.Health.Timeout == "" {
		cfg.Health.Timeout = "10s"
	}
	if cfg.Health.Threshold == 0 {
		cfg.Health.Threshold = 3
	}
	if cfg.Health.DegradedMs == 0 {
		cfg.Health.DegradedMs = 500
	}
	if cfg.Health.TestURL == "" {
		cfg.Health.TestURL = "http://www.gstatic.com/generate_204"
	}

	// ProxyIPs 默认值
	if cfg.ProxyIPs.Mode == "" {
		cfg.ProxyIPs.Mode = "latency"
	}
	if cfg.ProxyIPs.Options.CheckPeriod == "" {
		cfg.ProxyIPs.Options.CheckPeriod = "5m"
	}
	if cfg.ProxyIPs.Options.Timeout == "" {
		cfg.ProxyIPs.Options.Timeout = "10s"
	}
	if cfg.ProxyIPs.Options.MaxFails == 0 {
		cfg.ProxyIPs.Options.MaxFails = 3
	}

	// Node 默认值
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Transport == "" {
			cfg.Nodes[i].Transport = "raw"
		}

		// Retry.Jitter 默认值
		if cfg.Nodes[i].Retry.Jitter == 0 {
			cfg.Nodes[i].Retry.Jitter = 0.2
		}

		// Pool 默认值
		if cfg.Nodes[i].Pool.MaxIdle == 0 {
			cfg.Nodes[i].Pool.MaxIdle = 10
		}
		if cfg.Nodes[i].Pool.MaxPerKey == 0 {
			cfg.Nodes[i].Pool.MaxPerKey = 3
		}
		if cfg.Nodes[i].Pool.IdleTimeout == "" {
			cfg.Nodes[i].Pool.IdleTimeout = "120s"
		}
		if cfg.Nodes[i].Pool.MaxLifetime == "" {
			cfg.Nodes[i].Pool.MaxLifetime = "10m"
		}
	}

	// ProxyIP entries 默认值
	for i := range cfg.ProxyIPs.Entries {
		if cfg.ProxyIPs.Entries[i].Weight == 0 {
			cfg.ProxyIPs.Entries[i].Weight = 1
		}
	}
}

func validate(cfg *Config) error {
	if len(cfg.Nodes) == 0 {
		return fmt.Errorf("at least one node must be defined")
	}

	// 验证日志级别
	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if !validLogLevels[cfg.Global.LogLevel] {
		return fmt.Errorf("global.log_level: unknown level %q", cfg.Global.LogLevel)
	}

	// 验证TLS模式
	validVerifyModes := map[string]bool{
		"strict": true, "sni-skip": true, "insecure": true, "pin": true,
	}
	if !validVerifyModes[cfg.TLS.VerifyMode] {
		return fmt.Errorf("tls.verify_mode: unknown mode %q", cfg.TLS.VerifyMode)
	}

	// 验证指纹轮换模式
	validRotationModes := map[string]bool{
		"fixed": true, "random": true, "per-domain": true,
		"weighted": true, "timed": true,
	}
	if !validRotationModes[cfg.Fingerprint.Rotation.Mode] {
		return fmt.Errorf("fingerprint.rotation.mode: unknown mode %q",
			cfg.Fingerprint.Rotation.Mode)
	}

	// 验证时序模式
	validCadenceModes := map[string]bool{
		"none": true, "browsing": true, "fast": true, "aggressive": true,
		"random": true, "custom": true, "sequence": true,
	}
	if cfg.ClientBehavior.Cadence.Mode != "" && !validCadenceModes[cfg.ClientBehavior.Cadence.Mode] {
		return fmt.Errorf("client_behavior.cadence.mode: unknown mode %q",
			cfg.ClientBehavior.Cadence.Mode)
	}

	// 验证ProxyIP选择模式
	validProxyIPModes := map[string]bool{
		"round-robin": true, "random": true, "latency": true,
		"weighted": true, "failover": true,
	}
	if cfg.ProxyIPs.Enabled && !validProxyIPModes[cfg.ProxyIPs.Mode] {
		return fmt.Errorf("proxy_ips.mode: unknown mode %q", cfg.ProxyIPs.Mode)
	}

	// 验证节点配置
	names := make(map[string]bool)
	for i, n := range cfg.Nodes {
		if n.Name == "" {
			return fmt.Errorf("nodes[%d]: name is required", i)
		}
		if names[n.Name] {
			return fmt.Errorf("nodes[%d]: duplicate name %q", i, n.Name)
		}
		names[n.Name] = true
		if n.Address == "" {
			return fmt.Errorf("node %q: address is required", n.Name)
		}
		if n.SNI == "" {
			return fmt.Errorf("node %q: sni is required", n.Name)
		}

		validTransports := map[string]bool{
			"raw": true, "ws": true, "h2": true,
			"websocket": true, "http2": true,
			"direct": true, "tcp": true,
			"socks5-out": true, "socks5out": true,
		}
		normalized := strings.ToLower(n.Transport)
		if !validTransports[normalized] {
			return fmt.Errorf("node %q: unknown transport %q", n.Name, n.Transport)
		}

		// 当使用 socks5-out 时，必须配置 socks5_addr
		if (normalized == "socks5-out" || normalized == "socks5out") &&
			n.TransportOpts.SOCKS5Addr == "" {
			return fmt.Errorf("node %q: transport socks5-out requires transport_opts.socks5_addr", n.Name)
		}

		// 验证 Jitter 范围
		if n.Retry.Jitter < 0 || n.Retry.Jitter > 1 {
			return fmt.Errorf("node %q: retry.jitter must be between 0 and 1, got %f",
				n.Name, n.Retry.Jitter)
		}
	}

	return nil
}

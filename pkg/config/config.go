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
	if cfg.Global.LogLevel == "" {
		cfg.Global.LogLevel = "info"
	}
	if cfg.Global.LogOutput == "" {
		cfg.Global.LogOutput = "stderr"
	}
	if cfg.Inbound.SOCKS5.Listen == "" {
		cfg.Inbound.SOCKS5.Listen = "127.0.0.1:1080"
	}
	if cfg.Fingerprint.Rotation.Mode == "" {
		cfg.Fingerprint.Rotation.Mode = "fixed"
	}
	if cfg.Fingerprint.Rotation.Profile == "" {
		cfg.Fingerprint.Rotation.Profile = "chrome-126-win"
	}
	if cfg.TLS.VerifyMode == "" {
		cfg.TLS.VerifyMode = "sni-skip"
	}
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Transport == "" {
			cfg.Nodes[i].Transport = "raw"
		}
	}
}

func validate(cfg *Config) error {
	if len(cfg.Nodes) == 0 {
		return fmt.Errorf("at least one node must be defined")
	}

	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if !validLogLevels[cfg.Global.LogLevel] {
		return fmt.Errorf("global.log_level: unknown level %q", cfg.Global.LogLevel)
	}

	validVerifyModes := map[string]bool{
		"strict": true, "sni-skip": true, "insecure": true, "pin": true,
	}
	if !validVerifyModes[cfg.TLS.VerifyMode] {
		return fmt.Errorf("tls.verify_mode: unknown mode %q", cfg.TLS.VerifyMode)
	}

	validRotationModes := map[string]bool{
		"fixed": true, "random": true, "per-domain": true,
		"weighted": true, "timed": true,
	}
	if !validRotationModes[cfg.Fingerprint.Rotation.Mode] {
		return fmt.Errorf("fingerprint.rotation.mode: unknown mode %q",
			cfg.Fingerprint.Rotation.Mode)
	}

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
		}
		normalized := strings.ToLower(n.Transport)
		if !validTransports[normalized] {
			return fmt.Errorf("node %q: unknown transport %q", n.Name, n.Transport)
		}
	}
	return nil
}

func (cfg *Config) ActiveNode() *NodeConfig {
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Active {
			return &cfg.Nodes[i]
		}
	}
	if len(cfg.Nodes) > 0 {
		return &cfg.Nodes[0]
	}
	return nil
}

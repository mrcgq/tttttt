package config
 
import (
	go-string">"fmt"
	go-string">"os"
	go-string">"regexp"
	go-string">"strings"
 
	go-string">"gopkg.in/yaml.v3"
)
 
var envVarRegex = regexp.MustCompile(`\$\{([^}]+)\}`)
 
// Load reads and parses a YAML config file.
func Load(path string) (*Config, error) {
	// Check file permissions (warn if too open)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf(go-string">"config: stat %s: %w", path, err)
	}
	if info.Mode().Perm()&go-number">0077 != go-number">0 {
		fmt.Fprintf(os.Stderr, go-string">"WARNING: config file %s has permissions %o, recommend go-number">0600\n",
			path, info.Mode().Perm())
	}
 
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf(go-string">"config: read %s: %w", path, err)
	}
 
	// Expand environment variables
	expanded := expandEnvVars(string(data))
 
	cfg := &Config{}
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf(go-string">"config: parse %s: %w", path, err)
	}
 
	applyDefaults(cfg)
 
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf(go-string">"config: validate: %w", err)
	}
 
	return cfg, nil
}
 
// expandEnvVars replaces ${VAR_NAME} with the environment variable value.
func expandEnvVars(s string) string {
	return envVarRegex.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[go-number">2 : len(match)-go-number">1] // strip ${ and }
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return match // leave unchanged if not set
	})
}
 
func applyDefaults(cfg *Config) {
	if cfg.Global.LogLevel == go-string">"" {
		cfg.Global.LogLevel = go-string">"info"
	}
	if cfg.Global.LogOutput == go-string">"" {
		cfg.Global.LogOutput = go-string">"stderr"
	}
	if cfg.Inbound.SOCKS5.Listen == go-string">"" {
		cfg.Inbound.SOCKS5.Listen = go-string">"go-number">127.0.go-number">0.1:go-number">1080"
	}
	if cfg.Fingerprint.Rotation.Mode == go-string">"" {
		cfg.Fingerprint.Rotation.Mode = go-string">"fixed"
	}
	if cfg.Fingerprint.Rotation.Profile == go-string">"" {
		cfg.Fingerprint.Rotation.Profile = go-string">"chrome-go-number">126-win"
	}
	if cfg.TLS.VerifyMode == go-string">"" {
		cfg.TLS.VerifyMode = go-string">"sni-skip"
	}
 
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Transport == go-string">"" {
			cfg.Nodes[i].Transport = go-string">"raw"
		}
	}
}
 
func validate(cfg *Config) error {
	if len(cfg.Nodes) == go-number">0 {
		return fmt.Errorf(go-string">"at least one node must be defined")
	}
 
	validLogLevels := map[string]bool{
		go-string">"debug": true, go-string">"info": true, go-string">"warn": true, go-string">"error": true,
	}
	if !validLogLevels[cfg.Global.LogLevel] {
		return fmt.Errorf(go-string">"global.log_level: unknown level %q", cfg.Global.LogLevel)
	}
 
	validVerifyModes := map[string]bool{
		go-string">"strict": true, go-string">"sni-skip": true, go-string">"insecure": true, go-string">"pin": true,
	}
	if !validVerifyModes[cfg.TLS.VerifyMode] {
		return fmt.Errorf(go-string">"tls.verify_mode: unknown mode %q", cfg.TLS.VerifyMode)
	}
 
	validRotationModes := map[string]bool{
		go-string">"fixed": true, go-string">"random": true, go-string">"per-domain": true,
		go-string">"weighted": true, go-string">"timed": true,
	}
	if !validRotationModes[cfg.Fingerprint.Rotation.Mode] {
		return fmt.Errorf(go-string">"fingerprint.rotation.mode: unknown mode %q",
			cfg.Fingerprint.Rotation.Mode)
	}
 
	names := make(map[string]bool)
	for i, n := range cfg.Nodes {
		if n.Name == go-string">"" {
			return fmt.Errorf(go-string">"nodes[%d]: name is required", i)
		}
		if names[n.Name] {
			return fmt.Errorf(go-string">"nodes[%d]: duplicate name %q", i, n.Name)
		}
		names[n.Name] = true
		if n.Address == go-string">"" {
			return fmt.Errorf(go-string">"node %q: address is required", n.Name)
		}
		if n.SNI == go-string">"" {
			return fmt.Errorf(go-string">"node %q: sni is required", n.Name)
		}
 
		validTransports := map[string]bool{
			go-string">"raw": true, go-string">"ws": true, go-string">"h2": true,
			go-string">"websocket": true, go-string">"http2": true,
			go-string">"direct": true, go-string">"tcp": true,
		}
		normalized := strings.ToLower(n.Transport)
		if !validTransports[normalized] {
			return fmt.Errorf(go-string">"node %q: unknown transport %q", n.Name, n.Transport)
		}
	}
	return nil
}
 
// ActiveNode returns the first node marked active, or the first node.
func (cfg *Config) ActiveNode() *NodeConfig {
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Active {
			return &cfg.Nodes[i]
		}
	}
	if len(cfg.Nodes) > go-number">0 {
		return &cfg.Nodes[go-number">0]
	}
	return nil
}


package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/user/tls-client/pkg/config"
	"github.com/user/tls-client/pkg/engine"
	"github.com/user/tls-client/pkg/fingerprint"
	"github.com/user/tls-client/pkg/health"
	"github.com/user/tls-client/pkg/transport"
)

// ConfigUpdater 配置更新接口
type ConfigUpdater interface {
	Get() *config.Config
	Update(cfg *config.Config) error
}

// Server is the management API HTTP server with embedded WebUI.
type Server struct {
	Addr      string
	Token     string
	Logger    *zap.Logger
	Checker   *health.Checker
	StartTime time.Time

	server   *http.Server
	requests int64

	// 配置管理器
	configManager ConfigUpdater

	// 引擎状态
	mu             sync.RWMutex
	engineRunning  bool
	activeConns    int64
	totalBytes     int64
	currentProfile string
	currentNode    string
}

// NewServer creates a management API server.
func NewServer(addr, token string, logger *zap.Logger) *Server {
	return &Server{
		Addr:           addr,
		Token:          token,
		Logger:         logger,
		StartTime:      time.Now(),
		currentProfile: fingerprint.DefaultProfile(),
	}
}

// SetHealthChecker attaches a health checker for /api/proxies.
func (s *Server) SetHealthChecker(c *health.Checker) {
	s.Checker = c
}

// SetConfigManager 设置配置管理器
func (s *Server) SetConfigManager(cm ConfigUpdater) {
	s.configManager = cm
}

// SetCurrentNode sets the active node name.
func (s *Server) SetCurrentNode(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentNode = name
}

// SetCurrentProfile sets the active profile name.
func (s *Server) SetCurrentProfile(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentProfile = name
}

// SetEngineRunning sets the engine running state.
func (s *Server) SetEngineRunning(running bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.engineRunning = running
}

// AddConnection increments connection count.
func (s *Server) AddConnection() {
	atomic.AddInt64(&s.activeConns, 1)
}

// RemoveConnection decrements connection count.
func (s *Server) RemoveConnection() {
	atomic.AddInt64(&s.activeConns, -1)
}

// AddBytes adds to total bytes transferred.
func (s *Server) AddBytes(n int64) {
	atomic.AddInt64(&s.totalBytes, n)
}

// Start begins serving the management API and WebUI.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// ==================== API 端点 ====================
	mux.HandleFunc("/api/status", s.auth(s.handleStatus))
	mux.HandleFunc("/api/proxies", s.auth(s.handleProxies))
	mux.HandleFunc("/api/fingerprints", s.auth(s.handleFingerprints))
	mux.HandleFunc("/api/transports", s.auth(s.handleTransports))
	mux.HandleFunc("/api/dial-metrics", s.auth(s.handleDialMetrics))

	// 引擎控制端点
	mux.HandleFunc("/api/start", s.auth(s.handleStart))
	mux.HandleFunc("/api/stop", s.auth(s.handleStop))
	mux.HandleFunc("/api/reload", s.auth(s.handleReload))
	mux.HandleFunc("/api/config", s.auth(s.handleConfig))

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// ==================== WebUI 静态文件 ====================
	webFS, err := fs.Sub(WebUIFS, "webui")
	if err != nil {
		s.Logger.Warn("webui not embedded", zap.Error(err))
	} else {
		fileServer := http.FileServer(http.FS(webFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			if r.URL.Path == "/" {
				r.URL.Path = "/index.html"
			}
			fileServer.ServeHTTP(w, r)
		})
		s.Logger.Info("webui enabled", zap.String("url", fmt.Sprintf("http://%s", s.Addr)))
	}

	s.server = &http.Server{
		Addr:         s.Addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.Logger.Info("api server started", zap.String("addr", s.Addr))

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.Logger.Error("api server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop gracefully shuts down the API server.
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		atomic.AddInt64(&s.requests, 1)

		if s.Token != "" {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || auth[7:] != s.Token {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	s.mu.RLock()
	engineRunning := s.engineRunning
	currentProfile := s.currentProfile
	currentNode := s.currentNode
	s.mu.RUnlock()

	status := map[string]any{
		"engine_running":  engineRunning,
		"uptime_seconds":  int(time.Since(s.StartTime).Seconds()),
		"uptime_human":    time.Since(s.StartTime).Round(time.Second).String(),
		"goroutines":      runtime.NumGoroutine(),
		"active_conns":    atomic.LoadInt64(&s.activeConns),
		"total_bytes":     atomic.LoadInt64(&s.totalBytes),
		"current_profile": currentProfile,
		"current_node":    currentNode,
		"memory": map[string]any{
			"alloc_mb":       mem.Alloc / 1024 / 1024,
			"total_alloc_mb": mem.TotalAlloc / 1024 / 1024,
			"sys_mb":         mem.Sys / 1024 / 1024,
			"gc_cycles":      mem.NumGC,
		},
		"api_requests":   atomic.LoadInt64(&s.requests),
		"go_version":     runtime.Version(),
		"profiles_count": fingerprint.Count(),
		"version":        "3.5.0",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (s *Server) handleProxies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.Checker == nil {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "health checker not configured"})
		return
	}

	allHealth := s.Checker.GetAllHealth()
	proxies := make([]map[string]any, 0, len(allHealth))
	for _, h := range allHealth {
		proxies = append(proxies, map[string]any{
			"name":                 h.Name,
			"status":               h.Status.String(),
			"latency_ms":           h.Latency.Milliseconds(),
			"last_check":           h.LastCheck.Format(time.RFC3339),
			"last_success":         h.LastSuccess.Format(time.RFC3339),
			"consecutive_failures": h.ConsecFailures,
			"total_checks":         h.TotalChecks,
			"total_successes":      h.TotalSuccesses,
			"total_failures":       h.TotalFailures,
			"last_error":           h.LastError,
		})
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"proxies":   proxies,
		"best_node": s.Checker.BestNode(),
	})
}

func (s *Server) handleFingerprints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	profiles := make([]map[string]any, 0)
	for _, name := range fingerprint.List() {
		p := fingerprint.Get(name)
		profiles = append(profiles, map[string]any{
			"name":           p.Name,
			"browser":        p.Browser,
			"platform":       p.Platform,
			"version":        p.Version,
			"tags":           p.Tags,
			"h2_fingerprint": p.H2Fingerprint(),
			"ja4h":           fingerprint.ComputeJA4H(p),
			"user_agent":     p.UserAgent,
		})
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"profiles": profiles,
		"count":    len(profiles),
		"default":  fingerprint.DefaultProfile(),
	})
}

func (s *Server) handleTransports(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var transports []map[string]any
	for _, name := range transport.Names() {
		t := transport.Get(name)
		info := t.Info()
		transports = append(transports, map[string]any{
			"name":               name,
			"supports_multiplex": info.SupportsMultiplex,
			"supports_binary":    info.SupportsBinary,
			"requires_upgrade":   info.RequiresUpgrade,
			"max_frame_size":     info.MaxFrameSize,
			"alpn":               t.ALPNProtos(),
		})
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"transports": transports})
}

func (s *Server) handleDialMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	metrics := engine.GetDialMetrics()
	avgLatencyMs := int64(0)
	if metrics.SuccessCount > 0 {
		avgLatencyMs = (metrics.TotalLatency / metrics.SuccessCount) / int64(time.Millisecond)
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success_count":  metrics.SuccessCount,
		"failure_count":  metrics.FailureCount,
		"avg_latency_ms": avgLatencyMs,
		"success_rate":   fmt.Sprintf("%.1f%%", successRate(metrics.SuccessCount, metrics.FailureCount)),
	})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	s.mu.Lock()
	s.engineRunning = true
	s.mu.Unlock()

	s.Logger.Info("engine start requested via API")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "引擎启动命令已接收"})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	s.mu.Lock()
	s.engineRunning = false
	s.mu.Unlock()

	s.Logger.Info("engine stop requested via API")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "引擎停止命令已接收"})
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if s.configManager != nil {
		currentCfg := s.configManager.Get()
		if currentCfg != nil {
			if err := s.configManager.Update(currentCfg); err != nil {
				s.Logger.Error("reload failed", zap.Error(err))
				_ = json.NewEncoder(w).Encode(map[string]any{
					"success": false,
					"message": fmt.Sprintf("重载失败: %v", err),
				})
				return
			}
		}
	}

	s.Logger.Info("config reload requested via API")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "配置重载命令已接收"})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		if s.configManager == nil {
			s.mu.RLock()
			basicConfig := map[string]any{
				"current_profile": s.currentProfile,
				"current_node":    s.currentNode,
				"engine_running":  s.engineRunning,
			}
			s.mu.RUnlock()
			_ = json.NewEncoder(w).Encode(basicConfig)
			return
		}

		cfg := s.configManager.Get()
		if cfg == nil {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "no configuration loaded"})
			return
		}

		fullConfig := configToJSON(cfg)
		s.mu.RLock()
		fullConfig["current_profile"] = s.currentProfile
		fullConfig["current_node"] = s.currentNode
		fullConfig["engine_running"] = s.engineRunning
		s.mu.RUnlock()

		_ = json.NewEncoder(w).Encode(fullConfig)

	case http.MethodPost:
		if s.configManager == nil {
			http.Error(w, "config manager not available", http.StatusServiceUnavailable)
			return
		}

		var newConfigJSON map[string]any
		if err := json.NewDecoder(r.Body).Decode(&newConfigJSON); err != nil {
			http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		newCfg, err := jsonToConfig(newConfigJSON)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid config: %v", err), http.StatusBadRequest)
			return
		}

		if err := s.configManager.Update(newCfg); err != nil {
			s.Logger.Error("config update failed", zap.Error(err))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"message": fmt.Sprintf("配置更新失败: %v", err),
			})
			return
		}

		s.mu.Lock()
		if profile, ok := newConfigJSON["current_profile"].(string); ok && profile != "" {
			s.currentProfile = profile
		}
		if nodes, ok := newConfigJSON["nodes"].([]any); ok {
			for _, n := range nodes {
				if node, ok := n.(map[string]any); ok {
					if active, ok := node["active"].(bool); ok && active {
						if name, ok := node["name"].(string); ok {
							s.currentNode = name
						}
						break
					}
				}
			}
		}
		s.mu.Unlock()

		s.Logger.Info("config updated via API",
			zap.Int("nodes_count", len(newCfg.Nodes)),
			zap.String("socks5_listen", newCfg.Inbound.SOCKS5.Listen),
		)

		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "配置已更新并触发重载",
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ================================================================
// configToJSON: Config → JSON map
// 字段名严格对齐 pkg/config/types.go
// ================================================================
func configToJSON(cfg *config.Config) map[string]any {
	nodes := make([]map[string]any, 0, len(cfg.Nodes))
	for _, n := range cfg.Nodes {
		wsHeaders := n.TransportOpts.WSHeaders
		if wsHeaders == nil {
			wsHeaders = make(map[string]string)
		}

		node := map[string]any{
			"name":        n.Name,
			"address":     n.Address,
			"sni":         n.SNI,
			"transport":   n.Transport,
			"fingerprint": n.Fingerprint,
			"active":      n.Active,
			"transport_opts": map[string]any{
				"ws_path":         n.TransportOpts.WSPath,
				"ws_host":         n.TransportOpts.WSHost,
				"ws_headers":      wsHeaders,
				"h2_path":         n.TransportOpts.H2Path,
				"socks5_addr":     n.TransportOpts.SOCKS5Addr,
				"socks5_username": n.TransportOpts.SOCKS5Username,
				"socks5_password": n.TransportOpts.SOCKS5Password,
			},
			"transport_fallback": n.Fallback,
			"remote_proxy": map[string]any{
				"socks5":   n.RemoteProxy.SOCKS5,
				"fallback": n.RemoteProxy.Fallback,
			},
			"retry": map[string]any{
				"max_attempts": n.Retry.MaxAttempts,
				"base_delay":   n.Retry.BaseDelay,
				"max_delay":    n.Retry.MaxDelay,
				"jitter":       n.Retry.Jitter,
			},
			"pool": map[string]any{
				"max_idle":     n.Pool.MaxIdle,
				"max_per_key":  n.Pool.MaxPerKey,
				"idle_timeout": n.Pool.IdleTimeout,
				"max_lifetime": n.Pool.MaxLifetime,
			},
		}
		nodes = append(nodes, node)
	}

	// proxy_ips entries
	proxyIPEntries := make([]map[string]any, 0, len(cfg.ProxyIPs.Entries))
	for _, e := range cfg.ProxyIPs.Entries {
		proxyIPEntries = append(proxyIPEntries, map[string]any{
			"address":  e.Address,
			"sni":      e.SNI,
			"weight":   e.Weight,
			"region":   e.Region,
			"provider": e.Provider,
			"enabled":  e.Enabled,
		})
	}

	// fingerprint weights ([]int)
	fpWeights := cfg.Fingerprint.Rotation.Weights
	if fpWeights == nil {
		fpWeights = []int{}
	}

	return map[string]any{
		"global": map[string]any{
			"log_level":  cfg.Global.LogLevel,
			"log_output": cfg.Global.LogOutput,
			"metrics": map[string]any{
				"enabled":  cfg.Global.Metrics.Enabled,
				"endpoint": cfg.Global.Metrics.Endpoint,
			},
		},
		"inbound": map[string]any{
			"socks5": map[string]any{
				"listen":   cfg.Inbound.SOCKS5.Listen,
				"username": cfg.Inbound.SOCKS5.Username,
				"password": cfg.Inbound.SOCKS5.Password,
			},
			"http": map[string]any{
				"listen": cfg.Inbound.HTTP.Listen,
			},
		},
		"fingerprint": map[string]any{
			"rotation": map[string]any{
				"mode":     cfg.Fingerprint.Rotation.Mode,
				"profile":  cfg.Fingerprint.Rotation.Profile,
				"profiles": cfg.Fingerprint.Rotation.Profiles,
				"interval": cfg.Fingerprint.Rotation.Interval,
				"weights":  fpWeights,
			},
		},
		"tls": map[string]any{
			"verify_mode": cfg.TLS.VerifyMode,
			"verify_opts": map[string]any{
				"cert_pin":  cfg.TLS.VerifyOpts.CertPin,
				"custom_ca": cfg.TLS.VerifyOpts.CustomCA,
			},
		},
		"client_behavior": map[string]any{
			"cadence": map[string]any{
				"mode":      cfg.ClientBehavior.Cadence.Mode,
				"min_delay": cfg.ClientBehavior.Cadence.MinDelay,
				"max_delay": cfg.ClientBehavior.Cadence.MaxDelay,
				"jitter":    cfg.ClientBehavior.Cadence.Jitter,
				"sequence":  cfg.ClientBehavior.Cadence.Sequence,
			},
			"cookies": map[string]any{
				"enabled":           cfg.ClientBehavior.Cookies.Enabled,
				"clear_on_rotation": cfg.ClientBehavior.Cookies.ClearOnRotation,
			},
			"follow_redirects": cfg.ClientBehavior.FollowRedirects,
			"max_redirects":    cfg.ClientBehavior.MaxRedirects,
		},
		"api": map[string]any{
			"enabled": cfg.API.Enabled,
			"listen":  cfg.API.Listen,
			"token":   cfg.API.Token,
		},
		"health": map[string]any{
			"enabled":     cfg.Health.Enabled,
			"interval":    cfg.Health.Interval,
			"timeout":     cfg.Health.Timeout,
			"threshold":   cfg.Health.Threshold,
			"degraded_ms": cfg.Health.DegradedMs,
			"test_url":    cfg.Health.TestURL,
		},
		"proxy_ips": map[string]any{
			"enabled": cfg.ProxyIPs.Enabled,
			"mode":    cfg.ProxyIPs.Mode,
			"options": map[string]any{
				"check_period": cfg.ProxyIPs.Options.CheckPeriod,
				"timeout":      cfg.ProxyIPs.Options.Timeout,
				"max_fails":    cfg.ProxyIPs.Options.MaxFails,
			},
			"entries": proxyIPEntries,
		},
		"nodes": nodes,
	}
}

// ================================================================
// jsonToConfig: JSON map → Config
// 字段名严格对齐 pkg/config/types.go
// ================================================================
func jsonToConfig(data map[string]any) (*config.Config, error) {
	cfg := &config.Config{}

	// ---- global ----
	if global, ok := data["global"].(map[string]any); ok {
		if v, ok := global["log_level"].(string); ok {
			cfg.Global.LogLevel = v
		}
		if v, ok := global["log_output"].(string); ok {
			cfg.Global.LogOutput = v
		}
		if metrics, ok := global["metrics"].(map[string]any); ok {
			if v, ok := metrics["enabled"].(bool); ok {
				cfg.Global.Metrics.Enabled = v
			}
			if v, ok := metrics["endpoint"].(string); ok {
				cfg.Global.Metrics.Endpoint = v
			}
		}
	}

	// ---- inbound ----
	if inbound, ok := data["inbound"].(map[string]any); ok {
		if socks5, ok := inbound["socks5"].(map[string]any); ok {
			if v, ok := socks5["listen"].(string); ok {
				cfg.Inbound.SOCKS5.Listen = v
			}
			if v, ok := socks5["username"].(string); ok {
				cfg.Inbound.SOCKS5.Username = v
			}
			if v, ok := socks5["password"].(string); ok {
				cfg.Inbound.SOCKS5.Password = v
			}
		}
		if httpCfg, ok := inbound["http"].(map[string]any); ok {
			if v, ok := httpCfg["listen"].(string); ok {
				cfg.Inbound.HTTP.Listen = v
			}
		}
	}

	// ---- fingerprint ----
	if fp, ok := data["fingerprint"].(map[string]any); ok {
		if rotation, ok := fp["rotation"].(map[string]any); ok {
			if v, ok := rotation["mode"].(string); ok {
				cfg.Fingerprint.Rotation.Mode = v
			}
			if v, ok := rotation["profile"].(string); ok {
				cfg.Fingerprint.Rotation.Profile = v
			}
			if v, ok := rotation["profiles"].([]any); ok {
				cfg.Fingerprint.Rotation.Profiles = nil
				for _, p := range v {
					if s, ok := p.(string); ok {
						cfg.Fingerprint.Rotation.Profiles = append(cfg.Fingerprint.Rotation.Profiles, s)
					}
				}
			}
			if v, ok := rotation["interval"].(string); ok {
				cfg.Fingerprint.Rotation.Interval = v
			}
			// weights: []int
			if weights, ok := rotation["weights"].([]any); ok {
				cfg.Fingerprint.Rotation.Weights = nil
				for _, w := range weights {
					if f, ok := w.(float64); ok {
						cfg.Fingerprint.Rotation.Weights = append(cfg.Fingerprint.Rotation.Weights, int(f))
					}
				}
			}
		}
	}

	// ---- tls ----
	if tlsCfg, ok := data["tls"].(map[string]any); ok {
		if v, ok := tlsCfg["verify_mode"].(string); ok {
			cfg.TLS.VerifyMode = v
		}
		if opts, ok := tlsCfg["verify_opts"].(map[string]any); ok {
			if v, ok := opts["cert_pin"].(string); ok {
				cfg.TLS.VerifyOpts.CertPin = v
			}
			if v, ok := opts["custom_ca"].(string); ok {
				cfg.TLS.VerifyOpts.CustomCA = v
			}
		}
	}

	// ---- client_behavior ----
	if cb, ok := data["client_behavior"].(map[string]any); ok {
		if cadence, ok := cb["cadence"].(map[string]any); ok {
			if v, ok := cadence["mode"].(string); ok {
				cfg.ClientBehavior.Cadence.Mode = v
			}
			if v, ok := cadence["min_delay"].(string); ok {
				cfg.ClientBehavior.Cadence.MinDelay = v
			}
			if v, ok := cadence["max_delay"].(string); ok {
				cfg.ClientBehavior.Cadence.MaxDelay = v
			}
			if v, ok := cadence["jitter"].(float64); ok {
				cfg.ClientBehavior.Cadence.Jitter = v
			}
			if seq, ok := cadence["sequence"].([]any); ok {
				cfg.ClientBehavior.Cadence.Sequence = nil
				for _, item := range seq {
					if s, ok := item.(string); ok {
						cfg.ClientBehavior.Cadence.Sequence = append(cfg.ClientBehavior.Cadence.Sequence, s)
					}
				}
			}
		}
		if cookies, ok := cb["cookies"].(map[string]any); ok {
			if v, ok := cookies["enabled"].(bool); ok {
				cfg.ClientBehavior.Cookies.Enabled = v
			}
			if v, ok := cookies["clear_on_rotation"].(bool); ok {
				cfg.ClientBehavior.Cookies.ClearOnRotation = v
			}
		}
		if v, ok := cb["follow_redirects"].(bool); ok {
			cfg.ClientBehavior.FollowRedirects = v
		}
		if v, ok := cb["max_redirects"].(float64); ok {
			cfg.ClientBehavior.MaxRedirects = int(v)
		}
	}

	// ---- api ----
	if apiCfg, ok := data["api"].(map[string]any); ok {
		if v, ok := apiCfg["enabled"].(bool); ok {
			cfg.API.Enabled = v
		}
		if v, ok := apiCfg["listen"].(string); ok {
			cfg.API.Listen = v
		}
		if v, ok := apiCfg["token"].(string); ok {
			cfg.API.Token = v
		}
	}

	// ---- health ----
	if healthCfg, ok := data["health"].(map[string]any); ok {
		if v, ok := healthCfg["enabled"].(bool); ok {
			cfg.Health.Enabled = v
		}
		if v, ok := healthCfg["interval"].(string); ok {
			cfg.Health.Interval = v
		}
		if v, ok := healthCfg["timeout"].(string); ok {
			cfg.Health.Timeout = v
		}
		if v, ok := healthCfg["threshold"].(float64); ok {
			cfg.Health.Threshold = int(v)
		}
		if v, ok := healthCfg["degraded_ms"].(float64); ok {
			cfg.Health.DegradedMs = int64(v)
		}
		if v, ok := healthCfg["test_url"].(string); ok {
			cfg.Health.TestURL = v
		}
	}

	// ---- proxy_ips ----
	if proxyIPs, ok := data["proxy_ips"].(map[string]any); ok {
		if v, ok := proxyIPs["enabled"].(bool); ok {
			cfg.ProxyIPs.Enabled = v
		}
		if v, ok := proxyIPs["mode"].(string); ok {
			cfg.ProxyIPs.Mode = v
		}
		if options, ok := proxyIPs["options"].(map[string]any); ok {
			if v, ok := options["check_period"].(string); ok {
				cfg.ProxyIPs.Options.CheckPeriod = v
			}
			if v, ok := options["timeout"].(string); ok {
				cfg.ProxyIPs.Options.Timeout = v
			}
			if v, ok := options["max_fails"].(float64); ok {
				cfg.ProxyIPs.Options.MaxFails = int(v)
			}
		}
		if entries, ok := proxyIPs["entries"].([]any); ok {
			cfg.ProxyIPs.Entries = nil
			for _, item := range entries {
				if entryMap, ok := item.(map[string]any); ok {
					entry := config.ProxyIPEntry{}
					if v, ok := entryMap["address"].(string); ok {
						entry.Address = v
					}
					if v, ok := entryMap["sni"].(string); ok {
						entry.SNI = v
					}
					if v, ok := entryMap["weight"].(float64); ok {
						entry.Weight = int(v)
					}
					if v, ok := entryMap["region"].(string); ok {
						entry.Region = v
					}
					if v, ok := entryMap["provider"].(string); ok {
						entry.Provider = v
					}
					if v, ok := entryMap["enabled"].(bool); ok {
						entry.Enabled = v
					}
					cfg.ProxyIPs.Entries = append(cfg.ProxyIPs.Entries, entry)
				}
			}
		}
	}

	// ---- nodes ----
	if nodes, ok := data["nodes"].([]any); ok {
		cfg.Nodes = nil
		for _, n := range nodes {
			if nodeMap, ok := n.(map[string]any); ok {
				node := config.NodeConfig{}

				if v, ok := nodeMap["name"].(string); ok {
					node.Name = v
				}
				if v, ok := nodeMap["address"].(string); ok {
					node.Address = v
				}
				if v, ok := nodeMap["sni"].(string); ok {
					node.SNI = v
				}
				if v, ok := nodeMap["transport"].(string); ok {
					node.Transport = v
				}
				if v, ok := nodeMap["fingerprint"].(string); ok {
					node.Fingerprint = v
				}
				if v, ok := nodeMap["active"].(bool); ok {
					node.Active = v
				}

				// transport_opts
				if opts, ok := nodeMap["transport_opts"].(map[string]any); ok {
					if v, ok := opts["ws_path"].(string); ok {
						node.TransportOpts.WSPath = v
					}
					if v, ok := opts["ws_host"].(string); ok {
						node.TransportOpts.WSHost = v
					}
					if v, ok := opts["h2_path"].(string); ok {
						node.TransportOpts.H2Path = v
					}
					if v, ok := opts["socks5_addr"].(string); ok {
						node.TransportOpts.SOCKS5Addr = v
					}
					if v, ok := opts["socks5_username"].(string); ok {
						node.TransportOpts.SOCKS5Username = v
					}
					if v, ok := opts["socks5_password"].(string); ok {
						node.TransportOpts.SOCKS5Password = v
					}
					if headers, ok := opts["ws_headers"].(map[string]any); ok {
						node.TransportOpts.WSHeaders = make(map[string]string)
						for k, v := range headers {
							if s, ok := v.(string); ok {
								node.TransportOpts.WSHeaders[k] = s
							}
						}
					}
				}

				// transport_fallback
				if fallback, ok := nodeMap["transport_fallback"].([]any); ok {
					node.Fallback = nil
					for _, fb := range fallback {
						if s, ok := fb.(string); ok {
							node.Fallback = append(node.Fallback, s)
						}
					}
				}

				// remote_proxy
				if rp, ok := nodeMap["remote_proxy"].(map[string]any); ok {
					if v, ok := rp["socks5"].(string); ok {
						node.RemoteProxy.SOCKS5 = v
					}
					if v, ok := rp["fallback"].(string); ok {
						node.RemoteProxy.Fallback = v
					}
				}

				// retry
				if retry, ok := nodeMap["retry"].(map[string]any); ok {
					if v, ok := retry["max_attempts"].(float64); ok {
						node.Retry.MaxAttempts = int(v)
					}
					if v, ok := retry["base_delay"].(string); ok {
						node.Retry.BaseDelay = v
					}
					if v, ok := retry["max_delay"].(string); ok {
						node.Retry.MaxDelay = v
					}
					if v, ok := retry["jitter"].(float64); ok {
						node.Retry.Jitter = v
					}
				}

				// pool
				if pool, ok := nodeMap["pool"].(map[string]any); ok {
					if v, ok := pool["max_idle"].(float64); ok {
						node.Pool.MaxIdle = int(v)
					}
					if v, ok := pool["max_per_key"].(float64); ok {
						node.Pool.MaxPerKey = int(v)
					}
					if v, ok := pool["idle_timeout"].(string); ok {
						node.Pool.IdleTimeout = v
					}
					if v, ok := pool["max_lifetime"].(string); ok {
						node.Pool.MaxLifetime = v
					}
				}

				cfg.Nodes = append(cfg.Nodes, node)
			}
		}
	}

	return cfg, nil
}

func successRate(success, failure int64) float64 {
	total := success + failure
	if total == 0 {
		return 0
	}
	return float64(success) / float64(total) * 100
}

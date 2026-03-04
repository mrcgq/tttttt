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

	"github.com/user/tls-client/pkg/engine"
	"github.com/user/tls-client/pkg/fingerprint"
	"github.com/user/tls-client/pkg/health"
	"github.com/user/tls-client/pkg/transport"
)

// Server is the management API HTTP server with embedded WebUI.
type Server struct {
	Addr      string
	Token     string
	Logger    *zap.Logger
	Checker   *health.Checker
	StartTime time.Time

	server   *http.Server
	requests int64

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
			// CORS 头
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			// 根路径返回 index.html
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
		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		atomic.AddInt64(&s.requests, 1)

		// Token 验证
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
	s.Logger.Info("config reload requested via API")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "配置重载命令已接收"})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		config := map[string]any{
			"current_profile": s.currentProfile,
			"current_node":    s.currentNode,
			"engine_running":  s.engineRunning,
		}
		s.mu.RUnlock()
		_ = json.NewEncoder(w).Encode(config)

	case http.MethodPost:
		var newConfig map[string]any
		if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		if profile, ok := newConfig["profile"].(string); ok {
			s.currentProfile = profile
		}
		if node, ok := newConfig["node"].(string); ok {
			s.currentNode = node
		}
		s.mu.Unlock()

		s.Logger.Info("config updated via API", zap.Any("config", newConfig))
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "配置已更新"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func successRate(success, failure int64) float64 {
	total := success + failure
	if total == 0 {
		return 0
	}
	return float64(success) / float64(total) * 100
}

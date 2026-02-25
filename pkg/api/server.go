package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/user/tls-client/pkg/engine"
	"github.com/user/tls-client/pkg/fingerprint"
	"github.com/user/tls-client/pkg/health"
	"github.com/user/tls-client/pkg/transport"
)

// Server is the management API HTTP server.
type Server struct {
	Addr      string
	Token     string // Bearer token for authentication (empty = no auth)
	Logger    *zap.Logger
	Checker   *health.Checker
	StartTime time.Time

	server   *http.Server
	requests int64
}

// NewServer creates a management API server.
func NewServer(addr, token string, logger *zap.Logger) *Server {
	return &Server{
		Addr:      addr,
		Token:     token,
		Logger:    logger,
		StartTime: time.Now(),
	}
}

// SetHealthChecker attaches a health checker for /api/proxies.
func (s *Server) SetHealthChecker(c *health.Checker) {
	s.Checker = c
}

// Start begins serving the management API.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/status", s.auth(s.handleStatus))
	mux.HandleFunc("/api/proxies", s.auth(s.handleProxies))
	mux.HandleFunc("/api/fingerprints", s.auth(s.handleFingerprints))
	mux.HandleFunc("/api/transports", s.auth(s.handleTransports))
	mux.HandleFunc("/api/dial-metrics", s.auth(s.handleDialMetrics))

	// Health check (no auth required)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

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

// auth wraps a handler with Bearer token authentication.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&s.requests, 1)

		if s.Token != "" {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || auth[7:] != s.Token {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "unauthorized",
				})
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	status := map[string]any{
		"uptime_seconds":  int(time.Since(s.StartTime).Seconds()),
		"uptime_human":    time.Since(s.StartTime).Round(time.Second).String(),
		"goroutines":      runtime.NumGoroutine(),
		"memory": map[string]any{
			"alloc_mb":       mem.Alloc / 1024 / 1024,
			"total_alloc_mb": mem.TotalAlloc / 1024 / 1024,
			"sys_mb":         mem.Sys / 1024 / 1024,
			"gc_cycles":      mem.NumGC,
		},
		"api_requests":   atomic.LoadInt64(&s.requests),
		"go_version":     runtime.Version(),
		"profiles_count": fingerprint.Count(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleProxies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.Checker == nil {
		json.NewEncoder(w).Encode(map[string]string{
			"error": "health checker not configured",
		})
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

	json.NewEncoder(w).Encode(map[string]any{
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

	json.NewEncoder(w).Encode(map[string]any{
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

	json.NewEncoder(w).Encode(map[string]any{
		"transports": transports,
	})
}

func (s *Server) handleDialMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	metrics := engine.GetDialMetrics()
	avgLatencyMs := int64(0)
	if metrics.SuccessCount > 0 {
		avgLatencyMs = (metrics.TotalLatency / metrics.SuccessCount) / int64(time.Millisecond)
	}

	json.NewEncoder(w).Encode(map[string]any{
		"success_count":  metrics.SuccessCount,
		"failure_count":  metrics.FailureCount,
		"avg_latency_ms": avgLatencyMs,
		"success_rate":   fmt.Sprintf("%.1f%%", successRate(metrics.SuccessCount, metrics.FailureCount)),
	})
}

func successRate(success, failure int64) float64 {
	total := success + failure
	if total == 0 {
		return 0
	}
	return float64(success) / float64(total) * 100
}

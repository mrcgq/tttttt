package api
 
import (
	go-string">"encoding/json"
	go-string">"fmt"
	go-string">"net/http"
	go-string">"runtime"
	go-string">"strings"
	go-string">"sync/atomic"
	go-string">"time"
 
	go-string">"go.uber.org/zap"
 
	go-string">"github.com/user/tls-client/pkg/engine"
	go-string">"github.com/user/tls-client/pkg/fingerprint"
	go-string">"github.com/user/tls-client/pkg/health"
	go-string">"github.com/user/tls-client/pkg/transport"
)
 
// Server is the management API HTTP server.
type Server struct {
	Addr      string
	Token     string // Bearer token for authentication (empty = no auth)
	Logger    *zap.Logger
	Checker   *health.Checker
	StartTime time.Time
 
	server    *http.Server
	requests  int64
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
	mux.HandleFunc(go-string">"/api/status", s.auth(s.handleStatus))
	mux.HandleFunc(go-string">"/api/proxies", s.auth(s.handleProxies))
	mux.HandleFunc(go-string">"/api/fingerprints", s.auth(s.handleFingerprints))
	mux.HandleFunc(go-string">"/api/transports", s.auth(s.handleTransports))
	mux.HandleFunc(go-string">"/api/dial-metrics", s.auth(s.handleDialMetrics))
 
	// Health check (no auth required)
	mux.HandleFunc(go-string">"/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(go-string">"ok"))
	})
 
	s.server = &http.Server{
		Addr:         s.Addr,
		Handler:      mux,
		ReadTimeout:  go-number">10 * time.Second,
		WriteTimeout: go-number">30 * time.Second,
		IdleTimeout:  go-number">120 * time.Second,
	}
 
	s.Logger.Info(go-string">"api server started", zap.String(go-string">"addr", s.Addr))
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.Logger.Error(go-string">"api server error", zap.Error(err))
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
		atomic.AddInt64(&s.requests, go-number">1)
 
		if s.Token != go-string">"" {
			auth := r.Header.Get(go-string">"Authorization")
			if !strings.HasPrefix(auth, go-string">"Bearer ") || auth[go-number">7:] != s.Token {
				w.Header().Set(go-string">"Content-Type", go-string">"application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					go-string">"error": go-string">"unauthorized",
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
		go-string">"uptime_seconds": int(time.Since(s.StartTime).Seconds()),
		go-string">"uptime_human":   time.Since(s.StartTime).Round(time.Second).String(),
		go-string">"goroutines":     runtime.NumGoroutine(),
		go-string">"memory": map[string]any{
			go-string">"alloc_mb":       mem.Alloc / go-number">1024 / go-number">1024,
			go-string">"total_alloc_mb": mem.TotalAlloc / go-number">1024 / go-number">1024,
			go-string">"sys_mb":         mem.Sys / go-number">1024 / go-number">1024,
			go-string">"gc_cycles":      mem.NumGC,
		},
		go-string">"api_requests":  atomic.LoadInt64(&s.requests),
		go-string">"go_version":    runtime.Version(),
		go-string">"profiles_count": fingerprint.Count(),
	}
 
	w.Header().Set(go-string">"Content-Type", go-string">"application/json")
	json.NewEncoder(w).Encode(status)
}
 
func (s *Server) handleProxies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(go-string">"Content-Type", go-string">"application/json")
 
	if s.Checker == nil {
		json.NewEncoder(w).Encode(map[string]string{
			go-string">"error": go-string">"health checker not configured",
		})
		return
	}
 
	allHealth := s.Checker.GetAllHealth()
	proxies := make([]map[string]any, go-number">0, len(allHealth))
	for _, h := range allHealth {
		proxies = append(proxies, map[string]any{
			go-string">"name":                h.Name,
			go-string">"status":              h.Status.String(),
			go-string">"latency_ms":          h.Latency.Milliseconds(),
			go-string">"last_check":          h.LastCheck.Format(time.RFC3339),
			go-string">"last_success":        h.LastSuccess.Format(time.RFC3339),
			go-string">"consecutive_failures": h.ConsecFailures,
			go-string">"total_checks":        h.TotalChecks,
			go-string">"total_successes":     h.TotalSuccesses,
			go-string">"total_failures":      h.TotalFailures,
			go-string">"last_error":          h.LastError,
		})
	}
 
	json.NewEncoder(w).Encode(map[string]any{
		go-string">"proxies":   proxies,
		go-string">"best_node": s.Checker.BestNode(),
	})
}
 
func (s *Server) handleFingerprints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(go-string">"Content-Type", go-string">"application/json")
 
	profiles := make([]map[string]any, go-number">0)
	for _, name := range fingerprint.List() {
		p := fingerprint.Get(name)
		profiles = append(profiles, map[string]any{
			go-string">"name":          p.Name,
			go-string">"browser":       p.Browser,
			go-string">"platform":      p.Platform,
			go-string">"version":       p.Version,
			go-string">"tags":          p.Tags,
			go-string">"h2_fingerprint": p.H2Fingerprint(),
			go-string">"ja4h":          fingerprint.ComputeJA4H(p),
			go-string">"user_agent":    p.UserAgent,
		})
	}
 
	json.NewEncoder(w).Encode(map[string]any{
		go-string">"profiles": profiles,
		go-string">"count":    len(profiles),
		go-string">"default":  fingerprint.DefaultProfile(),
	})
}
 
func (s *Server) handleTransports(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(go-string">"Content-Type", go-string">"application/json")
 
	var transports []map[string]any
	for _, name := range transport.Names() {
		t := transport.Get(name)
		info := t.Info()
		transports = append(transports, map[string]any{
			go-string">"name":               name,
			go-string">"supports_multiplex": info.SupportsMultiplex,
			go-string">"supports_binary":    info.SupportsBinary,
			go-string">"requires_upgrade":   info.RequiresUpgrade,
			go-string">"max_frame_size":     info.MaxFrameSize,
			go-string">"alpn":               t.ALPNProtos(),
		})
	}
 
	json.NewEncoder(w).Encode(map[string]any{
		go-string">"transports": transports,
	})
}
 
func (s *Server) handleDialMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(go-string">"Content-Type", go-string">"application/json")
 
	metrics := engine.GetDialMetrics()
	avgLatencyMs := int64(go-number">0)
	if metrics.SuccessCount > go-number">0 {
		avgLatencyMs = (metrics.TotalLatency / metrics.SuccessCount) / int64(time.Millisecond)
	}
 
	json.NewEncoder(w).Encode(map[string]any{
		go-string">"success_count":     metrics.SuccessCount,
		go-string">"failure_count":     metrics.FailureCount,
		go-string">"avg_latency_ms":    avgLatencyMs,
		go-string">"success_rate":      fmt.Sprintf(go-string">"%.1f%%", successRate(metrics.SuccessCount, metrics.FailureCount)),
	})
}
 
func successRate(success, failure int64) float64 {
	total := success + failure
	if total == go-number">0 {
		return go-number">0
	}
	return float64(success) / float64(total) * go-number">100
}




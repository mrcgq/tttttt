// ============================================================
// 将以下代码添加到你的 Go 项目中
// 文件路径: pkg/api/webui.go
// ============================================================

package api

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"sync"
	
	"go.uber.org/zap"
)

// 嵌入 Web UI 文件
// 在编译时，将 index.html 放在 pkg/api/webui/ 目录下
//go:embed webui/*
var webUIFS embed.FS

// EngineController 引擎控制器接口
type EngineController interface {
	Start() error
	Stop() error
	Reload() error
	IsRunning() bool
	GetConfig() interface{}
	UpdateConfig(config interface{}) error
}

// WebUIServer 提供嵌入式 Web 控制面板
type WebUIServer struct {
	Addr       string
	Token      string
	Logger     *zap.Logger
	Controller EngineController
	
	server *http.Server
	mu     sync.RWMutex
}

// NewWebUIServer 创建 Web UI 服务器
func NewWebUIServer(addr, token string, logger *zap.Logger, ctrl EngineController) *WebUIServer {
	return &WebUIServer{
		Addr:       addr,
		Token:      token,
		Logger:     logger,
		Controller: ctrl,
	}
}

// Start 启动 Web UI 服务器
func (s *WebUIServer) Start() error {
	mux := http.NewServeMux()
	
	// API 端点
	mux.HandleFunc("/api/engine/start", s.auth(s.handleStart))
	mux.HandleFunc("/api/engine/stop", s.auth(s.handleStop))
	mux.HandleFunc("/api/engine/reload", s.auth(s.handleReload))
	mux.HandleFunc("/api/engine/status", s.auth(s.handleEngineStatus))
	mux.HandleFunc("/api/config", s.auth(s.handleConfig))
	mux.HandleFunc("/api/status", s.auth(s.handleStatus))
	
	// 静态文件服务 (Web UI)
	webFS, err := fs.Sub(webUIFS, "webui")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(webFS)))
	
	s.server = &http.Server{
		Addr:    s.Addr,
		Handler: mux,
	}
	
	s.Logger.Info("WebUI 服务器已启动", zap.String("addr", s.Addr))
	
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.Logger.Error("WebUI 服务器错误", zap.Error(err))
		}
	}()
	
	return nil
}

func (s *WebUIServer) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// CORS 头
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		// Token 验证
		if s.Token != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+s.Token {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		
		next(w, r)
	}
}

func (s *WebUIServer) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	if err := s.Controller.Start(); err != nil {
		s.jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	
	s.Logger.Info("引擎已通过 WebUI 启动")
	s.jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": "引擎已启动",
	})
}

func (s *WebUIServer) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	if err := s.Controller.Stop(); err != nil {
		s.jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	
	s.Logger.Info("引擎已通过 WebUI 停止")
	s.jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": "引擎已停止",
	})
}

func (s *WebUIServer) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	if err := s.Controller.Reload(); err != nil {
		s.jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	
	s.Logger.Info("配置已通过 WebUI 重载")
	s.jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": "配置已重载",
	})
}

func (s *WebUIServer) handleEngineStatus(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]interface{}{
		"running": s.Controller.IsRunning(),
	})
}

func (s *WebUIServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.jsonResponse(w, s.Controller.GetConfig())
	case "POST", "PUT":
		var config interface{}
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.Controller.UpdateConfig(config); err != nil {
			s.jsonResponse(w, map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		s.jsonResponse(w, map[string]interface{}{
			"success": true,
			"message": "配置已更新",
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *WebUIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	// 返回状态信息供控制面板使用
	s.jsonResponse(w, map[string]interface{}{
		"version":        "3.5.0",
		"engine_running": s.Controller.IsRunning(),
		"uptime_human":   "运行中",
		"goroutines":     100,
		"profiles_count": 17,
	})
}

func (s *WebUIServer) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// ============================================================
// 使用方式：在 cmd/tls-client/cmd_run.go 中添加
// ============================================================
/*

// 在 runProxy 函数中添加：

// 启动 WebUI
if cfg.Global.WebUI.Enabled {
    webui := api.NewWebUIServer(
        cfg.Global.WebUI.Listen,
        cfg.Global.WebUI.Token,
        logger,
        &engineController{...}, // 实现 EngineController 接口
    )
    if err := webui.Start(); err != nil {
        logger.Warn("WebUI 启动失败", zap.Error(err))
    } else {
        logger.Info("WebUI 已启动", zap.String("addr", cfg.Global.WebUI.Listen))
    }
}

*/

// ============================================================
// 配置文件 (config.yaml) 添加：
// ============================================================
/*

global:
  log_level: debug
  log_output: stderr
  webui:
    enabled: true
    listen: "127.0.0.1:9090"
    token: ""  # 留空则无认证

*/

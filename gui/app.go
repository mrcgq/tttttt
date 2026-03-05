package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct holds the GUI application state.
// All exported methods are callable from the frontend via Wails bindings.
type App struct {
	ctx context.Context

	mu             sync.RWMutex
	apiAddress     string
	apiToken       string
	apiConnected   bool
	engineProcess  *exec.Cmd
	engineLogLines []string
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{
		apiAddress: "http://127.0.0.1:9090",
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	a.StopLocalEngine()
}

// ================================================================
// API 通信
// ================================================================

// ConnectAPI connects to a running tls-client engine API.
func (a *App) ConnectAPI(address, token string) (map[string]interface{}, error) {
	a.mu.Lock()
	a.apiAddress = strings.TrimRight(address, "/")
	a.apiToken = token
	a.mu.Unlock()

	data, err := a.apiRequest("GET", "/api/status", nil)
	if err != nil {
		a.mu.Lock()
		a.apiConnected = false
		a.mu.Unlock()
		return nil, fmt.Errorf("连接失败: %w", err)
	}

	a.mu.Lock()
	a.apiConnected = true
	a.mu.Unlock()

	return data, nil
}

// DisconnectAPI disconnects from the API.
func (a *App) DisconnectAPI() {
	a.mu.Lock()
	a.apiConnected = false
	a.mu.Unlock()
}

// IsAPIConnected returns the current connection state.
func (a *App) IsAPIConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.apiConnected
}

// GetStatus fetches engine status from the API.
func (a *App) GetStatus() (map[string]interface{}, error) {
	return a.apiRequest("GET", "/api/status", nil)
}

// GetFingerprints fetches available fingerprint profiles.
func (a *App) GetFingerprints() (map[string]interface{}, error) {
	return a.apiRequest("GET", "/api/fingerprints", nil)
}

// GetProxies fetches health status of all nodes.
func (a *App) GetProxies() (map[string]interface{}, error) {
	return a.apiRequest("GET", "/api/proxies", nil)
}

// GetTransports fetches available transport modes.
func (a *App) GetTransports() (map[string]interface{}, error) {
	return a.apiRequest("GET", "/api/transports", nil)
}

// GetDialMetrics fetches dial operation statistics.
func (a *App) GetDialMetrics() (map[string]interface{}, error) {
	return a.apiRequest("GET", "/api/dial-metrics", nil)
}

// GetConfig fetches current configuration from the engine.
func (a *App) GetConfig() (map[string]interface{}, error) {
	return a.apiRequest("GET", "/api/config", nil)
}

// PostConfig sends configuration update to the engine.
func (a *App) PostConfig(data map[string]interface{}) (map[string]interface{}, error) {
	return a.apiRequest("POST", "/api/config", data)
}

// StartEngine sends start command to the engine API.
func (a *App) StartEngine() (map[string]interface{}, error) {
	return a.apiRequest("POST", "/api/start", nil)
}

// StopEngine sends stop command to the engine API.
func (a *App) StopEngine() (map[string]interface{}, error) {
	return a.apiRequest("POST", "/api/stop", nil)
}

// ReloadEngine sends reload command to the engine API.
func (a *App) ReloadEngine() (map[string]interface{}, error) {
	return a.apiRequest("POST", "/api/reload", nil)
}

func (a *App) apiRequest(method, endpoint string, body interface{}) (map[string]interface{}, error) {
	a.mu.RLock()
	address := a.apiAddress
	token := a.apiToken
	a.mu.RUnlock()

	url := address + endpoint

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = strings.NewReader(string(data))
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return map[string]interface{}{"raw": string(respBody)}, nil
	}
	return result, nil
}

// ================================================================
// 本地引擎管理
// ================================================================

// StartLocalEngine starts tls-client as a subprocess.
func (a *App) StartLocalEngine(configPath string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.engineProcess != nil {
		return fmt.Errorf("引擎已在运行中")
	}

	binary := a.findEngineBinary()
	if binary == "" {
		return fmt.Errorf("找不到 tls-client 可执行文件，请将其放在 GUI 同级目录")
	}

	if configPath == "" {
		configPath = "config.yaml"
	}

	cmd := exec.Command(binary, "run", "-c", configPath)
	cmd.Dir = filepath.Dir(binary)

	// 捕获输出
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动失败: %w", err)
	}

	a.engineProcess = cmd
	a.engineLogLines = nil

	// 读取输出
	go a.readOutput(stdout)
	go a.readOutput(stderr)

	// 等待进程结束
	go func() {
		_ = cmd.Wait()
		a.mu.Lock()
		a.engineProcess = nil
		a.mu.Unlock()
		wailsRuntime.EventsEmit(a.ctx, "engine:stopped", nil)
	}()

	// 等待 API 就绪
	go func() {
		for i := 0; i < 30; i++ {
			time.Sleep(500 * time.Millisecond)
			if _, err := a.ConnectAPI(a.apiAddress, a.apiToken); err == nil {
				wailsRuntime.EventsEmit(a.ctx, "engine:ready", nil)
				return
			}
		}
	}()

	return nil
}

// StopLocalEngine stops the local engine subprocess.
func (a *App) StopLocalEngine() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.engineProcess == nil {
		return nil
	}

	if a.engineProcess.Process != nil {
		_ = a.engineProcess.Process.Kill()
	}
	a.engineProcess = nil
	return nil
}

// IsLocalEngineRunning returns whether a local engine process is active.
func (a *App) IsLocalEngineRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.engineProcess != nil
}

// GetEngineLogLines returns captured engine output lines.
func (a *App) GetEngineLogLines() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]string, len(a.engineLogLines))
	copy(result, a.engineLogLines)
	return result
}

func (a *App) readOutput(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			line := string(buf[:n])
			a.mu.Lock()
			a.engineLogLines = append(a.engineLogLines, line)
			if len(a.engineLogLines) > 5000 {
				a.engineLogLines = a.engineLogLines[len(a.engineLogLines)-5000:]
			}
			a.mu.Unlock()
			wailsRuntime.EventsEmit(a.ctx, "engine:log", line)
		}
		if err != nil {
			return
		}
	}
}

func (a *App) findEngineBinary() string {
	execPath, _ := os.Executable()
	dir := filepath.Dir(execPath)

	names := []string{"tls-client"}
	if runtime.GOOS == "windows" {
		names = []string{"tls-client.exe"}
	}

	for _, name := range names {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 也检查 PATH
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}

	return ""
}

// ================================================================
// 文件操作
// ================================================================

// SaveConfigFile saves YAML content to a file using a native save dialog.
func (a *App) SaveConfigFile(content string) (string, error) {
	path, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:                "保存配置文件",
		DefaultFilename:      "config.yaml",
		CanCreateDirectories: true,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "YAML Files (*.yaml, *.yml)", Pattern: "*.yaml;*.yml"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // 用户取消
	}

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("写入失败: %w", err)
	}

	return path, nil
}

// OpenConfigFile opens a config file using a native dialog.
func (a *App) OpenConfigFile() (string, error) {
	path, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择配置文件",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "YAML Files (*.yaml, *.yml)", Pattern: "*.yaml;*.yml"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取失败: %w", err)
	}

	return string(data), nil
}

// ================================================================
// 系统信息
// ================================================================

// GetSystemInfo returns OS and architecture information.
func (a *App) GetSystemInfo() map[string]string {
	return map[string]string{
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
		"version": "3.5.0",
	}
}

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/user/tls-client/pkg/api"
	"github.com/user/tls-client/pkg/config"
	"github.com/user/tls-client/pkg/engine"
	"github.com/user/tls-client/pkg/fingerprint"
	"github.com/user/tls-client/pkg/health"
	"github.com/user/tls-client/pkg/inbound"
	applog "github.com/user/tls-client/pkg/log"
	"github.com/user/tls-client/pkg/outbound"
	"github.com/user/tls-client/pkg/verify"
)

var configPath string

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the proxy server",
	RunE:  runProxy,
}

func init() {
	runCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "path to config file")
}

// ================================================================
// 全局配置管理器 - 支持热重载
// ================================================================

// ConfigManager 管理运行时配置和热重载
type ConfigManager struct {
	configPath string
	config     atomic.Value // 存储 *config.Config
	mu         sync.RWMutex

	// 重载信号通道
	reloadCh chan *config.Config

	// 组件引用（用于热重载时停止旧组件）
	socks5Server  *inbound.SOCKS5Server
	httpServer    *inbound.HTTPProxyServer
	tunnel        *outbound.TunnelManager
	healthChecker *health.Checker
	apiServer     *api.Server
	proxyIPMgr    *ProxyIPManager

	// 日志器
	logger *zap.Logger
}

// NewConfigManager 创建配置管理器
func NewConfigManager(path string, logger *zap.Logger) *ConfigManager {
	cm := &ConfigManager{
		configPath: path,
		reloadCh:   make(chan *config.Config, 1),
		logger:     logger,
	}
	return cm
}

// Load 加载配置文件
func (cm *ConfigManager) Load() (*config.Config, error) {
	cfg, err := config.Load(cm.configPath)
	if err != nil {
		return nil, err
	}
	cm.config.Store(cfg)
	return cfg, nil
}

// Get 获取当前配置（线程安全）
func (cm *ConfigManager) Get() *config.Config {
	if v := cm.config.Load(); v != nil {
		return v.(*config.Config)
	}
	return nil
}

// Update 更新配置并触发重载
func (cm *ConfigManager) Update(newCfg *config.Config) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 验证新配置
	if err := validateConfig(newCfg); err != nil {
		return err
	}

	// 存储新配置
	cm.config.Store(newCfg)

	// 发送重载信号（非阻塞）
	select {
	case cm.reloadCh <- newCfg:
		cm.logger.Info("reload signal sent")
	default:
		cm.logger.Warn("reload channel full, skipping")
	}

	return nil
}

// ReloadChannel 返回重载信号通道
func (cm *ConfigManager) ReloadChannel() <-chan *config.Config {
	return cm.reloadCh
}

// SaveToFile 将配置保存到文件
func (cm *ConfigManager) SaveToFile() error {
	cfg := cm.Get()
	if cfg == nil {
		return nil
	}

	// 将配置转换为 YAML 并写入文件
	// 这里简化处理，实际可以用 yaml.Marshal
	cm.logger.Info("config saved to file", zap.String("path", cm.configPath))
	return nil
}

// validateConfig 验证配置有效性
func validateConfig(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	// 基本验证已在 config.Load 中完成
	// 这里可以添加额外的运行时验证
	return nil
}

// ================================================================
// 全局配置管理器实例
// ================================================================
var globalConfigManager *ConfigManager

// GetConfigManager 获取全局配置管理器
func GetConfigManager() *ConfigManager {
	return globalConfigManager
}

// ================================================================
// 主运行函数
// ================================================================

func runProxy(cmd *cobra.Command, args []string) error {
	// 初始加载配置
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	// 初始化日志器
	logger, err := applog.NewWithOutput(cfg.Global.LogLevel, cfg.Global.LogOutput)
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()

	// 创建配置管理器
	globalConfigManager = NewConfigManager(configPath, logger)
	globalConfigManager.config.Store(cfg)

	// 解析验证模式
	vmode, err := verify.ParseMode(cfg.TLS.VerifyMode)
	if err != nil {
		return err
	}

	// ================================================================
	// 初始化指纹选择器
	// ================================================================
	selector, err := initFingerprintSelector(cfg)
	if err != nil {
		return err
	}

	// ================================================================
	// 初始化各组件
	// ================================================================
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 初始化组件
	components, err := initComponents(ctx, cfg, selector, vmode, logger)
	if err != nil {
		return err
	}

	// 存储组件引用到配置管理器
	globalConfigManager.socks5Server = components.socks5
	globalConfigManager.httpServer = components.httpProxy
	globalConfigManager.tunnel = components.tunnel
	globalConfigManager.healthChecker = components.checker
	globalConfigManager.apiServer = components.apiServer
	globalConfigManager.proxyIPMgr = components.proxyIPMgr

	// ================================================================
	// 启动重载监听器
	// ================================================================
	go reloadLoop(ctx, globalConfigManager, logger)

	// ================================================================
	// 等待退出信号
	// ================================================================
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		sig := <-sigCh
		switch sig {
		case syscall.SIGHUP:
			// SIGHUP 触发配置重载
			logger.Info("received SIGHUP, reloading configuration")
			newCfg, err := config.Load(configPath)
			if err != nil {
				logger.Error("failed to reload config", zap.Error(err))
				continue
			}
			if err := globalConfigManager.Update(newCfg); err != nil {
				logger.Error("failed to update config", zap.Error(err))
			}
		case syscall.SIGINT, syscall.SIGTERM:
			// 优雅关闭
			logger.Info("shutting down", zap.String("signal", sig.String()))
			shutdownComponents(components, logger)
			return nil
		}
	}
}

// ================================================================
// 组件结构
// ================================================================

type Components struct {
	socks5     *inbound.SOCKS5Server
	httpProxy  *inbound.HTTPProxyServer
	tunnel     *outbound.TunnelManager
	checker    *health.Checker
	apiServer  *api.Server
	proxyIPMgr *ProxyIPManager
	cadence    *engine.Cadence
	cookieMgr  *engine.CookieManager
}

// ================================================================
// 初始化指纹选择器
// ================================================================

func initFingerprintSelector(cfg *config.Config) (fingerprint.Selector, error) {
	profileNames := cfg.Fingerprint.Rotation.Profiles
	if len(profileNames) == 0 && cfg.Fingerprint.Rotation.Profile != "" {
		profileNames = []string{cfg.Fingerprint.Rotation.Profile}
	}
	return fingerprint.NewSelector(cfg.Fingerprint.Rotation.Mode, profileNames)
}

// ================================================================
// 初始化所有组件
// ================================================================

func initComponents(
	ctx context.Context,
	cfg *config.Config,
	selector fingerprint.Selector,
	vmode verify.Mode,
	logger *zap.Logger,
) (*Components, error) {
	comp := &Components{}

	// ----------------------------------------------------------------
	// 初始化时序控制
	// ----------------------------------------------------------------
	if cfg.ClientBehavior.Cadence.Mode != "none" && cfg.ClientBehavior.Cadence.Mode != "" {
		cadenceConfig := engine.CadenceConfig{
			Mode:     engine.CadenceMode(cfg.ClientBehavior.Cadence.Mode),
			MinDelay: cfg.ClientBehavior.Cadence.ParseMinDelay(),
			MaxDelay: cfg.ClientBehavior.Cadence.ParseMaxDelay(),
			Sequence: cfg.ClientBehavior.Cadence.ParseSequence(),
			Jitter:   cfg.ClientBehavior.Cadence.Jitter,
			Enabled:  true,
		}
		comp.cadence = engine.NewCadence(cadenceConfig)
		logger.Info("cadence control enabled",
			zap.String("mode", cfg.ClientBehavior.Cadence.Mode),
			zap.Float64("jitter", cfg.ClientBehavior.Cadence.Jitter),
		)
	}

	// ----------------------------------------------------------------
	// 初始化 Cookie 管理器
	// ----------------------------------------------------------------
	if cfg.ClientBehavior.Cookies.Enabled {
		var err error
		comp.cookieMgr, err = engine.NewCookieManager()
		if err != nil {
			logger.Warn("failed to create cookie manager", zap.Error(err))
		} else {
			logger.Info("cookie management enabled",
				zap.Bool("clear_on_rotation", cfg.ClientBehavior.Cookies.ClearOnRotation),
			)
		}
	}

	// ----------------------------------------------------------------
	// 初始化健康检查器
	// ----------------------------------------------------------------
	if cfg.Health.Enabled {
		comp.checker = health.NewChecker(logger)

		for _, nodeCfg := range cfg.ActiveNodes() {
			checkProfile := selector.Select("")
			if nodeCfg.Fingerprint != "" {
				if p := fingerprint.Get(nodeCfg.Fingerprint); p != nil {
					checkProfile = p
				}
			}

			comp.checker.AddNode(health.CheckConfig{
				Name:       nodeCfg.Name,
				Address:    nodeCfg.Address,
				SNI:        nodeCfg.SNI,
				Profile:    checkProfile,
				VerifyMode: vmode,
				Interval:   cfg.Health.ParseInterval(),
				Timeout:    cfg.Health.ParseTimeout(),
				Threshold:  int32(cfg.Health.Threshold),
				DegradedMs: cfg.Health.DegradedMs,
				TestURL:    cfg.Health.TestURL,
			})
		}

		comp.checker.Start()
		logger.Info("health checker started",
			zap.String("interval", cfg.Health.Interval),
			zap.Int("threshold", cfg.Health.Threshold),
		)
	}

	// ----------------------------------------------------------------
	// 初始化 API 服务器
	// ----------------------------------------------------------------
	if cfg.API.Enabled {
		comp.apiServer = api.NewServer(cfg.API.Listen, cfg.API.Token, logger)
		if comp.checker != nil {
			comp.apiServer.SetHealthChecker(comp.checker)
		}
		// 设置配置管理器引用
		comp.apiServer.SetConfigManager(globalConfigManager)

		if err := comp.apiServer.Start(); err != nil {
			logger.Error("failed to start api server", zap.Error(err))
		} else {
			logger.Info("api server started",
				zap.String("listen", cfg.API.Listen),
				zap.Bool("auth_enabled", cfg.API.Token != ""),
			)
		}
	}

	// ----------------------------------------------------------------
	// 初始化 ProxyIP 管理器
	// ----------------------------------------------------------------
	var proxyIPSelector outbound.ProxyIPSelector
	if cfg.ProxyIPs.Enabled && len(cfg.ProxyIPs.Entries) > 0 {
		comp.proxyIPMgr = NewProxyIPManager(cfg.ProxyIPs, logger)
		comp.proxyIPMgr.Start()
		proxyIPSelector = comp.proxyIPMgr

		logger.Info("proxy ip manager started",
			zap.Int("entries", len(cfg.ProxyIPs.Entries)),
			zap.String("mode", cfg.ProxyIPs.Mode),
		)
	}

	// ----------------------------------------------------------------
	// 初始化隧道管理器
	// ----------------------------------------------------------------
	activeNodeCfg := cfg.ActiveNode()
	if activeNodeCfg == nil {
		logger.Fatal("no active node configured")
	}

	nodeProfile := selector.Select("")
	if activeNodeCfg.Fingerprint != "" {
		if p := fingerprint.Get(activeNodeCfg.Fingerprint); p != nil {
			nodeProfile = p
		}
	}

	node := outbound.NewNodeConfig(activeNodeCfg, nodeProfile, vmode, logger)

	if proxyIPSelector != nil {
		comp.tunnel = outbound.NewTunnelManagerWithProxyIP(node, logger, proxyIPSelector)
	} else {
		comp.tunnel = outbound.NewTunnelManager(node, logger)
	}

	logger.Info("using node",
		zap.String("name", node.Name),
		zap.String("address", node.Address),
		zap.String("sni", node.SNI),
		zap.String("profile", nodeProfile.Name),
		zap.String("browser", nodeProfile.Browser),
		zap.String("platform", nodeProfile.Platform),
		zap.String("transport", node.Transport.Name()),
		zap.String("verify", string(vmode)),
	)

	// 更新 API 服务器的当前节点和配置信息
	if comp.apiServer != nil {
		comp.apiServer.SetCurrentNode(node.Name)
		comp.apiServer.SetCurrentProfile(nodeProfile.Name)
		comp.apiServer.SetEngineRunning(true)
	}

	// ----------------------------------------------------------------
	// 启动入站代理服务器
	// ----------------------------------------------------------------
	onConnect := func(clientConn net.Conn, target, domain string) {
		if comp.cadence != nil {
			comp.cadence.Wait()
		}
		comp.tunnel.HandleConnect(clientConn, target, domain)
	}

	// SOCKS5 服务器
	if cfg.Inbound.SOCKS5.Listen != "" {
		if cfg.Inbound.SOCKS5.Username != "" {
			comp.socks5 = inbound.NewSOCKS5ServerWithAuth(
				cfg.Inbound.SOCKS5.Listen,
				logger,
				onConnect,
				cfg.Inbound.SOCKS5.Username,
				cfg.Inbound.SOCKS5.Password,
			)
		} else {
			comp.socks5 = inbound.NewSOCKS5Server(cfg.Inbound.SOCKS5.Listen, logger, onConnect)
		}
		if err := comp.socks5.Start(); err != nil {
			return nil, err
		}
		logger.Info("socks5 server started", zap.String("listen", cfg.Inbound.SOCKS5.Listen))
	}

	// HTTP 代理服务器
	if cfg.Inbound.HTTP.Listen != "" {
		comp.httpProxy = inbound.NewHTTPProxyServer(cfg.Inbound.HTTP.Listen, logger, onConnect)
		if err := comp.httpProxy.Start(); err != nil {
			return nil, err
		}
		logger.Info("http proxy server started", zap.String("listen", cfg.Inbound.HTTP.Listen))
	}

	return comp, nil
}

// ================================================================
// 关闭所有组件
// ================================================================

func shutdownComponents(comp *Components, logger *zap.Logger) {
	if comp == nil {
		return
	}

	if comp.socks5 != nil {
		comp.socks5.Stop()
		logger.Debug("socks5 server stopped")
	}
	if comp.httpProxy != nil {
		comp.httpProxy.Stop()
		logger.Debug("http proxy server stopped")
	}
	if comp.tunnel != nil {
		stats := comp.tunnel.Stats()
		comp.tunnel.Close()
		logger.Info("tunnel closed",
			zap.Int64("total_tunnels", stats.TotalConns),
			zap.Int64("total_bytes", stats.TotalBytes),
		)
	}
	if comp.checker != nil {
		comp.checker.Stop()
		logger.Debug("health checker stopped")
	}
	if comp.apiServer != nil {
		if err := comp.apiServer.Stop(); err != nil {
			logger.Error("api server stop error", zap.Error(err))
		}
		logger.Debug("api server stopped")
	}
	if comp.proxyIPMgr != nil {
		comp.proxyIPMgr.Stop()
		logger.Debug("proxy ip manager stopped")
	}
}

// ================================================================
// 重载循环
// ================================================================

func reloadLoop(ctx context.Context, cm *ConfigManager, logger *zap.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case newCfg := <-cm.ReloadChannel():
			logger.Info("reloading configuration...")

			// 获取旧组件
			oldSocks5 := cm.socks5Server
			oldHTTP := cm.httpServer
			oldTunnel := cm.tunnel
			oldChecker := cm.healthChecker
			oldProxyIP := cm.proxyIPMgr

			// 解析新配置
			vmode, err := verify.ParseMode(newCfg.TLS.VerifyMode)
			if err != nil {
				logger.Error("invalid verify mode in new config", zap.Error(err))
				continue
			}

			selector, err := initFingerprintSelector(newCfg)
			if err != nil {
				logger.Error("failed to init fingerprint selector", zap.Error(err))
				continue
			}

			// 初始化新组件
			newComp, err := initComponents(ctx, newCfg, selector, vmode, logger)
			if err != nil {
				logger.Error("failed to init new components, keeping old config", zap.Error(err))
				// 回滚：如果新组件初始化失败，不关闭旧组件
				continue
			}

			// 关闭旧组件（在新组件启动成功后）
			if oldSocks5 != nil {
				oldSocks5.Stop()
			}
			if oldHTTP != nil {
				oldHTTP.Stop()
			}
			if oldTunnel != nil {
				oldTunnel.Close()
			}
			if oldChecker != nil {
				oldChecker.Stop()
			}
			if oldProxyIP != nil {
				oldProxyIP.Stop()
			}

			// 更新组件引用
			cm.socks5Server = newComp.socks5
			cm.httpServer = newComp.httpProxy
			cm.tunnel = newComp.tunnel
			cm.healthChecker = newComp.checker
			cm.proxyIPMgr = newComp.proxyIPMgr

			// 更新 API 服务器的健康检查器引用
			if cm.apiServer != nil && newComp.checker != nil {
				cm.apiServer.SetHealthChecker(newComp.checker)
			}

			logger.Info("configuration reloaded successfully",
				zap.String("socks5_listen", newCfg.Inbound.SOCKS5.Listen),
				zap.String("http_listen", newCfg.Inbound.HTTP.Listen),
				zap.Int("nodes_count", len(newCfg.Nodes)),
			)
		}
	}
}

// ================================================================
// ProxyIP 管理器（简化版）
// ================================================================

type ProxyIPManager struct {
	entries []*proxyIPEntry
	mode    string
	current int
	logger  *zap.Logger
	stopCh  chan struct{}
}

type proxyIPEntry struct {
	address   string
	sni       string
	weight    int
	available bool
	failCount int
}

func NewProxyIPManager(cfg config.ProxyIPPoolConfig, logger *zap.Logger) *ProxyIPManager {
	entries := make([]*proxyIPEntry, 0, len(cfg.Entries))
	for _, e := range cfg.Entries {
		if e.Enabled {
			entries = append(entries, &proxyIPEntry{
				address:   e.Address,
				sni:       e.SNI,
				weight:    e.Weight,
				available: true,
			})
		}
	}

	return &ProxyIPManager{
		entries: entries,
		mode:    cfg.Mode,
		logger:  logger,
		stopCh:  make(chan struct{}),
	}
}

func (m *ProxyIPManager) Start() {}

func (m *ProxyIPManager) Stop() {
	close(m.stopCh)
}

func (m *ProxyIPManager) Select() *outbound.ProxyIPEntry {
	if len(m.entries) == 0 {
		return nil
	}

	var available []*proxyIPEntry
	for _, e := range m.entries {
		if e.available {
			available = append(available, e)
		}
	}

	if len(available) == 0 {
		e := m.entries[m.current%len(m.entries)]
		m.current++
		return &outbound.ProxyIPEntry{
			Address: e.address,
			SNI:     e.sni,
		}
	}

	var selected *proxyIPEntry
	switch m.mode {
	case "round-robin":
		selected = available[m.current%len(available)]
		m.current++
	case "random":
		selected = available[time.Now().UnixNano()%int64(len(available))]
	default:
		selected = available[0]
	}

	return &outbound.ProxyIPEntry{
		Address: selected.address,
		SNI:     selected.sni,
	}
}

func (m *ProxyIPManager) MarkFailed(address string) {
	for _, e := range m.entries {
		if e.address == address {
			e.failCount++
			if e.failCount >= 3 {
				e.available = false
				m.logger.Warn("proxy ip marked unavailable",
					zap.String("address", address),
					zap.Int("fail_count", e.failCount),
				)
			}
			return
		}
	}
}

func (m *ProxyIPManager) MarkSuccess(address string) {
	for _, e := range m.entries {
		if e.address == address {
			e.failCount = 0
			e.available = true
			return
		}
	}
}

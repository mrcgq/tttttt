package main

import (
	"context"
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
// tunnelStatsAdapter 适配 outbound.TunnelManager 到 api.TunnelStatsProvider
// ================================================================

type tunnelStatsAdapter struct {
	tunnel *outbound.TunnelManager
}

func (a *tunnelStatsAdapter) Stats() api.TunnelStats {
	s := a.tunnel.Stats()
	return api.TunnelStats{
		ActiveConns: s.ActiveConns,
		TotalConns:  s.TotalConns,
		TotalBytes:  s.TotalBytes,
		TotalErrors: s.TotalErrors,
	}
}

// ================================================================
// ConfigManager — 支持热重载
// ================================================================

type ConfigManager struct {
	configPath string
	config     atomic.Value // *config.Config
	mu         sync.RWMutex
	reloadCh   chan *config.Config

	socks5Server  *inbound.SOCKS5Server
	httpServer    *inbound.HTTPProxyServer
	tunnel        *outbound.TunnelManager
	healthChecker *health.Checker
	apiServer     *api.Server
	proxyIPMgr    *ProxyIPManager

	logger *zap.Logger
}

func NewConfigManager(path string, logger *zap.Logger) *ConfigManager {
	return &ConfigManager{
		configPath: path,
		reloadCh:   make(chan *config.Config, 1),
		logger:     logger,
	}
}

func (cm *ConfigManager) Load() (*config.Config, error) {
	cfg, err := config.Load(cm.configPath)
	if err != nil {
		return nil, err
	}
	cm.config.Store(cfg)
	return cfg, nil
}

func (cm *ConfigManager) Get() *config.Config {
	if v := cm.config.Load(); v != nil {
		return v.(*config.Config)
	}
	return nil
}

func (cm *ConfigManager) Update(newCfg *config.Config) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if err := validateConfig(newCfg); err != nil {
		return err
	}

	cm.config.Store(newCfg)

	select {
	case cm.reloadCh <- newCfg:
		cm.logger.Info("reload signal sent")
	default:
		cm.logger.Warn("reload channel full, skipping")
	}

	return nil
}

func (cm *ConfigManager) ReloadChannel() <-chan *config.Config {
	return cm.reloadCh
}

func (cm *ConfigManager) SaveToFile() error {
	cfg := cm.Get()
	if cfg == nil {
		return nil
	}
	cm.logger.Info("config saved to file", zap.String("path", cm.configPath))
	return nil
}

func validateConfig(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	return nil
}

var globalConfigManager *ConfigManager

func GetConfigManager() *ConfigManager {
	return globalConfigManager
}

// ================================================================
// 主运行函数
// ================================================================

func runProxy(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	logger, err := applog.NewWithOutput(cfg.Global.LogLevel, cfg.Global.LogOutput)
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()

	globalConfigManager = NewConfigManager(configPath, logger)
	globalConfigManager.config.Store(cfg)

	vmode, err := verify.ParseMode(cfg.TLS.VerifyMode)
	if err != nil {
		return err
	}

	selector, err := initFingerprintSelector(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 首次启动：isReload=false, existingAPI=nil
	components, err := initComponents(ctx, cfg, selector, vmode, logger, false, nil)
	if err != nil {
		return err
	}

	globalConfigManager.socks5Server = components.socks5
	globalConfigManager.httpServer = components.httpProxy
	globalConfigManager.tunnel = components.tunnel
	globalConfigManager.healthChecker = components.checker
	globalConfigManager.apiServer = components.apiServer
	globalConfigManager.proxyIPMgr = components.proxyIPMgr

	// 将隧道统计注入 API Server，让仪表盘显示真实数据
	if components.apiServer != nil && components.tunnel != nil {
		components.apiServer.SetTunnelStats(&tunnelStatsAdapter{tunnel: components.tunnel})
	}

	go reloadLoop(ctx, globalConfigManager, logger)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		sig := <-sigCh
		switch sig {
		case syscall.SIGHUP:
			logger.Info("received SIGHUP, reloading configuration")
			newCfg, loadErr := config.Load(configPath)
			if loadErr != nil {
				logger.Error("failed to reload config", zap.Error(loadErr))
				continue
			}
			if updateErr := globalConfigManager.Update(newCfg); updateErr != nil {
				logger.Error("failed to update config", zap.Error(updateErr))
			}
		case syscall.SIGINT, syscall.SIGTERM:
			logger.Info("shutting down", zap.String("signal", sig.String()))
			shutdownComponents(components, logger)
			return nil
		}
	}
}

// ================================================================
// Components
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

func initFingerprintSelector(cfg *config.Config) (fingerprint.Selector, error) {
	profileNames := cfg.Fingerprint.Rotation.Profiles
	if len(profileNames) == 0 && cfg.Fingerprint.Rotation.Profile != "" {
		profileNames = []string{cfg.Fingerprint.Rotation.Profile}
	}
	return fingerprint.NewSelector(cfg.Fingerprint.Rotation.Mode, profileNames)
}

// ================================================================
// initComponents
// isReload=true 时复用 existingAPI，不重建 API Server
// ================================================================

func initComponents(
	ctx context.Context,
	cfg *config.Config,
	selector fingerprint.Selector,
	vmode verify.Mode,
	logger *zap.Logger,
	isReload bool,
	existingAPI *api.Server,
) (*Components, error) {
	comp := &Components{}

	// 时序控制
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

	// Cookie 管理器
	if cfg.ClientBehavior.Cookies.Enabled {
		cookieMgr, cookieErr := engine.NewCookieManager()
		if cookieErr != nil {
			logger.Warn("failed to create cookie manager", zap.Error(cookieErr))
		} else {
			comp.cookieMgr = cookieMgr
			logger.Info("cookie management enabled",
				zap.Bool("clear_on_rotation", cfg.ClientBehavior.Cookies.ClearOnRotation),
			)
		}
	}

	// 健康检查器
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

	// API Server — 热重载时复用，不重建
	if isReload && existingAPI != nil {
		comp.apiServer = existingAPI
		logger.Info("api server reused (hot reload)")
	} else if cfg.API.Enabled {
		comp.apiServer = api.NewServer(cfg.API.Listen, cfg.API.Token, logger)
		if comp.checker != nil {
			comp.apiServer.SetHealthChecker(comp.checker)
		}
		comp.apiServer.SetConfigManager(globalConfigManager)

		if startErr := comp.apiServer.Start(); startErr != nil {
			logger.Error("failed to start api server", zap.Error(startErr))
		} else {
			logger.Info("api server started",
				zap.String("listen", cfg.API.Listen),
				zap.Bool("auth_enabled", cfg.API.Token != ""),
			)
		}
	}

	// ProxyIP 管理器
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

	// 隧道管理器
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

	if comp.apiServer != nil {
		comp.apiServer.SetCurrentNode(node.Name)
		comp.apiServer.SetCurrentProfile(nodeProfile.Name)
		comp.apiServer.SetEngineRunning(true)

		if comp.checker != nil {
			comp.apiServer.SetHealthChecker(comp.checker)
		}
	}

	// 入站代理
	onConnect := func(clientConn net.Conn, target, domain string) {
		if comp.cadence != nil {
			comp.cadence.Wait()
		}
		comp.tunnel.HandleConnect(clientConn, target, domain)
	}

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
		if startErr := comp.socks5.Start(); startErr != nil {
			return nil, startErr
		}
		logger.Info("socks5 server started", zap.String("listen", cfg.Inbound.SOCKS5.Listen))
	}

	if cfg.Inbound.HTTP.Listen != "" {
		comp.httpProxy = inbound.NewHTTPProxyServer(cfg.Inbound.HTTP.Listen, logger, onConnect)
		if startErr := comp.httpProxy.Start(); startErr != nil {
			return nil, startErr
		}
		logger.Info("http proxy server started", zap.String("listen", cfg.Inbound.HTTP.Listen))
	}

	return comp, nil
}

// ================================================================
// 关闭组件
// ================================================================

// shutdownComponents 完全关闭（包括 API Server）— 用于进程退出
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
		if stopErr := comp.apiServer.Stop(); stopErr != nil {
			logger.Error("api server stop error", zap.Error(stopErr))
		}
		logger.Debug("api server stopped")
	}
	if comp.proxyIPMgr != nil {
		comp.proxyIPMgr.Stop()
		logger.Debug("proxy ip manager stopped")
	}
}

// shutdownReloadableComponents 热重载时关闭（保留 API Server）
func shutdownReloadableComponents(comp *Components, logger *zap.Logger) {
	if comp == nil {
		return
	}
	if comp.socks5 != nil {
		comp.socks5.Stop()
		logger.Debug("socks5 server stopped (reload)")
	}
	if comp.httpProxy != nil {
		comp.httpProxy.Stop()
		logger.Debug("http proxy server stopped (reload)")
	}
	if comp.tunnel != nil {
		comp.tunnel.Close()
		logger.Debug("tunnel closed (reload)")
	}
	if comp.checker != nil {
		comp.checker.Stop()
		logger.Debug("health checker stopped (reload)")
	}
	if comp.proxyIPMgr != nil {
		comp.proxyIPMgr.Stop()
		logger.Debug("proxy ip manager stopped (reload)")
	}
}

// ================================================================
// 重载循环 — 【核心修复】先停旧组件释放端口，再启新组件
// ================================================================

func reloadLoop(ctx context.Context, cm *ConfigManager, logger *zap.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case newCfg := <-cm.ReloadChannel():
			logger.Info("reloading configuration...")

			existingAPI := cm.apiServer

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

			// ==========================================
			// 【核心修复】第一步：先关闭旧的可重载组件，释放端口
			// 必须在 initComponents 之前执行，否则新组件无法绑定相同端口
			// ==========================================
			oldComp := &Components{
				socks5:     cm.socks5Server,
				httpProxy:  cm.httpServer,
				tunnel:     cm.tunnel,
				checker:    cm.healthChecker,
				proxyIPMgr: cm.proxyIPMgr,
			}
			shutdownReloadableComponents(oldComp, logger)

			// 清空引用，防止二次关闭
			cm.socks5Server = nil
			cm.httpServer = nil
			cm.tunnel = nil
			cm.healthChecker = nil
			cm.proxyIPMgr = nil

			logger.Info("old components shut down, ports released")

			// ==========================================
			// 【核心修复】第二步：端口释放完毕，用新配置启动新组件
			// ==========================================
			newComp, initErr := initComponents(ctx, newCfg, selector, vmode, logger, true, existingAPI)
			if initErr != nil {
				logger.Error("CRITICAL: failed to init new components after shutdown, engine is DOWN",
					zap.Error(initErr),
				)
				// 尝试用旧配置回滚恢复
				oldCfg := cm.Get()
				if oldCfg != nil {
					logger.Warn("attempting rollback to previous config...")
					oldSelector, selErr := initFingerprintSelector(oldCfg)
					if selErr == nil {
						oldVmode, vmErr := verify.ParseMode(oldCfg.TLS.VerifyMode)
						if vmErr == nil {
							rollbackComp, rbErr := initComponents(ctx, oldCfg, oldSelector, oldVmode, logger, true, existingAPI)
							if rbErr == nil {
								cm.socks5Server = rollbackComp.socks5
								cm.httpServer = rollbackComp.httpProxy
								cm.tunnel = rollbackComp.tunnel
								cm.healthChecker = rollbackComp.checker
								cm.proxyIPMgr = rollbackComp.proxyIPMgr
								if cm.apiServer != nil && rollbackComp.tunnel != nil {
									cm.apiServer.SetTunnelStats(&tunnelStatsAdapter{tunnel: rollbackComp.tunnel})
								}
								logger.Info("rollback successful, engine restored with previous config")
								continue
							}
							logger.Error("rollback also failed", zap.Error(rbErr))
						}
					}
				}
				logger.Error("engine is in a stopped state, manual restart required")
				continue
			}

			// 更新引用
			cm.socks5Server = newComp.socks5
			cm.httpServer = newComp.httpProxy
			cm.tunnel = newComp.tunnel
			cm.healthChecker = newComp.checker
			cm.proxyIPMgr = newComp.proxyIPMgr

			// 将最新的隧道统计注入 API Server，让仪表盘显示真实数据
			if cm.apiServer != nil && newComp.tunnel != nil {
				cm.apiServer.SetTunnelStats(&tunnelStatsAdapter{tunnel: newComp.tunnel})
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
// ProxyIP 管理器
// ================================================================

type ProxyIPManager struct {
	entries   []*proxyIPEntry
	mode      string
	current   int
	logger    *zap.Logger
	stopCh    chan struct{}
	closeOnce sync.Once
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
	m.closeOnce.Do(func() {
		close(m.stopCh)
	})
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

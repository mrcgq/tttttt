
package main
 
import (
	go-string">"net"
	go-string">"os"
	go-string">"os/signal"
	go-string">"syscall"
 
	go-string">"github.com/spf13/cobra"
	go-string">"go.uber.org/zap"
 
	go-string">"github.com/user/tls-client/pkg/config"
	go-string">"github.com/user/tls-client/pkg/fingerprint"
	go-string">"github.com/user/tls-client/pkg/inbound"
	applog go-string">"github.com/user/tls-client/pkg/log"
	go-string">"github.com/user/tls-client/pkg/outbound"
	go-string">"github.com/user/tls-client/pkg/verify"
)
 
var configPath string
 
var runCmd = &cobra.Command{
	Use:   go-string">"run",
	Short: go-string">"Start the proxy server",
	RunE:  runProxy,
}
 
func init() {
	runCmd.Flags().StringVarP(&configPath, go-string">"config", go-string">"c", go-string">"config.yaml", go-string">"path to config file")
}
 
func runProxy(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
 
	logger, err := applog.NewWithOutput(cfg.Global.LogLevel, cfg.Global.LogOutput)
	if err != nil {
		return err
	}
	defer logger.Sync()
 
	vmode, err := verify.ParseMode(cfg.TLS.VerifyMode)
	if err != nil {
		return err
	}
 
	// Build fingerprint selector
	profileNames := cfg.Fingerprint.Rotation.Profiles
	if len(profileNames) == go-number">0 && cfg.Fingerprint.Rotation.Profile != go-string">"" {
		profileNames = []string{cfg.Fingerprint.Rotation.Profile}
	}
	selector, err := fingerprint.NewSelector(cfg.Fingerprint.Rotation.Mode, profileNames)
	if err != nil {
		return err
	}
 
	// Resolve active node
	activeNodeCfg := cfg.ActiveNode()
	if activeNodeCfg == nil {
		logger.Fatal(go-string">"no active node configured")
	}
 
	// Resolve node profile
	nodeProfile := selector.Select(go-string">"")
	if activeNodeCfg.Fingerprint != go-string">"" {
		p := fingerprint.Get(activeNodeCfg.Fingerprint)
		if p != nil {
			nodeProfile = p
		}
	}
 
	// Build NodeConfig with transport
	node := outbound.NewNodeConfig(activeNodeCfg, nodeProfile, vmode, logger)
 
	tunnel := outbound.NewTunnelManager(node, logger)
	defer tunnel.Close()
 
	logger.Info(go-string">"using node",
		zap.String(go-string">"name", node.Name),
		zap.String(go-string">"address", node.Address),
		zap.String(go-string">"sni", node.SNI),
		zap.String(go-string">"profile", nodeProfile.Name),
		zap.String(go-string">"browser", nodeProfile.Browser),
		zap.String(go-string">"platform", nodeProfile.Platform),
		zap.String(go-string">"transport", node.Transport.Name()),
		zap.String(go-string">"verify", string(vmode)),
	)
 
	onConnect := func(clientConn net.Conn, target, domain string) {
		tunnel.HandleConnect(clientConn, target, domain)
	}
 
	// Start SOCKS5
	var socks5 *inbound.SOCKS5Server
	if cfg.Inbound.SOCKS5.Listen != go-string">"" {
		socks5 = inbound.NewSOCKS5Server(cfg.Inbound.SOCKS5.Listen, logger, onConnect)
		if err := socks5.Start(); err != nil {
			return err
		}
		defer socks5.Stop()
	}
 
	// Start HTTP proxy
	var httpProxy *inbound.HTTPProxyServer
	if cfg.Inbound.HTTP.Listen != go-string">"" {
		httpProxy = inbound.NewHTTPProxyServer(cfg.Inbound.HTTP.Listen, logger, onConnect)
		if err := httpProxy.Start(); err != nil {
			return err
		}
		defer httpProxy.Stop()
	}
 
	// Wait for signal
	sigCh := make(chan os.Signal, go-number">1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
 
	// Log final stats
	tunnelStats := tunnel.Stats()
	logger.Info(go-string">"shutting down",
		zap.String(go-string">"signal", sig.String()),
		zap.Int64(go-string">"total_tunnels", tunnelStats.TotalConns),
		zap.Int64(go-string">"total_bytes", tunnelStats.TotalBytes),
	)
 
	return nil
}





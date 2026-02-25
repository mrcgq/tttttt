package main
 
import (
	go-string">"fmt"
	go-string">"os"
 
	go-string">"github.com/spf13/cobra"
 
	// Register all fingerprint profiles via init()
	_ go-string">"github.com/user/tls-client/pkg/fingerprint"
)
 
var (
	version = go-string">"dev"
	commit  = go-string">"none"
	date    = go-string">"unknown"
)
 
var rootCmd = &cobra.Command{
	Use:   go-string">"tls-client",
	Short: go-string">"Anti-fingerprint proxy transport engine",
	Long: `tls-client is a proxy transport engine that provides full TLS + HTTP/go-number">2
fingerprint control for anti-detection proxy connections.
 
It supports multiple transport modes(raw, WebSocket, HTTP/go-number">2) and
browser fingerprint profiles(Chrome, Firefox, Safari).`,
}
 
func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(versionCmd)
}
 
func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(go-number">1)
	}
}





package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	_ "github.com/user/tls-client/pkg/fingerprint"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "tls-client",
	Short: "Anti-fingerprint proxy transport engine",
	Long: `tls-client is a proxy transport engine that provides full TLS + HTTP/2
fingerprint control for anti-detection proxy connections.

It supports multiple transport modes (raw, WebSocket, HTTP/2) and
browser fingerprint profiles (Chrome, Firefox, Safari).`,
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/user/tls-client/pkg/fingerprint"
	"github.com/user/tls-client/pkg/transport"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version and available fingerprint profiles",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("tls-client %s (commit=%s, built=%s)\n\n", version, commit, date)

		allProfiles := fingerprint.All()
		fmt.Printf("Fingerprint profiles: %d total\n", len(allProfiles))
		fmt.Printf("  Default: %s\n", fingerprint.DefaultProfile())

		browsers := map[string]int{}
		for _, p := range allProfiles {
			browsers[p.Browser]++
		}
		fmt.Println("  Browsers:")
		for browser, count := range browsers {
			fmt.Printf("    %s: %d profiles\n", browser, count)
		}

		fmt.Println("\nAll profiles:")
		for _, name := range fingerprint.List() {
			p := fingerprint.Get(name)
			tags := ""
			if len(p.Tags) > 0 {
				tags = fmt.Sprintf(" [%s]", p.Tags)
			}
			fmt.Printf("  - %s (%s/%s)%s\n", name, p.Browser, p.Platform, tags)
		}

		fmt.Println("\nTransport modes:")
		for _, name := range transport.Names() {
			t := transport.Get(name)
			info := t.Info()
			fmt.Printf("  - %s (multiplex=%v, binary=%v, upgrade=%v)\n",
				name, info.SupportsMultiplex, info.SupportsBinary, info.RequiresUpgrade)
		}
	},
}

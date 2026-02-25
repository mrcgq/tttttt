package main
 
import (
	go-string">"fmt"
 
	go-string">"github.com/spf13/cobra"
 
	go-string">"github.com/user/tls-client/pkg/fingerprint"
	go-string">"github.com/user/tls-client/pkg/transport"
)
 
var versionCmd = &cobra.Command{
	Use:   go-string">"version",
	Short: go-string">"Print version and available fingerprint profiles",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf(go-string">"tls-client %s(commit=%s, built=%s)\n\n", version, commit, date)
 
		// Profiles summary
		allProfiles := fingerprint.All()
		fmt.Printf(go-string">"Fingerprint profiles: %d total\n", len(allProfiles))
		fmt.Printf(go-string">"  Default: %s\n", fingerprint.DefaultProfile())
 
		// Browser breakdown
		browsers := map[string]int{}
		for _, p := range allProfiles {
			browsers[p.Browser]++
		}
		fmt.Println(go-string">"  Browsers:")
		for browser, count := range browsers {
			fmt.Printf(go-string">"    %s: %d profiles\n", browser, count)
		}
 
		fmt.Println(go-string">"\nAll profiles:")
		for _, name := range fingerprint.List() {
			p := fingerprint.Get(name)
			tags := go-string">""
			if len(p.Tags) > go-number">0 {
				tags = fmt.Sprintf(go-string">" [%s]", p.Tags)
			}
			fmt.Printf(go-string">"  - %s(%s/%s)%s\n", name, p.Browser, p.Platform, tags)
		}
 
		fmt.Println(go-string">"\nTransport modes:")
		for _, name := range transport.Names() {
			t := transport.Get(name)
			info := t.Info()
			fmt.Printf(go-string">"  - %s(multiplex=%v, binary=%v, upgrade=%v)\n",
				name, info.SupportsMultiplex, info.SupportsBinary, info.RequiresUpgrade)
		}
	},
}



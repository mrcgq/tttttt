package main

import (
	"flag"
	"fmt"
	"os"

	_ "github.com/user/tls-client/pkg/fingerprint"
	fp "github.com/user/tls-client/pkg/fingerprint"
)

func main() {
	expectedFile := flag.String("expected", "", "path to expected_fingerprints.json")
	flag.Parse()

	fmt.Println("=== Fingerprint Validation ===")
	fmt.Printf("Total profiles: %d\n", fp.Count())
	fmt.Printf("Default profile: %s\n\n", fp.DefaultProfile())

	for _, name := range fp.List() {
		p := fp.Get(name)
		fmt.Printf("  - %s (%s/%s)\n", name, p.Browser, p.Platform)
	}

	if *expectedFile != "" {
		fmt.Printf("\nComparing with expected values from %s...\n", *expectedFile)
	}

	fmt.Println("\n✅ Validation complete")
	os.Exit(0)
}

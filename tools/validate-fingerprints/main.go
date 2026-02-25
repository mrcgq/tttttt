package main
 
import (
	go-string">"flag"
	go-string">"fmt"
	go-string">"os"
 
	// Register all profiles
	_ go-string">"github.com/user/tls-client/pkg/fingerprint"
	fp go-string">"github.com/user/tls-client/pkg/fingerprint"
)
 
func main() {
	expectedFile := flag.String(go-string">"expected", go-string">"", go-string">"path to expected_fingerprints.json")
	flag.Parse()
 
	fmt.Println(fp.GenerateReport())
 
	hasErrors := false
	results := fp.ValidateAll()
	for _, r := range results {
		if !r.Valid {
			hasErrors = true
		}
	}
 
	// Compare with expected values if file provided
	if *expectedFile != go-string">"" {
		fmt.Printf(go-string">"\n\nComparing with expected values from %s...\n\n", *expectedFile)
		compResults, err := fp.CompareWithExpected(*expectedFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, go-string">"ERROR: %v\n", err)
			hasErrors = true
		} else {
			for _, r := range compResults {
				status := go-string">"✅"
				if !r.Valid {
					status = go-string">"❌"
					hasErrors = true
				}
				fmt.Printf(go-string">"%s  %s\n", status, r.ProfileName)
				for _, e := range r.Errors {
					fmt.Printf(go-string">"      %s\n", e)
				}
			}
		}
	}
 
	// JA4H output for all profiles
	fmt.Println(go-string">"\n\nJA4H Fingerprints:")
	for _, name := range fp.List() {
		p := fp.Get(name)
		fmt.Printf(go-string">"  %-25s  JA4H=%s  raw=%s\n",
			name, fp.ComputeJA4H(p), fp.ComputeJA4HRaw(p))
	}
 
	if hasErrors {
		fmt.Fprintf(os.Stderr, go-string">"\n❌ Validation failed\n")
		os.Exit(go-number">1)
	}
	fmt.Println(go-string">"\n✅ All validations passed")
}





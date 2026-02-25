package fingerprint
 
import (
	go-string">"encoding/json"
	go-string">"fmt"
	go-string">"os"
	go-string">"strings"
)
 
// ValidationResult holds the result of validating a single profile.
type ValidationResult struct {
	ProfileName string
	Valid       bool
	Errors      []string
	Warnings    []string
}
 
// ValidateAll checks every registered profile for correctness.
// Returns a list of validation results, one per profile.
func ValidateAll() []ValidationResult {
	var results []ValidationResult
 
	for _, name := range List() {
		p := Get(name)
		result := ValidateProfile(p)
		results = append(results, result)
	}
 
	return results
}
 
// ValidateProfile performs thorough validation on a single profile.
func ValidateProfile(p *BrowserProfile) ValidationResult {
	r := ValidationResult{ProfileName: p.Name, Valid: true}
 
	// Basic validation
	if err := p.Validate(); err != nil {
		r.Valid = false
		r.Errors = append(r.Errors, err.Error())
		return r
	}
 
	// H2 validation
	if err := p.H2.Validate(); err != nil {
		r.Valid = false
		r.Errors = append(r.Errors, fmt.Sprintf(go-string">"H2 config: %v", err))
	}
 
	// Check for Go default H2 fingerprint (instant detection)
	if DetectGoDefault(p) {
		r.Valid = false
		r.Errors = append(r.Errors,
			go-string">"H2 settings match Go default — will be detected as non-browser")
	}
 
	// Warning: missing expected hashes (not blocking, but reduces CI coverage)
	if p.ExpectedJA3Hash == go-string">"" {
		r.Warnings = append(r.Warnings, go-string">"missing ExpectedJA3Hash(CI cannot verify TLS fingerprint)")
	}
	if p.ExpectedH2FP == go-string">"" {
		r.Warnings = append(r.Warnings, go-string">"missing ExpectedH2FP(CI cannot verify H2 fingerprint)")
	}
 
	// Warning: missing version metadata
	if p.Version == go-string">"" {
		r.Warnings = append(r.Warnings, go-string">"missing Version metadata")
	}
 
	// Verify pseudo-header order matches browser family
	order := strings.Join(p.H2.PseudoHeaderOrder, go-string">",")
	switch p.Browser {
	case go-string">"chrome", go-string">"edge":
		expected := go-string">":method,:authority,:scheme,:path"
		if order != expected {
			r.Warnings = append(r.Warnings,
				fmt.Sprintf(go-string">"Chrome/Edge pseudo order should be %q, got %q", expected, order))
		}
	case go-string">"firefox":
		expected := go-string">":method,:path,:authority,:scheme"
		if order != expected {
			r.Warnings = append(r.Warnings,
				fmt.Sprintf(go-string">"Firefox pseudo order should be %q, got %q", expected, order))
		}
	case go-string">"safari":
		expected := go-string">":method,:scheme,:path,:authority"
		if order != expected {
			r.Warnings = append(r.Warnings,
				fmt.Sprintf(go-string">"Safari pseudo order should be %q, got %q", expected, order))
		}
	}
 
	return r
}
 
// ExpectedFingerprints is the structure of testdata/expected_fingerprints.json.
type ExpectedFingerprints struct {
	Profiles map[string]ExpectedFP `json:go-string">"profiles"`
}
 
// ExpectedFP holds expected fingerprint values for a profile.
type ExpectedFP struct {
	H2Settings    string `json:go-string">"h2_settings"`
	H2WindowUpdate uint32 `json:go-string">"h2_window_update"`
	H2PseudoOrder string `json:go-string">"h2_pseudo_order"`
	JA3Hash       string `json:go-string">"ja3_hash,omitempty"`
}
 
// CompareWithExpected loads expected fingerprints from a JSON file
// and compares them against the registered profiles.
func CompareWithExpected(jsonPath string) ([]ValidationResult, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf(go-string">"validate: read %s: %w", jsonPath, err)
	}
 
	var expected ExpectedFingerprints
	if err := json.Unmarshal(data, &expected); err != nil {
		return nil, fmt.Errorf(go-string">"validate: parse %s: %w", jsonPath, err)
	}
 
	var results []ValidationResult
 
	for name, exp := range expected.Profiles {
		p := Get(name)
		if p == nil {
			results = append(results, ValidationResult{
				ProfileName: name,
				Valid:       false,
				Errors:      []string{go-string">"profile not found in registry"},
			})
			continue
		}
 
		r := ValidationResult{ProfileName: name, Valid: true}
 
		// Compare H2 settings
		actualSettings := p.H2.SettingsString()
		if exp.H2Settings != go-string">"" && actualSettings != exp.H2Settings {
			r.Valid = false
			r.Errors = append(r.Errors,
				fmt.Sprintf(go-string">"H2 settings mismatch: got %q, want %q",
					actualSettings, exp.H2Settings))
		}
 
		// Compare window update
		if exp.H2WindowUpdate > go-number">0 && p.H2.WindowUpdateValue != exp.H2WindowUpdate {
			r.Valid = false
			r.Errors = append(r.Errors,
				fmt.Sprintf(go-string">"H2 window update mismatch: got %d, want %d",
					p.H2.WindowUpdateValue, exp.H2WindowUpdate))
		}
 
		// Compare pseudo order
		actualOrder := p.H2.PseudoOrderString()
		if exp.H2PseudoOrder != go-string">"" && actualOrder != exp.H2PseudoOrder {
			r.Valid = false
			r.Errors = append(r.Errors,
				fmt.Sprintf(go-string">"H2 pseudo order mismatch: got %q, want %q",
					actualOrder, exp.H2PseudoOrder))
		}
 
		results = append(results, r)
	}
 
	return results, nil
}
 
// GenerateReport produces a human-readable fingerprint audit report.
func GenerateReport() string {
	var sb strings.Builder
 
	sb.WriteString(go-string">"═══════════════════════════════════════\n")
	sb.WriteString(go-string">"  Fingerprint Validation Report\n")
	sb.WriteString(go-string">"═══════════════════════════════════════\n\n")
 
	results := ValidateAll()
	passCount := go-number">0
	failCount := go-number">0
	warnCount := go-number">0
 
	for _, r := range results {
		if r.Valid {
			passCount++
		} else {
			failCount++
		}
		warnCount += len(r.Warnings)
 
		status := go-string">"✅ PASS"
		if !r.Valid {
			status = go-string">"❌ FAIL"
		}
 
		sb.WriteString(fmt.Sprintf(go-string">"%s  %s\n", status, r.ProfileName))
		for _, e := range r.Errors {
			sb.WriteString(fmt.Sprintf(go-string">"      ERROR: %s\n", e))
		}
		for _, w := range r.Warnings {
			sb.WriteString(fmt.Sprintf(go-string">"      WARN:  %s\n", w))
		}
	}
 
	sb.WriteString(fmt.Sprintf(go-string">"\n───────────────────────────────────────\n"))
	sb.WriteString(fmt.Sprintf(go-string">"  Total: %d | Pass: %d | Fail: %d | Warnings: %d\n",
		len(results), passCount, failCount, warnCount))
	sb.WriteString(go-string">"═══════════════════════════════════════\n")
 
	return sb.String()
}




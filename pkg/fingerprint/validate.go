package fingerprint

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ValidationResult holds the result of validating a single profile.
type ValidationResult struct {
	ProfileName string
	Valid       bool
	Errors      []string
	Warnings    []string
}

// ValidateAll checks every registered profile for correctness.
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

	if err := p.Validate(); err != nil {
		r.Valid = false
		r.Errors = append(r.Errors, err.Error())
		return r
	}

	if err := p.H2.Validate(); err != nil {
		r.Valid = false
		r.Errors = append(r.Errors, fmt.Sprintf("H2 config: %v", err))
	}

	if DetectGoDefault(p) {
		r.Valid = false
		r.Errors = append(r.Errors,
			"H2 settings match Go default — will be detected as non-browser")
	}

	if p.ExpectedJA3Hash == "" {
		r.Warnings = append(r.Warnings, "missing ExpectedJA3Hash (CI cannot verify TLS fingerprint)")
	}
	if p.ExpectedH2FP == "" {
		r.Warnings = append(r.Warnings, "missing ExpectedH2FP (CI cannot verify H2 fingerprint)")
	}

	if p.Version == "" {
		r.Warnings = append(r.Warnings, "missing Version metadata")
	}

	order := strings.Join(p.H2.PseudoHeaderOrder, ",")
	switch p.Browser {
	case "chrome", "edge":
		expected := ":method,:authority,:scheme,:path"
		if order != expected {
			r.Warnings = append(r.Warnings,
				fmt.Sprintf("Chrome/Edge pseudo order should be %q, got %q", expected, order))
		}
	case "firefox":
		expected := ":method,:path,:authority,:scheme"
		if order != expected {
			r.Warnings = append(r.Warnings,
				fmt.Sprintf("Firefox pseudo order should be %q, got %q", expected, order))
		}
	case "safari":
		expected := ":method,:scheme,:path,:authority"
		if order != expected {
			r.Warnings = append(r.Warnings,
				fmt.Sprintf("Safari pseudo order should be %q, got %q", expected, order))
		}
	}

	return r
}

// ExpectedFingerprints is the structure of testdata/expected_fingerprints.json.
type ExpectedFingerprints struct {
	Profiles map[string]ExpectedFP `json:"profiles"`
}

// ExpectedFP holds expected fingerprint values for a profile.
type ExpectedFP struct {
	H2Settings     string `json:"h2_settings"`
	H2WindowUpdate uint32 `json:"h2_window_update"`
	H2PseudoOrder  string `json:"h2_pseudo_order"`
	JA3Hash        string `json:"ja3_hash,omitempty"`
}

// CompareWithExpected loads expected fingerprints from a JSON file
// and compares them against the registered profiles.
func CompareWithExpected(jsonPath string) ([]ValidationResult, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("validate: read %s: %w", jsonPath, err)
	}

	var expected ExpectedFingerprints
	if err := json.Unmarshal(data, &expected); err != nil {
		return nil, fmt.Errorf("validate: parse %s: %w", jsonPath, err)
	}

	var results []ValidationResult

	for name, exp := range expected.Profiles {
		p := Get(name)
		if p == nil {
			results = append(results, ValidationResult{
				ProfileName: name,
				Valid:       false,
				Errors:      []string{"profile not found in registry"},
			})
			continue
		}

		r := ValidationResult{ProfileName: name, Valid: true}

		actualSettings := p.H2.SettingsString()
		if exp.H2Settings != "" && actualSettings != exp.H2Settings {
			r.Valid = false
			r.Errors = append(r.Errors,
				fmt.Sprintf("H2 settings mismatch: got %q, want %q",
					actualSettings, exp.H2Settings))
		}

		if exp.H2WindowUpdate > 0 && p.H2.WindowUpdateValue != exp.H2WindowUpdate {
			r.Valid = false
			r.Errors = append(r.Errors,
				fmt.Sprintf("H2 window update mismatch: got %d, want %d",
					p.H2.WindowUpdateValue, exp.H2WindowUpdate))
		}

		actualOrder := p.H2.PseudoOrderString()
		if exp.H2PseudoOrder != "" && actualOrder != exp.H2PseudoOrder {
			r.Valid = false
			r.Errors = append(r.Errors,
				fmt.Sprintf("H2 pseudo order mismatch: got %q, want %q",
					actualOrder, exp.H2PseudoOrder))
		}

		results = append(results, r)
	}

	return results, nil
}

// GenerateReport produces a human-readable fingerprint audit report.
func GenerateReport() string {
	var sb strings.Builder

	sb.WriteString("═══════════════════════════════════════\n")
	sb.WriteString("  Fingerprint Validation Report\n")
	sb.WriteString("═══════════════════════════════════════\n\n")

	results := ValidateAll()
	passCount := 0
	failCount := 0
	warnCount := 0

	for _, r := range results {
		if r.Valid {
			passCount++
		} else {
			failCount++
		}
		warnCount += len(r.Warnings)

		status := "✅ PASS"
		if !r.Valid {
			status = "❌ FAIL"
		}

		sb.WriteString(fmt.Sprintf("%s  %s\n", status, r.ProfileName))
		for _, e := range r.Errors {
			sb.WriteString(fmt.Sprintf("      ERROR: %s\n", e))
		}
		for _, w := range r.Warnings {
			sb.WriteString(fmt.Sprintf("      WARN:  %s\n", w))
		}
	}

	sb.WriteString("\n───────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  Total: %d | Pass: %d | Fail: %d | Warnings: %d\n",
		len(results), passCount, failCount, warnCount))
	sb.WriteString("═══════════════════════════════════════\n")

	return sb.String()
}

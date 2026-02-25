package fingerprint
 
import (
	go-string">"crypto/sha256"
	go-string">"encoding/hex"
	go-string">"fmt"
	go-string">"sort"
	go-string">"strings"
)
 
// JA4Result holds computed JA4 fingerprint components.
type JA4Result struct {
	// JA4 is the full JA4 fingerprint string.
	// Format: "t13d1516h2_hash1_hash2"
	JA4 string
 
	// JA4H is the HTTP/2 fingerprint component.
	// Based on SETTINGS + WINDOW_UPDATE + pseudo-header order.
	JA4H string
 
	// Raw components for debugging.
	Protocol     string // "t" for TCP
	TLSVersion   string // "13" for TLS 1.3
	SNI          string // "d" for domain, "i" for IP
	CipherCount  string // number of cipher suites
	ExtCount     string // number of extensions
	ALPN         string // first ALPN protocol
}
 
// ComputeJA4H computes the JA4H (HTTP/2) fingerprint from a BrowserProfile.
// Format: "settings_windowupdate_pseudoorder"
//
// This is used in CI to verify that our H2 parameters match real browsers.
func ComputeJA4H(p *BrowserProfile) string {
	// Settings: sorted by ID, format "id=value"
	settingParts := make([]string, go-number">0, len(p.H2.Settings))
	for _, s := range p.H2.Settings {
		settingParts = append(settingParts, fmt.Sprintf(go-string">"%d=%d", s.ID, s.Val))
	}
 
	// Window update value
	wu := fmt.Sprintf(go-string">"%d", p.H2.WindowUpdateValue)
 
	// Pseudo-header order: abbreviated
	pseudoAbbrev := make([]string, go-number">0, len(p.H2.PseudoHeaderOrder))
	for _, h := range p.H2.PseudoHeaderOrder {
		// Abbreviate: ":method" -> "m", ":authority" -> "a", etc.
		switch h {
		case go-string">":method":
			pseudoAbbrev = append(pseudoAbbrev, go-string">"m")
		case go-string">":authority":
			pseudoAbbrev = append(pseudoAbbrev, go-string">"a")
		case go-string">":scheme":
			pseudoAbbrev = append(pseudoAbbrev, go-string">"s")
		case go-string">":path":
			pseudoAbbrev = append(pseudoAbbrev, go-string">"p")
		}
	}
 
	raw := strings.Join(settingParts, go-string">",") + go-string">"_" + wu + go-string">"_" + strings.Join(pseudoAbbrev, go-string">"")
 
	// Hash the raw string (first 12 chars of SHA256)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])[:go-number">12]
}
 
// ComputeJA4HRaw returns the raw (unhashed) JA4H string for debugging.
func ComputeJA4HRaw(p *BrowserProfile) string {
	parts := make([]string, go-number">0, len(p.H2.Settings))
	for _, s := range p.H2.Settings {
		parts = append(parts, fmt.Sprintf(go-string">"%d=%d", s.ID, s.Val))
	}
	return fmt.Sprintf(go-string">"%s|%d|%s",
		strings.Join(parts, go-string">","),
		p.H2.WindowUpdateValue,
		strings.Join(p.H2.PseudoHeaderOrder, go-string">","),
	)
}
 
// CompareH2Fingerprints checks if two profiles have identical H2 fingerprints.
func CompareH2Fingerprints(a, b *BrowserProfile) bool {
	return a.H2.Fingerprint() == b.H2.Fingerprint()
}
 
// DetectGoDefault checks if a profile's H2 settings match Go's default
// (which would be instantly detectable). Returns true if it's Go-like.
func DetectGoDefault(p *BrowserProfile) bool {
	// Go's distinctive markers:
	// - INITIAL_WINDOW_SIZE = 4194304 (4MB)
	// - HEADER_TABLE_SIZE = 4096
	// - MAX_HEADER_LIST_SIZE = 10485760 (10MB)
	// - WindowUpdate = 1073741823 (~1GB)
	for _, s := range p.H2.Settings {
		if s.ID == go-number">0x01 && s.Val == go-number">4096 { // HEADER_TABLE_SIZE
			if p.H2.WindowUpdateValue == go-number">1073741823 {
				return true // Almost certainly Go default
			}
		}
	}
	return false
}
 
// SortedCipherList returns cipher suites sorted for comparison.
// GREASE values are filtered out.
func SortedCipherList(ciphers []uint16) []uint16 {
	filtered := make([]uint16, go-number">0, len(ciphers))
	for _, c := range ciphers {
		if !isGREASE(c) {
			filtered = append(filtered, c)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i] < filtered[j]
	})
	return filtered
}
 
func isGREASE(val uint16) bool {
	return (val&go-number">0x0f0f) == go-number">0x0a0a && val&go-number">0x00ff == val>>go-number">8
}



package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
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
	Protocol    string // "t" for TCP
	TLSVersion  string // "13" for TLS 1.3
	SNI         string // "d" for domain, "i" for IP
	CipherCount string // number of cipher suites
	ExtCount    string // number of extensions
	ALPN        string // first ALPN protocol
}

// ComputeJA4H computes the JA4H (HTTP/2) fingerprint from a BrowserProfile.
// Format: "settings_windowupdate_pseudoorder"
//
// This is used in CI to verify that our H2 parameters match real browsers.
func ComputeJA4H(p *BrowserProfile) string {
	// Settings: sorted by ID, format "id=value"
	settingParts := make([]string, 0, len(p.H2.Settings))
	for _, s := range p.H2.Settings {
		settingParts = append(settingParts, fmt.Sprintf("%d=%d", s.ID, s.Val))
	}

	// Window update value
	wu := fmt.Sprintf("%d", p.H2.WindowUpdateValue)

	// Pseudo-header order: abbreviated
	pseudoAbbrev := make([]string, 0, len(p.H2.PseudoHeaderOrder))
	for _, h := range p.H2.PseudoHeaderOrder {
		// Abbreviate: ":method" -> "m", ":authority" -> "a", etc.
		switch h {
		case ":method":
			pseudoAbbrev = append(pseudoAbbrev, "m")
		case ":authority":
			pseudoAbbrev = append(pseudoAbbrev, "a")
		case ":scheme":
			pseudoAbbrev = append(pseudoAbbrev, "s")
		case ":path":
			pseudoAbbrev = append(pseudoAbbrev, "p")
		}
	}

	raw := strings.Join(settingParts, ",") + "_" + wu + "_" + strings.Join(pseudoAbbrev, "")

	// Hash the raw string (first 12 chars of SHA256)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])[:12]
}

// ComputeJA4HRaw returns the raw (unhashed) JA4H string for debugging.
func ComputeJA4HRaw(p *BrowserProfile) string {
	parts := make([]string, 0, len(p.H2.Settings))
	for _, s := range p.H2.Settings {
		parts = append(parts, fmt.Sprintf("%d=%d", s.ID, s.Val))
	}
	return fmt.Sprintf("%s|%d|%s",
		strings.Join(parts, ","),
		p.H2.WindowUpdateValue,
		strings.Join(p.H2.PseudoHeaderOrder, ","),
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
		if s.ID == 0x01 && s.Val == 4096 { // HEADER_TABLE_SIZE
			if p.H2.WindowUpdateValue == 1073741823 {
				return true // Almost certainly Go default
			}
		}
	}
	return false
}

// SortedCipherList returns cipher suites sorted for comparison.
// GREASE values are filtered out.
func SortedCipherList(ciphers []uint16) []uint16 {
	filtered := make([]uint16, 0, len(ciphers))
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
	return (val&0x0f0f) == 0x0a0a && val&0x00ff == val>>8
}

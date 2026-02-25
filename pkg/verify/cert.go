package verify

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
)

// Mode defines certificate verification behavior.
type Mode string

const (
	// ModeStrict verifies certificate chain AND requires CN/SAN to match SNI.
	ModeStrict Mode = "strict"

	// ModeSNISkip verifies certificate chain but does NOT require CN/SAN to match SNI.
	// This is the correct mode for domain fronting / SNI spoofing.
	ModeSNISkip Mode = "sni-skip"

	// ModeInsecure skips all certificate verification. Use only for testing.
	ModeInsecure Mode = "insecure"

	// ModePin verifies certificate chain AND checks leaf cert SHA256 pin.
	ModePin Mode = "pin"
)

// Options holds additional verification options.
type Options struct {
	// CustomRoots is an optional custom root CA pool.
	// If nil, system roots are used.
	CustomRoots *x509.CertPool

	// PinnedCertHash is the expected SHA256 hash of the leaf certificate (hex-encoded).
	// Only used when Mode is ModePin.
	PinnedCertHash string
}

// ParseMode parses a string into a Mode value.
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case ModeStrict, ModeSNISkip, ModeInsecure, ModePin:
		return Mode(s), nil
	case "":
		return ModeSNISkip, nil
	default:
		return "", fmt.Errorf("verify: unknown mode %q (want strict, sni-skip, insecure, or pin)", s)
	}
}

// ApplyToTLSConfig configures certificate verification on a tls.Config.
// sni is the SNI value that will be sent (may differ from the actual target domain).
func ApplyToTLSConfig(cfg *tls.Config, mode Mode, sni string, opts *Options) {
	cfg.ServerName = sni

	if opts == nil {
		opts = &Options{}
	}

	switch mode {
	case ModeStrict:
		cfg.InsecureSkipVerify = false
		if opts.CustomRoots != nil {
			cfg.RootCAs = opts.CustomRoots
		}

	case ModeSNISkip:
		cfg.InsecureSkipVerify = true
		cfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("verify[sni-skip]: server presented no certificates")
			}
			certs := make([]*x509.Certificate, len(rawCerts))
			for i, raw := range rawCerts {
				c, err := x509.ParseCertificate(raw)
				if err != nil {
					return fmt.Errorf("verify[sni-skip]: parse cert %d: %w", i, err)
				}
				certs[i] = c
			}
			verifyOpts := x509.VerifyOptions{
				Intermediates: x509.NewCertPool(),
				// No DNSName check - intentional for domain fronting
			}
			if opts.CustomRoots != nil {
				verifyOpts.Roots = opts.CustomRoots
			}
			for _, c := range certs[1:] {
				verifyOpts.Intermediates.AddCert(c)
			}
			_, err := certs[0].Verify(verifyOpts)
			if err != nil {
				return fmt.Errorf("verify[sni-skip]: chain validation failed (issuer=%s): %w",
					certs[0].Issuer.CommonName, err)
			}
			return nil
		}

	case ModePin:
		cfg.InsecureSkipVerify = true
		cfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("verify[pin]: server presented no certificates")
			}
			// Check pin against leaf certificate
			hash := sha256.Sum256(rawCerts[0])
			actualPin := hex.EncodeToString(hash[:])
			if opts.PinnedCertHash != "" && actualPin != opts.PinnedCertHash {
				return fmt.Errorf("verify[pin]: certificate pin mismatch (got %s, want %s)",
					actualPin[:16]+"...", opts.PinnedCertHash[:16]+"...")
			}
			// Also verify chain
			certs := make([]*x509.Certificate, len(rawCerts))
			for i, raw := range rawCerts {
				c, err := x509.ParseCertificate(raw)
				if err != nil {
					return fmt.Errorf("verify[pin]: parse cert %d: %w", i, err)
				}
				certs[i] = c
			}
			verifyOpts := x509.VerifyOptions{
				Intermediates: x509.NewCertPool(),
			}
			if opts.CustomRoots != nil {
				verifyOpts.Roots = opts.CustomRoots
			}
			for _, c := range certs[1:] {
				verifyOpts.Intermediates.AddCert(c)
			}
			_, err := certs[0].Verify(verifyOpts)
			if err != nil {
				return fmt.Errorf("verify[pin]: chain validation failed: %w", err)
			}
			return nil
		}

	case ModeInsecure:
		cfg.InsecureSkipVerify = true
	}
}

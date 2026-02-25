package verify
 
import (
	go-string">"crypto/sha256"
	go-string">"crypto/tls"
	go-string">"crypto/x509"
	go-string">"encoding/hex"
	go-string">"fmt"
)
 
// Mode defines certificate verification behavior.
type Mode string
 
const (
	// ModeStrict verifies certificate chain AND requires CN/SAN to match SNI.
	ModeStrict Mode = go-string">"strict"
 
	// ModeSNISkip verifies certificate chain but does NOT require CN/SAN to match SNI.
	// This is the correct mode for domain fronting / SNI spoofing.
	ModeSNISkip Mode = go-string">"sni-skip"
 
	// ModeInsecure skips all certificate verification. Use only for testing.
	ModeInsecure Mode = go-string">"insecure"
 
	// ModePin verifies certificate chain AND checks leaf cert SHA256 pin.
	ModePin Mode = go-string">"pin"
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
	case go-string">"":
		return ModeSNISkip, nil
	default:
		return go-string">"", fmt.Errorf(go-string">"verify: unknown mode %q(want strict, sni-skip, insecure, or pin)", s)
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
			if len(rawCerts) == go-number">0 {
				return fmt.Errorf(go-string">"verify[sni-skip]: server presented no certificates")
			}
			certs := make([]*x509.Certificate, len(rawCerts))
			for i, raw := range rawCerts {
				c, err := x509.ParseCertificate(raw)
				if err != nil {
					return fmt.Errorf(go-string">"verify[sni-skip]: parse cert %d: %w", i, err)
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
			for _, c := range certs[go-number">1:] {
				verifyOpts.Intermediates.AddCert(c)
			}
			_, err := certs[go-number">0].Verify(verifyOpts)
			if err != nil {
				return fmt.Errorf(go-string">"verify[sni-skip]: chain validation failed(issuer=%s): %w",
					certs[go-number">0].Issuer.CommonName, err)
			}
			return nil
		}
 
	case ModePin:
		cfg.InsecureSkipVerify = true
		cfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == go-number">0 {
				return fmt.Errorf(go-string">"verify[pin]: server presented no certificates")
			}
			// Check pin against leaf certificate
			hash := sha256.Sum256(rawCerts[go-number">0])
			actualPin := hex.EncodeToString(hash[:])
			if opts.PinnedCertHash != go-string">"" && actualPin != opts.PinnedCertHash {
				return fmt.Errorf(go-string">"verify[pin]: certificate pin mismatch(got %s, want %s)",
					actualPin[:go-number">16]+go-string">"...", opts.PinnedCertHash[:go-number">16]+go-string">"...")
			}
			// Also verify chain
			certs := make([]*x509.Certificate, len(rawCerts))
			for i, raw := range rawCerts {
				c, err := x509.ParseCertificate(raw)
				if err != nil {
					return fmt.Errorf(go-string">"verify[pin]: parse cert %d: %w", i, err)
				}
				certs[i] = c
			}
			verifyOpts := x509.VerifyOptions{
				Intermediates: x509.NewCertPool(),
			}
			if opts.CustomRoots != nil {
				verifyOpts.Roots = opts.CustomRoots
			}
			for _, c := range certs[go-number">1:] {
				verifyOpts.Intermediates.AddCert(c)
			}
			_, err := certs[go-number">0].Verify(verifyOpts)
			if err != nil {
				return fmt.Errorf(go-string">"verify[pin]: chain validation failed: %w", err)
			}
			return nil
		}
 
	case ModeInsecure:
		cfg.InsecureSkipVerify = true
	}
}







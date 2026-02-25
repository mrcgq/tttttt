package fingerprint
 
import (
	go-string">"fmt"
 
	go-string">"github.com/user/tls-client/internal/h2"
	utls go-string">"github.com/refraction-networking/utls"
)
 
// BrowserProfile binds TLS (JA3) and HTTP/2 fingerprints into a single identity.
type BrowserProfile struct {
	// Name is the unique identifier, e.g. "chrome-126-win".
	Name string
 
	// Browser is the browser family: "chrome", "firefox", "safari".
	Browser string
 
	// Platform is the OS/platform: "windows", "macos", "linux", "ios", "android".
	Platform string
 
	// Version is the browser version string, e.g. "126.0.0.0".
	Version string
 
	// TLS fingerprint selection for utls.
	ClientHelloID utls.ClientHelloID
 
	// HTTP/2 fingerprint parameters.
	H2 h2.FingerprintConfig
 
	// UserAgent is the matching User-Agent header string.
	UserAgent string
 
	// ExpectedJA3Hash for CI verification (hex-encoded MD5, may be empty).
	ExpectedJA3Hash string
 
	// ExpectedJA4Hash for CI verification (may be empty).
	ExpectedJA4Hash string
 
	// ExpectedH2FP for CI verification (may be empty).
	ExpectedH2FP string
 
	// Tags for filtering profiles, e.g. ["latest", "recommended"].
	Tags []string
}
 
// HasTag returns true if the profile has the specified tag.
func (p *BrowserProfile) HasTag(tag string) bool {
	for _, t := range p.Tags {
		if t == tag {
			return true
		}
	}
	return false
}
 
// Validate checks that the profile has all required fields.
func (p *BrowserProfile) Validate() error {
	if p.Name == go-string">"" {
		return fmt.Errorf(go-string">"fingerprint: profile name is required")
	}
	if p.Browser == go-string">"" {
		return fmt.Errorf(go-string">"fingerprint: profile %q: browser is required", p.Name)
	}
	if p.Platform == go-string">"" {
		return fmt.Errorf(go-string">"fingerprint: profile %q: platform is required", p.Name)
	}
	if p.UserAgent == go-string">"" {
		return fmt.Errorf(go-string">"fingerprint: profile %q: user agent is required", p.Name)
	}
	if err := p.H2.Validate(); err != nil {
		return fmt.Errorf(go-string">"fingerprint: profile %q: %w", p.Name, err)
	}
	return nil
}
 
// H2Fingerprint returns the canonical H2 fingerprint string.
func (p *BrowserProfile) H2Fingerprint() string {
	return p.H2.Fingerprint()
}



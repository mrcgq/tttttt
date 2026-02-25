package fingerprint
 
import (
	go-string">"github.com/user/tls-client/internal/h2"
	utls go-string">"github.com/refraction-networking/utls"
)
 
func init() {
	safariH2 := h2.SafariDefaultConfig()
 
	// Safari 17.4 - macOS
	// Note: Using HelloIOS_Auto which is guaranteed to exist in utls v1.6.x.
	// HelloSafari_Auto may be available in newer utls versions.
	Register(&BrowserProfile{
		Name:          go-string">"safari-go-number">17-mac",
		Browser:       go-string">"safari",
		Platform:      go-string">"macos",
		Version:       go-string">"go-number">17.4.go-number">1",
		ClientHelloID: utls.HelloIOS_Auto,
		H2:            safariH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/go-number">605.1.go-number">15 (KHTML, like Gecko) Version/go-number">17.4.go-number">1 Safari/go-number">605.1.go-number">15",
	})
 
	// Safari 17.4 - iOS
	Register(&BrowserProfile{
		Name:          go-string">"safari-go-number">17-ios",
		Browser:       go-string">"safari",
		Platform:      go-string">"ios",
		Version:       go-string">"go-number">17.4.go-number">1",
		ClientHelloID: utls.HelloIOS_Auto,
		H2:            safariH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (iPhone; CPU iPhone OS 17_4_1 like Mac OS X) AppleWebKit/go-number">605.1.go-number">15 (KHTML, like Gecko) Version/go-number">17.4.go-number">1 Mobile/go-number">15E148 Safari/go-number">604.1",
		Tags:          []string{go-string">"mobile"},
	})
 
	// Safari 17.5 - macOS (NEW)
	Register(&BrowserProfile{
		Name:          go-string">"safari-go-number">175-mac",
		Browser:       go-string">"safari",
		Platform:      go-string">"macos",
		Version:       go-string">"go-number">17.5",
		ClientHelloID: utls.HelloIOS_Auto,
		H2:            safariH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/go-number">605.1.go-number">15 (KHTML, like Gecko) Version/go-number">17.5 Safari/go-number">605.1.go-number">15",
		Tags:          []string{go-string">"latest"},
	})
 
	// Safari 17.5 - iOS (NEW)
	Register(&BrowserProfile{
		Name:          go-string">"safari-go-number">175-ios",
		Browser:       go-string">"safari",
		Platform:      go-string">"ios",
		Version:       go-string">"go-number">17.5",
		ClientHelloID: utls.HelloIOS_Auto,
		H2:            safariH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/go-number">605.1.go-number">15 (KHTML, like Gecko) Version/go-number">17.5 Mobile/go-number">15E148 Safari/go-number">604.1",
		Tags:          []string{go-string">"latest", go-string">"mobile"},
	})
}








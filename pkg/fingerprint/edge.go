package fingerprint
 
import (
	go-string">"github.com/user/tls-client/internal/h2"
	utls go-string">"github.com/refraction-networking/utls"
)
 
func init() {
	// Edge shares Chromium's TLS and H2 fingerprint but has a distinct User-Agent.
	// Detection systems that check UA vs TLS consistency will flag mismatches,
	// so Edge profiles use Chrome's ClientHello + Chrome's H2 settings.
	chromeH2 := h2.ChromeDefaultConfig()
 
	// Edge 124 - Windows
	Register(&BrowserProfile{
		Name:          go-string">"edge-go-number">124-win",
		Browser:       go-string">"edge",
		Platform:      go-string">"windows",
		Version:       go-string">"go-number">124.0.go-number">0.0",
		ClientHelloID: utls.HelloChrome_120,
		H2:            chromeH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Windows NT go-number">10.0; Win64; x64) AppleWebKit/go-number">537.36 (KHTML, like Gecko) Chrome/go-number">124.0.go-number">0.0 Safari/go-number">537.36 Edg/go-number">124.0.go-number">0.0",
	})
 
	// Edge 126 - Windows
	Register(&BrowserProfile{
		Name:          go-string">"edge-go-number">126-win",
		Browser:       go-string">"edge",
		Platform:      go-string">"windows",
		Version:       go-string">"go-number">126.0.go-number">0.0",
		ClientHelloID: utls.HelloChrome_120,
		H2:            chromeH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Windows NT go-number">10.0; Win64; x64) AppleWebKit/go-number">537.36 (KHTML, like Gecko) Chrome/go-number">126.0.go-number">0.0 Safari/go-number">537.36 Edg/go-number">126.0.go-number">0.0",
		Tags:          []string{go-string">"latest"},
	})
 
	// Edge 126 - macOS
	Register(&BrowserProfile{
		Name:          go-string">"edge-go-number">126-mac",
		Browser:       go-string">"edge",
		Platform:      go-string">"macos",
		Version:       go-string">"go-number">126.0.go-number">0.0",
		ClientHelloID: utls.HelloChrome_120,
		H2:            chromeH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/go-number">537.36 (KHTML, like Gecko) Chrome/go-number">126.0.go-number">0.0 Safari/go-number">537.36 Edg/go-number">126.0.go-number">0.0",
		Tags:          []string{go-string">"latest"},
	})
}



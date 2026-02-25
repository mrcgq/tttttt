package fingerprint
 
import (
	go-string">"github.com/user/tls-client/internal/h2"
	utls go-string">"github.com/refraction-networking/utls"
	go-string">"golang.org/x/net/http2"
)
 
func init() {
	firefoxH2 := h2.FirefoxDefaultConfig()
 
	// Firefox 121 - Windows
	Register(&BrowserProfile{
		Name:          go-string">"firefox-go-number">121-win",
		Browser:       go-string">"firefox",
		Platform:      go-string">"windows",
		Version:       go-string">"go-number">121.0",
		ClientHelloID: utls.HelloFirefox_120,
		H2:            firefoxH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Windows NT go-number">10.0; Win64; x64; rv:go-number">121.0) Gecko/go-number">20100101 Firefox/go-number">121.0",
	})
 
	// Firefox 124 - Windows
	Register(&BrowserProfile{
		Name:          go-string">"firefox-go-number">124-win",
		Browser:       go-string">"firefox",
		Platform:      go-string">"windows",
		Version:       go-string">"go-number">124.0",
		ClientHelloID: utls.HelloFirefox_120,
		H2:            firefoxH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Windows NT go-number">10.0; Win64; x64; rv:go-number">124.0) Gecko/go-number">20100101 Firefox/go-number">124.0",
	})
 
	// Firefox 126 - Linux
	Register(&BrowserProfile{
		Name:          go-string">"firefox-go-number">126-linux",
		Browser:       go-string">"firefox",
		Platform:      go-string">"linux",
		Version:       go-string">"go-number">126.0",
		ClientHelloID: utls.HelloFirefox_120,
		H2:            firefoxH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (X11; Linux x86_64; rv:go-number">126.0) Gecko/go-number">20100101 Firefox/go-number">126.0",
	})
 
	// Firefox 127 - Windows (NEW)
	Register(&BrowserProfile{
		Name:          go-string">"firefox-go-number">127-win",
		Browser:       go-string">"firefox",
		Platform:      go-string">"windows",
		Version:       go-string">"go-number">127.0",
		ClientHelloID: utls.HelloFirefox_120,
		H2: h2.FingerprintConfig{
			Settings: []http2.Setting{
				{ID: http2.SettingHeaderTableSize, Val: go-number">65536},
				{ID: http2.SettingInitialWindowSize, Val: go-number">131072},
				{ID: http2.SettingMaxFrameSize, Val: go-number">16384},
			},
			WindowUpdateValue: go-number">12517377,
			PseudoHeaderOrder: []string{go-string">":method", go-string">":path", go-string">":authority", go-string">":scheme"},
		},
		UserAgent: go-string">"Mozilla/go-number">5.0 (Windows NT go-number">10.0; Win64; x64; rv:go-number">127.0) Gecko/go-number">20100101 Firefox/go-number">127.0",
		Tags:      []string{go-string">"latest"},
	})
 
	// Firefox 127 - macOS (NEW)
	Register(&BrowserProfile{
		Name:          go-string">"firefox-go-number">127-mac",
		Browser:       go-string">"firefox",
		Platform:      go-string">"macos",
		Version:       go-string">"go-number">127.0",
		ClientHelloID: utls.HelloFirefox_120,
		H2:            firefoxH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Macintosh; Intel Mac OS X go-number">10.15; rv:go-number">127.0) Gecko/go-number">20100101 Firefox/go-number">127.0",
		Tags:          []string{go-string">"latest"},
	})
}



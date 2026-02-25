package fingerprint
 
import (
	go-string">"github.com/user/tls-client/internal/h2"
	utls go-string">"github.com/refraction-networking/utls"
	go-string">"golang.org/x/net/http2"
)
 
func init() {
	chromeH2 := h2.ChromeDefaultConfig()
 
	// Chrome 120 - Windows
	Register(&BrowserProfile{
		Name:          go-string">"chrome-go-number">120-win",
		Browser:       go-string">"chrome",
		Platform:      go-string">"windows",
		Version:       go-string">"go-number">120.0.go-number">0.0",
		ClientHelloID: utls.HelloChrome_120,
		H2:            chromeH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Windows NT go-number">10.0; Win64; x64) AppleWebKit/go-number">537.36 (KHTML, like Gecko) Chrome/go-number">120.0.go-number">0.0 Safari/go-number">537.36",
	})
 
	// Chrome 123 - Windows
	Register(&BrowserProfile{
		Name:          go-string">"chrome-go-number">123-win",
		Browser:       go-string">"chrome",
		Platform:      go-string">"windows",
		Version:       go-string">"go-number">123.0.go-number">0.0",
		ClientHelloID: utls.HelloChrome_120,
		H2:            chromeH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Windows NT go-number">10.0; Win64; x64) AppleWebKit/go-number">537.36 (KHTML, like Gecko) Chrome/go-number">123.0.go-number">0.0 Safari/go-number">537.36",
	})
 
	// Chrome 124 - Windows
	Register(&BrowserProfile{
		Name:          go-string">"chrome-go-number">124-win",
		Browser:       go-string">"chrome",
		Platform:      go-string">"windows",
		Version:       go-string">"go-number">124.0.go-number">0.0",
		ClientHelloID: utls.HelloChrome_120,
		H2:            chromeH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Windows NT go-number">10.0; Win64; x64) AppleWebKit/go-number">537.36 (KHTML, like Gecko) Chrome/go-number">124.0.go-number">0.0 Safari/go-number">537.36",
	})
 
	// Chrome 125 - Windows
	Register(&BrowserProfile{
		Name:          go-string">"chrome-go-number">125-win",
		Browser:       go-string">"chrome",
		Platform:      go-string">"windows",
		Version:       go-string">"go-number">125.0.go-number">0.0",
		ClientHelloID: utls.HelloChrome_120,
		H2:            chromeH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Windows NT go-number">10.0; Win64; x64) AppleWebKit/go-number">537.36 (KHTML, like Gecko) Chrome/go-number">125.0.go-number">0.0 Safari/go-number">537.36",
	})
 
	// Chrome 126 - Windows (NEW DEFAULT)
	Register(&BrowserProfile{
		Name:          go-string">"chrome-go-number">126-win",
		Browser:       go-string">"chrome",
		Platform:      go-string">"windows",
		Version:       go-string">"go-number">126.0.go-number">0.0",
		ClientHelloID: utls.HelloChrome_120,
		H2:            chromeH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Windows NT go-number">10.0; Win64; x64) AppleWebKit/go-number">537.36 (KHTML, like Gecko) Chrome/go-number">126.0.go-number">0.0 Safari/go-number">537.36",
		Tags:          []string{go-string">"latest", go-string">"recommended", go-string">"default"},
	})
 
	// Chrome 126 - macOS
	Register(&BrowserProfile{
		Name:          go-string">"chrome-go-number">126-mac",
		Browser:       go-string">"chrome",
		Platform:      go-string">"macos",
		Version:       go-string">"go-number">126.0.go-number">0.0",
		ClientHelloID: utls.HelloChrome_120,
		H2:            chromeH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/go-number">537.36 (KHTML, like Gecko) Chrome/go-number">126.0.go-number">0.0 Safari/go-number">537.36",
		Tags:          []string{go-string">"latest"},
	})
 
	// Chrome 124 - macOS
	Register(&BrowserProfile{
		Name:          go-string">"chrome-go-number">124-mac",
		Browser:       go-string">"chrome",
		Platform:      go-string">"macos",
		Version:       go-string">"go-number">124.0.go-number">0.0",
		ClientHelloID: utls.HelloChrome_120,
		H2:            chromeH2,
		UserAgent:     go-string">"Mozilla/go-number">5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/go-number">537.36 (KHTML, like Gecko) Chrome/go-number">124.0.go-number">0.0 Safari/go-number">537.36",
	})
 
	// Chrome 126 - Android
	Register(&BrowserProfile{
		Name:          go-string">"chrome-go-number">126-android",
		Browser:       go-string">"chrome",
		Platform:      go-string">"android",
		Version:       go-string">"go-number">126.0.go-number">0.0",
		ClientHelloID: utls.HelloChrome_120,
		H2: h2.FingerprintConfig{
			Settings: []http2.Setting{
				{ID: http2.SettingHeaderTableSize, Val: go-number">65536},
				{ID: http2.SettingEnablePush, Val: go-number">0},
				{ID: http2.SettingMaxConcurrentStreams, Val: go-number">1000},
				{ID: http2.SettingInitialWindowSize, Val: go-number">6291456},
				{ID: http2.SettingMaxHeaderListSize, Val: go-number">262144},
			},
			WindowUpdateValue: go-number">15663105,
			PseudoHeaderOrder: []string{go-string">":method", go-string">":authority", go-string">":scheme", go-string">":path"},
		},
		UserAgent: go-string">"Mozilla/go-number">5.0 (Linux; Android go-number">14; Pixel go-number">8) AppleWebKit/go-number">537.36 (KHTML, like Gecko) Chrome/go-number">126.0.go-number">0.0 Mobile Safari/go-number">537.36",
		Tags:      []string{go-string">"latest", go-string">"mobile"},
	})
}



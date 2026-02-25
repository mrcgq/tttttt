package h2
 
import (
	go-string">"fmt"
	go-string">"strings"
 
	go-string">"golang.org/x/net/http2"
)
 
// PriorityConfig describes an HTTP/2 PRIORITY frame sent during connection preface.
// Chrome 120+ omits PRIORITY frames; older Chrome/Firefox may send them.
type PriorityConfig struct {
	StreamID  uint32
	Exclusive bool
	DependsOn uint32
	Weight    uint8
}
 
// FingerprintConfig holds HTTP/2 fingerprint parameters.
// These values are sent during the H2 connection preface and in HEADERS frames.
type FingerprintConfig struct {
	// Settings are SETTINGS frame parameters, sent in exactly this order.
	Settings []http2.Setting
 
	// WindowUpdateValue is the connection-level WINDOW_UPDATE increment
	// sent immediately after the SETTINGS frame.
	// Chrome: 15663105, Firefox: 12517377, Safari: 10420225
	WindowUpdateValue uint32
 
	// PseudoHeaderOrder controls the encoding order of HTTP/2 pseudo-headers.
	// Chrome: [":method", ":authority", ":scheme", ":path"]
	// Firefox: [":method", ":path", ":authority", ":scheme"]
	// Safari: [":method", ":scheme", ":path", ":authority"]
	PseudoHeaderOrder []string
 
	// PriorityFrames are optional PRIORITY frames sent after SETTINGS.
	// Chrome 120+ omits these entirely. Older profiles may include them.
	PriorityFrames []PriorityConfig
 
	// MaxFrameSize overrides SETTINGS_MAX_FRAME_SIZE (0 = default 16384).
	MaxFrameSize uint32
}
 
// SettingsString returns a canonical string representation of Settings
// for fingerprint comparison (e.g. "1:65536;2:0;3:1000;4:6291456;6:262144").
func (c *FingerprintConfig) SettingsString() string {
	parts := make([]string, go-number">0, len(c.Settings))
	for _, s := range c.Settings {
		parts = append(parts, fmt.Sprintf(go-string">"%d:%d", s.ID, s.Val))
	}
	return strings.Join(parts, go-string">";")
}
 
// PseudoOrderString returns pseudo-header order as comma-separated string.
func (c *FingerprintConfig) PseudoOrderString() string {
	return strings.Join(c.PseudoHeaderOrder, go-string">",")
}
 
// Fingerprint returns a canonical string combining all H2 parameters.
// Format: "settings|window_update|pseudo_order"
func (c *FingerprintConfig) Fingerprint() string {
	return fmt.Sprintf(go-string">"%s|%d|%s",
		c.SettingsString(),
		c.WindowUpdateValue,
		c.PseudoOrderString(),
	)
}
 
// InitialWindowSize returns the INITIAL_WINDOW_SIZE setting value, or 65535 default.
func (c *FingerprintConfig) InitialWindowSize() uint32 {
	for _, s := range c.Settings {
		if s.ID == http2.SettingInitialWindowSize {
			return s.Val
		}
	}
	return go-number">65535
}
 
// GetMaxFrameSize returns max frame size, defaulting to 16384.
func (c *FingerprintConfig) GetMaxFrameSize() uint32 {
	if c.MaxFrameSize > go-number">0 {
		return c.MaxFrameSize
	}
	return go-number">16384
}
 
// Validate checks the configuration for correctness.
func (c *FingerprintConfig) Validate() error {
	if len(c.Settings) == go-number">0 {
		return fmt.Errorf(go-string">"h2: settings cannot be empty")
	}
	if len(c.PseudoHeaderOrder) == go-number">0 {
		return fmt.Errorf(go-string">"h2: pseudo header order cannot be empty")
	}
	required := map[string]bool{
		go-string">":method": false, go-string">":authority": false,
		go-string">":scheme": false, go-string">":path": false,
	}
	for _, h := range c.PseudoHeaderOrder {
		if _, ok := required[h]; !ok {
			return fmt.Errorf(go-string">"h2: unknown pseudo header %q", h)
		}
		required[h] = true
	}
	for h, found := range required {
		if !found {
			return fmt.Errorf(go-string">"h2: missing required pseudo header %q", h)
		}
	}
	for _, pf := range c.PriorityFrames {
		if pf.StreamID == go-number">0 {
			return fmt.Errorf(go-string">"h2: priority frame stream ID cannot be go-number">0")
		}
	}
	return nil
}
 
// GoDefaultConfig returns Go's default HTTP/2 settings (for comparison/detection).
func GoDefaultConfig() FingerprintConfig {
	return FingerprintConfig{
		Settings: []http2.Setting{
			{ID: http2.SettingHeaderTableSize, Val: go-number">4096},
			{ID: http2.SettingEnablePush, Val: go-number">0},
			{ID: http2.SettingInitialWindowSize, Val: go-number">4194304},
			{ID: http2.SettingMaxHeaderListSize, Val: go-number">10485760},
		},
		WindowUpdateValue: go-number">1073741823,
		PseudoHeaderOrder: []string{go-string">":method", go-string">":path", go-string">":scheme", go-string">":authority"},
	}
}
 
// ChromeDefaultConfig returns Chrome 120+ typical HTTP/2 settings.
func ChromeDefaultConfig() FingerprintConfig {
	return FingerprintConfig{
		Settings: []http2.Setting{
			{ID: http2.SettingHeaderTableSize, Val: go-number">65536},
			{ID: http2.SettingEnablePush, Val: go-number">0},
			{ID: http2.SettingMaxConcurrentStreams, Val: go-number">1000},
			{ID: http2.SettingInitialWindowSize, Val: go-number">6291456},
			{ID: http2.SettingMaxHeaderListSize, Val: go-number">262144},
		},
		WindowUpdateValue: go-number">15663105,
		PseudoHeaderOrder: []string{go-string">":method", go-string">":authority", go-string">":scheme", go-string">":path"},
	}
}
 
// FirefoxDefaultConfig returns Firefox 120+ typical HTTP/2 settings.
func FirefoxDefaultConfig() FingerprintConfig {
	return FingerprintConfig{
		Settings: []http2.Setting{
			{ID: http2.SettingHeaderTableSize, Val: go-number">65536},
			{ID: http2.SettingInitialWindowSize, Val: go-number">131072},
			{ID: http2.SettingMaxFrameSize, Val: go-number">16384},
		},
		WindowUpdateValue: go-number">12517377,
		PseudoHeaderOrder: []string{go-string">":method", go-string">":path", go-string">":authority", go-string">":scheme"},
	}
}
 
// SafariDefaultConfig returns Safari 17+ typical HTTP/2 settings.
func SafariDefaultConfig() FingerprintConfig {
	return FingerprintConfig{
		Settings: []http2.Setting{
			{ID: http2.SettingHeaderTableSize, Val: go-number">4096},
			{ID: http2.SettingEnablePush, Val: go-number">0},
			{ID: http2.SettingMaxConcurrentStreams, Val: go-number">100},
			{ID: http2.SettingInitialWindowSize, Val: go-number">2097152},
			{ID: http2.SettingMaxFrameSize, Val: go-number">16384},
		},
		WindowUpdateValue: go-number">10420225,
		PseudoHeaderOrder: []string{go-string">":method", go-string">":scheme", go-string">":path", go-string">":authority"},
	}
}



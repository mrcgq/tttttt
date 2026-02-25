package h2
 
import (
	go-string">"fmt"
	go-string">"strings"
 
	go-string">"golang.org/x/net/http2"
)
 
// ComparisonResult holds the result of comparing two H2 fingerprint configs.
type ComparisonResult struct {
	Identical       bool
	SettingsDiff    []SettingDiff
	WindowUpdateDiff *WindowUpdateDiff
	PseudoOrderDiff *PseudoOrderDiff
}
 
// SettingDiff describes a difference in a single SETTINGS parameter.
type SettingDiff struct {
	ID     http2.SettingID
	NameA  string // empty if missing from A
	NameB  string
	ValueA uint32
	ValueB uint32
	OnlyIn string // "A", "B", or "" if in both
}
 
// WindowUpdateDiff describes a difference in WINDOW_UPDATE values.
type WindowUpdateDiff struct {
	ValueA uint32
	ValueB uint32
}
 
// PseudoOrderDiff describes a difference in pseudo-header ordering.
type PseudoOrderDiff struct {
	OrderA []string
	OrderB []string
}
 
// CompareConfigs performs a field-by-field comparison of two H2 configs.
func CompareConfigs(a, b *FingerprintConfig) ComparisonResult {
	result := ComparisonResult{Identical: true}
 
	// Compare settings
	aSettings := make(map[http2.SettingID]uint32)
	for _, s := range a.Settings {
		aSettings[s.ID] = s.Val
	}
	bSettings := make(map[http2.SettingID]uint32)
	for _, s := range b.Settings {
		bSettings[s.ID] = s.Val
	}
 
	// Check all settings in A
	for id, valA := range aSettings {
		if valB, ok := bSettings[id]; ok {
			if valA != valB {
				result.Identical = false
				result.SettingsDiff = append(result.SettingsDiff, SettingDiff{
					ID: id, ValueA: valA, ValueB: valB,
				})
			}
		} else {
			result.Identical = false
			result.SettingsDiff = append(result.SettingsDiff, SettingDiff{
				ID: id, ValueA: valA, OnlyIn: go-string">"A",
			})
		}
	}
 
	// Check settings only in B
	for id, valB := range bSettings {
		if _, ok := aSettings[id]; !ok {
			result.Identical = false
			result.SettingsDiff = append(result.SettingsDiff, SettingDiff{
				ID: id, ValueB: valB, OnlyIn: go-string">"B",
			})
		}
	}
 
	// Compare window update
	if a.WindowUpdateValue != b.WindowUpdateValue {
		result.Identical = false
		result.WindowUpdateDiff = &WindowUpdateDiff{
			ValueA: a.WindowUpdateValue,
			ValueB: b.WindowUpdateValue,
		}
	}
 
	// Compare pseudo-header order
	orderA := strings.Join(a.PseudoHeaderOrder, go-string">",")
	orderB := strings.Join(b.PseudoHeaderOrder, go-string">",")
	if orderA != orderB {
		result.Identical = false
		result.PseudoOrderDiff = &PseudoOrderDiff{
			OrderA: a.PseudoHeaderOrder,
			OrderB: b.PseudoHeaderOrder,
		}
	}
 
	return result
}
 
// DetectBrowser attempts to identify the browser family from H2 settings.
// Returns "chrome", "firefox", "safari", "go", or "unknown".
func DetectBrowser(cfg *FingerprintConfig) string {
	// Go detection: distinctive INITIAL_WINDOW_SIZE + huge WINDOW_UPDATE
	if cfg.WindowUpdateValue > go-number">1000000000 { // >1GB typical of Go
		return go-string">"go"
	}
 
	// Chrome: WindowUpdate ~15.6M, 5 settings including MAX_CONCURRENT_STREAMS
	if cfg.WindowUpdateValue >= go-number">15000000 && cfg.WindowUpdateValue <= go-number">16000000 {
		for _, s := range cfg.Settings {
			if s.ID == go-number">3 && s.Val == go-number">1000 { // MAX_CONCURRENT_STREAMS = 1000
				return go-string">"chrome"
			}
		}
	}
 
	// Firefox: WindowUpdate ~12.5M, 3 settings, different pseudo order
	if cfg.WindowUpdateValue >= go-number">12000000 && cfg.WindowUpdateValue <= go-number">13000000 {
		return go-string">"firefox"
	}
 
	// Safari: WindowUpdate ~10.4M, MAX_CONCURRENT_STREAMS = 100
	if cfg.WindowUpdateValue >= go-number">10000000 && cfg.WindowUpdateValue <= go-number">11000000 {
		for _, s := range cfg.Settings {
			if s.ID == go-number">3 && s.Val == go-number">100 {
				return go-string">"safari"
			}
		}
	}
 
	return go-string">"unknown"
}
 
// DiffReport generates a human-readable comparison between two H2 configs.
func DiffReport(nameA, nameB string, a, b *FingerprintConfig) string {
	result := CompareConfigs(a, b)
	if result.Identical {
		return fmt.Sprintf(go-string">"%s and %s: identical H2 fingerprint\n", nameA, nameB)
	}
 
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(go-string">"H2 Fingerprint Diff: %s vs %s\n", nameA, nameB))
	sb.WriteString(strings.Repeat(go-string">"─", go-number">60) + go-string">"\n")
 
	for _, d := range result.SettingsDiff {
		switch d.OnlyIn {
		case go-string">"A":
			sb.WriteString(fmt.Sprintf(go-string">"  SETTING %d: only in %s(val=%d)\n",
				d.ID, nameA, d.ValueA))
		case go-string">"B":
			sb.WriteString(fmt.Sprintf(go-string">"  SETTING %d: only in %s(val=%d)\n",
				d.ID, nameB, d.ValueB))
		default:
			sb.WriteString(fmt.Sprintf(go-string">"  SETTING %d: %s=%d vs %s=%d\n",
				d.ID, nameA, d.ValueA, nameB, d.ValueB))
		}
	}
 
	if result.WindowUpdateDiff != nil {
		sb.WriteString(fmt.Sprintf(go-string">"  WINDOW_UPDATE: %s=%d vs %s=%d\n",
			nameA, result.WindowUpdateDiff.ValueA,
			nameB, result.WindowUpdateDiff.ValueB))
	}
 
	if result.PseudoOrderDiff != nil {
		sb.WriteString(fmt.Sprintf(go-string">"  PSEUDO_ORDER: %s=%v vs %s=%v\n",
			nameA, result.PseudoOrderDiff.OrderA,
			nameB, result.PseudoOrderDiff.OrderB))
	}
 
	return sb.String()
}
 
// settingName returns a human-readable name for a SETTINGS parameter ID.
func settingName(id http2.SettingID) string {
	switch id {
	case http2.SettingHeaderTableSize:
		return go-string">"HEADER_TABLE_SIZE"
	case http2.SettingEnablePush:
		return go-string">"ENABLE_PUSH"
	case http2.SettingMaxConcurrentStreams:
		return go-string">"MAX_CONCURRENT_STREAMS"
	case http2.SettingInitialWindowSize:
		return go-string">"INITIAL_WINDOW_SIZE"
	case http2.SettingMaxFrameSize:
		return go-string">"MAX_FRAME_SIZE"
	case http2.SettingMaxHeaderListSize:
		return go-string">"MAX_HEADER_LIST_SIZE"
	default:
		return fmt.Sprintf(go-string">"UNKNOWN_%d", id)
	}
}






package h2

import (
	"fmt"
	"strings"

	"golang.org/x/net/http2"
)

type ComparisonResult struct {
	Identical        bool
	SettingsDiff     []SettingDiff
	WindowUpdateDiff *WindowUpdateDiff
	PseudoOrderDiff  *PseudoOrderDiff
}

type SettingDiff struct {
	ID     http2.SettingID
	NameA  string
	NameB  string
	ValueA uint32
	ValueB uint32
	OnlyIn string
}

type WindowUpdateDiff struct {
	ValueA uint32
	ValueB uint32
}

type PseudoOrderDiff struct {
	OrderA []string
	OrderB []string
}

func CompareConfigs(a, b *FingerprintConfig) ComparisonResult {
	result := ComparisonResult{Identical: true}

	aSettings := make(map[http2.SettingID]uint32)
	for _, s := range a.Settings {
		aSettings[s.ID] = s.Val
	}
	bSettings := make(map[http2.SettingID]uint32)
	for _, s := range b.Settings {
		bSettings[s.ID] = s.Val
	}

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
				ID: id, ValueA: valA, OnlyIn: "A",
			})
		}
	}

	for id, valB := range bSettings {
		if _, ok := aSettings[id]; !ok {
			result.Identical = false
			result.SettingsDiff = append(result.SettingsDiff, SettingDiff{
				ID: id, ValueB: valB, OnlyIn: "B",
			})
		}
	}

	if a.WindowUpdateValue != b.WindowUpdateValue {
		result.Identical = false
		result.WindowUpdateDiff = &WindowUpdateDiff{
			ValueA: a.WindowUpdateValue,
			ValueB: b.WindowUpdateValue,
		}
	}

	orderA := strings.Join(a.PseudoHeaderOrder, ",")
	orderB := strings.Join(b.PseudoHeaderOrder, ",")
	if orderA != orderB {
		result.Identical = false
		result.PseudoOrderDiff = &PseudoOrderDiff{
			OrderA: a.PseudoHeaderOrder,
			OrderB: b.PseudoHeaderOrder,
		}
	}

	return result
}

func DetectBrowser(cfg *FingerprintConfig) string {
	if cfg.WindowUpdateValue > 1000000000 {
		return "go"
	}

	if cfg.WindowUpdateValue >= 15000000 && cfg.WindowUpdateValue <= 16000000 {
		for _, s := range cfg.Settings {
			if s.ID == 3 && s.Val == 1000 {
				return "chrome"
			}
		}
	}

	if cfg.WindowUpdateValue >= 12000000 && cfg.WindowUpdateValue <= 13000000 {
		return "firefox"
	}

	if cfg.WindowUpdateValue >= 10000000 && cfg.WindowUpdateValue <= 11000000 {
		for _, s := range cfg.Settings {
			if s.ID == 3 && s.Val == 100 {
				return "safari"
			}
		}
	}

	return "unknown"
}

func DiffReport(nameA, nameB string, a, b *FingerprintConfig) string {
	result := CompareConfigs(a, b)
	if result.Identical {
		return fmt.Sprintf("%s and %s: identical H2 fingerprint\n", nameA, nameB)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("H2 Fingerprint Diff: %s vs %s\n", nameA, nameB))
	sb.WriteString(strings.Repeat("-", 60) + "\n")

	for _, d := range result.SettingsDiff {
		switch d.OnlyIn {
		case "A":
			sb.WriteString(fmt.Sprintf("  SETTING %d: only in %s (val=%d)\n", d.ID, nameA, d.ValueA))
		case "B":
			sb.WriteString(fmt.Sprintf("  SETTING %d: only in %s (val=%d)\n", d.ID, nameB, d.ValueB))
		default:
			sb.WriteString(fmt.Sprintf("  SETTING %d: %s=%d vs %s=%d\n", d.ID, nameA, d.ValueA, nameB, d.ValueB))
		}
	}

	if result.WindowUpdateDiff != nil {
		sb.WriteString(fmt.Sprintf("  WINDOW_UPDATE: %s=%d vs %s=%d\n",
			nameA, result.WindowUpdateDiff.ValueA,
			nameB, result.WindowUpdateDiff.ValueB))
	}

	if result.PseudoOrderDiff != nil {
		sb.WriteString(fmt.Sprintf("  PSEUDO_ORDER: %s=%v vs %s=%v\n",
			nameA, result.PseudoOrderDiff.OrderA,
			nameB, result.PseudoOrderDiff.OrderB))
	}

	return sb.String()
}

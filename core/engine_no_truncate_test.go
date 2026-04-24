package core

import (
	"strings"
	"testing"
)

// Default DisplayCfg should NOT truncate tool/thinking bodies — the
// platform-side sharder is now the single source of truth for size limits.
func TestDefaultDisplayCfg_NoTruncation(t *testing.T) {
	e := NewEngine("test", &stubAgent{}, nil, "", LangEnglish)
	if e.display.ThinkingMaxLen != 0 {
		t.Errorf("default ThinkingMaxLen = %d, want 0 (no truncation)", e.display.ThinkingMaxLen)
	}
	if e.display.ToolMaxLen != 0 {
		t.Errorf("default ToolMaxLen = %d, want 0 (no truncation)", e.display.ToolMaxLen)
	}
}

// truncateIf's 0-means-no-truncation contract must hold for huge bodies.
func TestTruncateIf_ZeroLeavesBodyIntact(t *testing.T) {
	body := strings.Repeat("X", 10_000)
	out := truncateIf(body, 0)
	if out != body {
		t.Errorf("truncateIf(body, 0) mutated body (in=%d out=%d)", len(body), len(out))
	}
}

// Compact style (single-message markdown fallback) keeps the 150-entry cap
// so a single Feishu message body stays bounded.
func TestProgressCompact_CompactKeepsWindow(t *testing.T) {
	p := &stubCompactProgressPlatform{
		stubPlatformEngine: stubPlatformEngine{n: "test"},
		style:              "compact",
	}
	w := newCompactProgressWriter(t.Context(), p, nil, "CC", LangEnglish, nil)
	if w.maxEntries != 150 {
		t.Errorf("compact maxEntries = %d, want 150", w.maxEntries)
	}
}

// Card mode with structured payload is paginated across multiple cards by
// the platform side; shrinking items would shrink the card count and cause
// bot-self-withdraw of earlier cards. Windowing must be disabled here.
func TestProgressCompact_CardPayloadNoWindowing(t *testing.T) {
	p := &stubCompactProgressPlatform{
		stubPlatformEngine: stubPlatformEngine{n: "test"},
		style:              "card",
		supportPayload:     true,
	}
	w := newCompactProgressWriter(t.Context(), p, nil, "CC", LangEnglish, nil)
	if !w.usePayload {
		t.Fatal("usePayload not set on card+payload platform")
	}
	if w.maxEntries != 0 {
		t.Errorf("card+payload maxEntries = %d, want 0", w.maxEntries)
	}
}

// Card mode WITHOUT structured payload falls back to single-message markdown
// rendering, so it still needs the 150-entry cap.
func TestProgressCompact_CardNoPayloadKeepsWindow(t *testing.T) {
	p := &stubCompactProgressPlatform{
		stubPlatformEngine: stubPlatformEngine{n: "test"},
		style:              "card",
		supportPayload:     false,
	}
	w := newCompactProgressWriter(t.Context(), p, nil, "CC", LangEnglish, nil)
	if w.usePayload {
		t.Fatal("usePayload must be false when platform does not support payload")
	}
	if w.maxEntries != 150 {
		t.Errorf("card+no-payload maxEntries = %d, want 150", w.maxEntries)
	}
}

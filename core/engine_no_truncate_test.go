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

// The compact progress writer's default maxEntries must be 150 so the
// card paginator has a full page budget to work with.
func TestProgressCompact_MaxEntriesRaised(t *testing.T) {
	p := &stubPlatformEngine{n: "test"}
	ctor := newCompactProgressWriter(t.Context(), p, nil, "CC", LangEnglish, nil)
	if ctor.maxEntries != 150 {
		t.Errorf("default maxEntries = %d, want 150", ctor.maxEntries)
	}
}

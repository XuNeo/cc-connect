package core

import (
	"encoding/json"
	"testing"
)

func TestProgressCardEntry_HasPanelFields(t *testing.T) {
	entry := ProgressCardEntry{
		Kind:       ProgressEntryToolUse,
		Text:       "ls",
		Tool:       "Bash",
		ID:         "tool_1",
		DurationMs: 42,
		PartIdx:    1,
		PartTotal:  3,
	}
	b, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{`"id":"tool_1"`, `"duration_ms":42`, `"part_idx":1`, `"part_total":3`} {
		if !contains(s, want) {
			t.Errorf("json missing %s: %s", want, s)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func TestProgressCardEntry_OmitsZeroPanelFields(t *testing.T) {
	entry := ProgressCardEntry{Kind: ProgressEntryInfo, Text: "hello"}
	b, _ := json.Marshal(entry)
	s := string(b)
	for _, bad := range []string{`"duration_ms"`, `"part_idx"`, `"part_total"`, `"id"`} {
		if contains(s, bad) {
			t.Errorf("zero field should be omitempty: %s in %s", bad, s)
		}
	}
}

package feishu

import (
	"encoding/json"
	"strings"
	"testing"

	core "github.com/chenhg5/cc-connect/core"
)

func TestBuildProgressCardJSON_HasUpdateMulti(t *testing.T) {
	payload := &core.ProgressCardPayload{
		Version: 1,
		Agent:   "CC",
		Lang:    "en",
		State:   core.ProgressCardStateRunning,
		Items: []core.ProgressCardEntry{
			{Kind: core.ProgressEntryInfo, Text: "hello"},
		},
	}
	got := buildProgressCardJSONFromPayload(payload)
	var card map[string]any
	if err := json.Unmarshal([]byte(got), &card); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	cfg, ok := card["config"].(map[string]any)
	if !ok {
		t.Fatalf("config missing or wrong type: %v", card["config"])
	}
	if v, _ := cfg["update_multi"].(bool); !v {
		t.Errorf("config.update_multi = %v, want true. card=%s", cfg["update_multi"], strings.TrimSpace(got))
	}
}

package feishu

import (
	"encoding/json"
	"strings"
	"testing"

	core "github.com/chenhg5/cc-connect/core"
)

func TestBuildProgressCard_ShardsLargeToolResult(t *testing.T) {
	big := strings.Repeat("B", 25_000)
	ok := true
	payload := &core.ProgressCardPayload{
		Agent: "CC", Lang: "en", State: core.ProgressCardStateCompleted,
		Items: []core.ProgressCardEntry{
			{Kind: core.ProgressEntryToolUse, Tool: "Bash", Text: `{"command":"cat big"}`, ID: "bsh_z", Status: "running"},
			{Kind: core.ProgressEntryToolResult, Tool: "Bash", Text: big, ID: "bsh_z", Status: "ok", Success: &ok},
		},
	}
	cards := buildProgressCardJSONs(payload)
	panels := 0
	partLabels := 0
	for _, raw := range cards {
		var card map[string]any
		_ = json.Unmarshal([]byte(raw), &card)
		body := card["body"].(map[string]any)
		els := body["elements"].([]any)
		for _, e := range els {
			m, ok := e.(map[string]any)
			if !ok || m["tag"] != "collapsible_panel" {
				continue
			}
			panels++
			hdr := m["header"].(map[string]any)
			title := hdr["title"].(map[string]any)["content"].(string)
			if strings.Contains(title, "/") && strings.Contains(title, "(") {
				partLabels++
			}
		}
	}
	if panels < 2 {
		t.Errorf("expected >=2 panels (sharded), got %d; cards=%v", panels, cards)
	}
	if partLabels == 0 {
		t.Errorf("expected part labels like (1/N) in titles; cards=%v", cards)
	}
}

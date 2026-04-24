package feishu

import (
	"encoding/json"
	"strings"
	"testing"

	core "github.com/chenhg5/cc-connect/core"
)

func TestBuildProgressCard_AssemblesCollapsiblePanels(t *testing.T) {
	ok := true
	exit := 0
	payload := &core.ProgressCardPayload{
		Agent: "CC", Lang: "zh-CN", State: core.ProgressCardStateCompleted,
		Items: []core.ProgressCardEntry{
			{Kind: core.ProgressEntryThinking, Text: "Plan the work", ID: "thk_1"},
			{Kind: core.ProgressEntryToolUse, Tool: "Bash", Text: `{"command":"ls"}`, ID: "bsh_1", Status: "running"},
			{Kind: core.ProgressEntryToolResult, Tool: "Bash", Text: "file1", ID: "bsh_1", Status: "ok", ExitCode: &exit, Success: &ok, DurationMs: 12},
		},
	}
	raw := buildProgressCardJSONFromPayload(payload)

	var card map[string]any
	if err := json.Unmarshal([]byte(raw), &card); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	body := card["body"].(map[string]any)
	els := body["elements"].([]any)

	panels := 0
	for _, el := range els {
		if m, ok := el.(map[string]any); ok && m["tag"] == "collapsible_panel" {
			panels++
		}
	}
	if panels != 2 {
		t.Errorf("want 2 panels (thinking + merged tool), got %d; card=%s", panels, raw)
	}

	if !strings.Contains(raw, `"update_multi":true`) {
		t.Error("still missing update_multi")
	}
}

func TestBuildProgressCard_RunningPanelExpanded(t *testing.T) {
	payload := &core.ProgressCardPayload{
		Agent: "CC", Lang: "en", State: core.ProgressCardStateRunning,
		Items: []core.ProgressCardEntry{
			{Kind: core.ProgressEntryToolUse, Tool: "Bash", Text: `{"command":"sleep 10"}`, ID: "bsh_1", Status: "running"},
		},
	}
	raw := buildProgressCardJSONFromPayload(payload)
	if !strings.Contains(raw, `"expanded":true`) {
		t.Errorf("the single running panel must be expanded: %s", raw)
	}
}

func TestBuildProgressCard_OnlyLastRunningExpanded(t *testing.T) {
	payload := &core.ProgressCardPayload{
		Agent: "CC", Lang: "en", State: core.ProgressCardStateRunning,
		Items: []core.ProgressCardEntry{
			{Kind: core.ProgressEntryToolUse, Tool: "Bash", Text: `{"command":"a"}`, ID: "bsh_1", Status: "running"},
			{Kind: core.ProgressEntryToolUse, Tool: "Bash", Text: `{"command":"b"}`, ID: "bsh_2", Status: "running"},
		},
	}
	raw := buildProgressCardJSONFromPayload(payload)
	got := strings.Count(raw, `"expanded":true`)
	if got != 1 {
		t.Errorf("want exactly 1 expanded panel, got %d: %s", got, raw)
	}
}

package feishu

import (
	"encoding/json"
	"strings"
	"testing"

	core "github.com/chenhg5/cc-connect/core"
)

func TestBuildToolPanel_MergesUseAndResult(t *testing.T) {
	ok := true
	exit := 0
	use := core.ProgressCardEntry{
		Kind: core.ProgressEntryToolUse,
		Tool: "Bash",
		Text: `{"command":"ls -la /tmp"}`,
		ID:   "bsh_1",
		Status: "running",
	}
	res := core.ProgressCardEntry{
		Kind:       core.ProgressEntryToolResult,
		Tool:       "Bash",
		Text:       "file1\nfile2",
		ID:         "bsh_1",
		Status:     "ok",
		ExitCode:   &exit,
		Success:    &ok,
		DurationMs: 42,
	}

	panel := buildToolPanel(use, &res, "zh-CN", false)
	b, err := json.Marshal(panel)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)

	if !strings.Contains(s, `"tag":"collapsible_panel"`) {
		t.Errorf("not a collapsible_panel: %s", s)
	}
	if !strings.Contains(s, `"expanded":false`) {
		t.Errorf("completed panel should default collapsed: %s", s)
	}
	if !strings.Contains(s, `"element_id":"bsh_1"`) {
		t.Errorf("missing element_id: %s", s)
	}
	if !strings.Contains(s, "✅") || !strings.Contains(s, "执行命令") {
		t.Errorf("title missing status/label: %s", s)
	}
	if !strings.Contains(s, "ls -la /tmp") {
		t.Errorf("title missing digest: %s", s)
	}
	if !strings.Contains(s, "42ms") {
		t.Errorf("title missing duration: %s", s)
	}
	if !strings.Contains(s, "📋") || !strings.Contains(s, "📄") {
		t.Errorf("missing 📋/📄 section headers: %s", s)
	}
	if !strings.Contains(s, "```bash") {
		t.Errorf("command code fence not bash: %s", s)
	}
	if !strings.Contains(s, "file1") || !strings.Contains(s, "file2") {
		t.Errorf("result body missing: %s", s)
	}
}

func TestBuildToolPanel_ExpandedWhenRunning(t *testing.T) {
	use := core.ProgressCardEntry{
		Kind: core.ProgressEntryToolUse, Tool: "Bash",
		Text: `{"command":"sleep 10"}`, ID: "bsh_2", Status: "running",
	}
	panel := buildToolPanel(use, nil, "en", true)
	b, _ := json.Marshal(panel)
	if !strings.Contains(string(b), `"expanded":true`) {
		t.Errorf("last-running panel should be expanded: %s", b)
	}
	if !strings.Contains(string(b), "⏳") {
		t.Errorf("running panel missing hourglass")
	}
}

func TestBuildToolPanel_ExpandedOnFailure(t *testing.T) {
	bad := false
	use := core.ProgressCardEntry{
		Kind: core.ProgressEntryToolUse, Tool: "Bash",
		Text: `{"command":"false"}`, ID: "bsh_3",
	}
	exit := 1
	res := core.ProgressCardEntry{
		Kind: core.ProgressEntryToolResult, Tool: "Bash",
		Text: "error: something failed",
		ID: "bsh_3", Status: "fail",
		Success: &bad, ExitCode: &exit,
	}
	panel := buildToolPanel(use, &res, "en", false)
	b, _ := json.Marshal(panel)
	if !strings.Contains(string(b), `"expanded":true`) {
		t.Errorf("failed panel must stay expanded: %s", b)
	}
	if !strings.Contains(string(b), "❌") {
		t.Errorf("missing fail emoji: %s", b)
	}
}

func TestBuildToolPanel_PartSuffixWhenSharded(t *testing.T) {
	use := core.ProgressCardEntry{
		Kind: core.ProgressEntryToolUse, Tool: "Bash",
		Text: `{"command":"cat huge"}`, ID: "bsh_4",
		PartIdx: 2, PartTotal: 3,
	}
	panel := buildToolPanel(use, nil, "zh", false)
	b, _ := json.Marshal(panel)
	if !strings.Contains(string(b), "(2/3)") {
		t.Errorf("missing part suffix 2/3: %s", b)
	}
}

func TestBuildThinkingPanel_CollapsedByDefault(t *testing.T) {
	entry := core.ProgressCardEntry{
		Kind: core.ProgressEntryThinking,
		Text: "Analyzing dependencies and whether to split into multiple PRs...",
		ID:   "thk_7",
	}
	panel := buildThinkingPanel(entry, "zh-CN", false)
	b, _ := json.Marshal(panel)
	s := string(b)
	if !strings.Contains(s, `"tag":"collapsible_panel"`) {
		t.Error("not collapsible_panel")
	}
	if !strings.Contains(s, `"expanded":false`) {
		t.Error("should default collapsed")
	}
	if !strings.Contains(s, "💭") {
		t.Error("missing thought bubble")
	}
	if !strings.Contains(s, "思考") {
		t.Error("missing zh label")
	}
	if !strings.Contains(s, "Analyzing") {
		t.Error("digest missing text")
	}
	if !strings.Contains(s, `"element_id":"thk_7"`) {
		t.Error("missing element_id")
	}
}

func TestBuildThinkingPanel_ExpandedWhenLatest(t *testing.T) {
	entry := core.ProgressCardEntry{
		Kind: core.ProgressEntryThinking, Text: "Latest thought", ID: "thk_8",
	}
	panel := buildThinkingPanel(entry, "en", true)
	b, _ := json.Marshal(panel)
	if !strings.Contains(string(b), `"expanded":true`) {
		t.Error("latest thinking panel should be expanded")
	}
}

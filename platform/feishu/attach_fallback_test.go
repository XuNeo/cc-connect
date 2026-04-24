package feishu

import (
	"encoding/json"
	"strings"
	"testing"

	core "github.com/chenhg5/cc-connect/core"
)

func TestAttachFallback_PanelShowsHeadTailAndNotice(t *testing.T) {
	body := strings.Repeat("H", 600) + "----MIDDLE----" + strings.Repeat("T", 600)
	e := core.ProgressCardEntry{
		Kind: core.ProgressEntryToolResult, Tool: "Bash",
		Text: body, ID: "bsh_X", Status: "attach",
	}
	use := core.ProgressCardEntry{Kind: core.ProgressEntryToolUse, Tool: "Bash", Text: `{"command":"cat huge"}`, ID: "bsh_X"}
	panel := buildToolPanel(use, &e, "zh-CN", false)
	b, _ := json.Marshal(panel)
	s := string(b)

	if !strings.Contains(s, "附件") {
		t.Errorf("missing attachment notice: %s", s)
	}
	if strings.Contains(s, "----MIDDLE----") {
		t.Errorf("middle content should be omitted in panel: %s", s)
	}
	if !strings.Contains(s, "HHHH") || !strings.Contains(s, "TTTT") {
		t.Errorf("head/tail should be preserved: %s", s)
	}
}

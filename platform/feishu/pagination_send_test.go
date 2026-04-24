package feishu

import (
	"strings"
	"testing"

	core "github.com/chenhg5/cc-connect/core"
)

func makePanelID(prefix string, seq int) string {
	return makeElementID(prefix, seq)
}

func TestBuildProgressCardJSONs_PaginatesLargePayload(t *testing.T) {
	items := make([]core.ProgressCardEntry, 400)
	for i := range items {
		items[i] = core.ProgressCardEntry{
			Kind: core.ProgressEntryToolUse, Tool: "Bash",
			Text: `{"command":"echo x"}`,
			ID:   makePanelID("bash", i+1),
		}
	}
	payload := &core.ProgressCardPayload{
		Agent: "CC", Lang: "en", State: core.ProgressCardStateRunning, Items: items,
	}
	cards := buildProgressCardJSONs(payload)
	if len(cards) < 3 {
		t.Errorf("expected multiple cards for 400 items, got %d", len(cards))
	}
	for i, c := range cards {
		if !strings.Contains(c, `"update_multi":true`) {
			t.Errorf("card %d missing update_multi", i)
		}
	}
}

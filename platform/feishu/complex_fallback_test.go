package feishu

import (
	"encoding/json"
	"strings"
	"testing"

	core "github.com/chenhg5/cc-connect/core"
)

func TestBuildProgressCardJSONs_HonorsBudgetOverride(t *testing.T) {
	items := make([]core.ProgressCardEntry, 250)
	for i := range items {
		items[i] = core.ProgressCardEntry{
			Kind: core.ProgressEntryToolUse, Tool: "Bash",
			Text: `{"command":"echo x"}`, ID: makePanelID("bash", i+1),
		}
	}
	payload := &core.ProgressCardPayload{Agent: "CC", Lang: "en", State: core.ProgressCardStateRunning, Items: items}
	cardsWide := buildProgressCardJSONsWithBudget(payload, 150, 1_000_000)
	cardsTight := buildProgressCardJSONsWithBudget(payload, 80, 1_000_000)
	if len(cardsTight) <= len(cardsWide) {
		t.Errorf("tighter budget should produce more cards: wide=%d tight=%d", len(cardsWide), len(cardsTight))
	}
	for _, c := range cardsTight {
		var card map[string]any
		_ = json.Unmarshal([]byte(c), &card)
		body := card["body"].(map[string]any)
		els := body["elements"].([]any)
		if len(els) > 81 {
			t.Errorf("page has %d elements, > tighter budget 80+1", len(els))
		}
	}
	if !strings.Contains(cardsTight[0], `"update_multi":true`) {
		t.Error("update_multi lost under tighter budget")
	}
}

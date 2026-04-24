package feishu

import (
	"strings"
	"testing"

	core "github.com/chenhg5/cc-connect/core"
)

// Regression: uploadAttachmentsFromPayload previously filtered on
// Status=="attach" but the shardLargeEntry logic only sets that flag on a
// local copy inside buildProgressCardElements, leaving payload.Items
// untouched. The uploader therefore never uploaded. selectAttablePayload
// uses rune count directly so renderer and uploader stay in sync.
func TestSelectAttachablePayloadEntries_LargeToolResultIsSelected(t *testing.T) {
	big := strings.Repeat("B", 30_000)
	small := "ok"
	payload := &core.ProgressCardPayload{Items: []core.ProgressCardEntry{
		{Kind: core.ProgressEntryToolUse, Tool: "Bash", Text: `{"command":"x"}`, ID: "bsh_1"},
		{Kind: core.ProgressEntryToolResult, Tool: "Bash", Text: small, ID: "bsh_1"},
		{Kind: core.ProgressEntryToolUse, Tool: "Bash", Text: `{"command":"y"}`, ID: "bsh_2"},
		{Kind: core.ProgressEntryToolResult, Tool: "Bash", Text: big, ID: "bsh_2"},
	}}
	got := selectAttachablePayloadEntries(payload, 28_000)
	if len(got) != 1 {
		t.Fatalf("want 1 attachable entry, got %d", len(got))
	}
	if got[0].ID != "bsh_2" {
		t.Errorf("selected wrong entry ID=%q", got[0].ID)
	}
	if got[0].Text != big {
		t.Errorf("selected entry text was mutated")
	}
}

func TestSelectAttachablePayloadEntries_SmallIgnored(t *testing.T) {
	payload := &core.ProgressCardPayload{Items: []core.ProgressCardEntry{
		{Kind: core.ProgressEntryToolResult, Tool: "Bash", Text: strings.Repeat("x", 27_999), ID: "a"},
		{Kind: core.ProgressEntryToolResult, Tool: "Bash", Text: strings.Repeat("x", 28_000), ID: "b"},
	}}
	if got := selectAttachablePayloadEntries(payload, 28_000); len(got) != 0 {
		t.Errorf("want none selected at or below threshold, got %d", len(got))
	}
}

func TestSelectAttachablePayloadEntries_NilPayload(t *testing.T) {
	if got := selectAttachablePayloadEntries(nil, 28_000); got != nil {
		t.Errorf("nil payload should return nil, got %v", got)
	}
}

func TestSelectAttachablePayloadEntries_IgnoresNonToolResultKinds(t *testing.T) {
	big := strings.Repeat("X", 30_000)
	payload := &core.ProgressCardPayload{Items: []core.ProgressCardEntry{
		{Kind: core.ProgressEntryThinking, Text: big, ID: "t1"},
		{Kind: core.ProgressEntryInfo, Text: big, ID: "i1"},
	}}
	if got := selectAttachablePayloadEntries(payload, 28_000); len(got) != 0 {
		t.Errorf("non-tool_result kinds must not be uploaded as attachments; got %d", len(got))
	}
}

package core

import (
	"testing"
	"time"
)

func TestAssignEntryIDAndDuration_Pair(t *testing.T) {
	tracker := newToolPanelTracker()

	useEntry := ProgressCardEntry{Kind: ProgressEntryToolUse, Tool: "Bash", Text: "ls"}
	tracker.onToolUse("toolcall_abc", &useEntry, time.Unix(1000, 0))
	if useEntry.ID == "" {
		t.Fatal("tool_use entry must get an ID")
	}
	if useEntry.Status != "running" {
		t.Errorf("tool_use status = %q, want running", useEntry.Status)
	}

	resEntry := ProgressCardEntry{Kind: ProgressEntryToolResult, Tool: "Bash", Text: "file1\n"}
	ok := true
	resEntry.Success = &ok
	tracker.onToolResult("toolcall_abc", &resEntry, time.Unix(1000, 500_000_000))

	if resEntry.ID != useEntry.ID {
		t.Errorf("tool_result ID %q should match tool_use ID %q", resEntry.ID, useEntry.ID)
	}
	if resEntry.Status != "ok" {
		t.Errorf("tool_result status = %q, want ok", resEntry.Status)
	}
	if resEntry.DurationMs != 500 {
		t.Errorf("duration = %d, want 500", resEntry.DurationMs)
	}
}

func TestAssignEntryIDAndDuration_FailureStatus(t *testing.T) {
	tracker := newToolPanelTracker()
	use := ProgressCardEntry{Kind: ProgressEntryToolUse, Tool: "Bash"}
	tracker.onToolUse("x", &use, time.Unix(100, 0))
	bad := false
	res := ProgressCardEntry{Kind: ProgressEntryToolResult, Tool: "Bash", Success: &bad}
	tracker.onToolResult("x", &res, time.Unix(100, 10_000_000))
	if res.Status != "fail" {
		t.Errorf("status = %q, want fail", res.Status)
	}
}

func TestAssignEntryIDAndDuration_OrphanResult(t *testing.T) {
	tracker := newToolPanelTracker()
	res := ProgressCardEntry{Kind: ProgressEntryToolResult, Tool: "Bash"}
	tracker.onToolResult("missing-id", &res, time.Unix(100, 0))
	if res.ID == "" {
		t.Error("orphan result should still get an ID to render")
	}
}

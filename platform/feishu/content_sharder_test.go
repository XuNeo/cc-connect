package feishu

import (
	"strings"
	"testing"

	core "github.com/chenhg5/cc-connect/core"
)

func TestShardLargeEntry_NoShardWhenSmall(t *testing.T) {
	e := core.ProgressCardEntry{Kind: core.ProgressEntryToolResult, Tool: "Bash", Text: "hello"}
	out := shardLargeEntry(e, 18_000, 28_000)
	if len(out) != 1 {
		t.Errorf("want 1 shard for small body, got %d", len(out))
	}
	if out[0].PartTotal != 0 {
		t.Errorf("single-shard should not mark PartTotal")
	}
}

func TestShardLargeEntry_SplitsMidRange(t *testing.T) {
	text := strings.Repeat("a", 25_000)
	e := core.ProgressCardEntry{Kind: core.ProgressEntryToolResult, Tool: "Bash", Text: text, ID: "bsh_9"}
	out := shardLargeEntry(e, 10_000, 28_000)
	if len(out) < 3 {
		t.Errorf("expected >=3 shards, got %d", len(out))
	}
	total := 0
	for i, s := range out {
		if s.PartIdx != i+1 {
			t.Errorf("shard %d has PartIdx=%d", i, s.PartIdx)
		}
		if s.PartTotal != len(out) {
			t.Errorf("shard %d has PartTotal=%d want %d", i, s.PartTotal, len(out))
		}
		if s.ID != "bsh_9" {
			t.Errorf("shard %d has ID=%q want bsh_9", i, s.ID)
		}
		total += len(s.Text)
	}
	if total != len(text) {
		t.Errorf("data loss: reassembled %d chars vs original %d", total, len(text))
	}
}

func TestShardLargeEntry_FlagsHugeEntryForAttachment(t *testing.T) {
	text := strings.Repeat("x", 40_000)
	e := core.ProgressCardEntry{Kind: core.ProgressEntryToolResult, Tool: "Bash", Text: text, ID: "bsh_10"}
	out := shardLargeEntry(e, 10_000, 28_000)
	if len(out) != 1 {
		t.Errorf("attachment path should return 1 entry, got %d", len(out))
	}
	if out[0].Status != "attach" {
		t.Errorf("status = %q, want attach", out[0].Status)
	}
	if out[0].Text != text {
		t.Error("original body must be preserved for the attachment uploader")
	}
}

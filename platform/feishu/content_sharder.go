package feishu

import (
	"unicode/utf8"

	core "github.com/chenhg5/cc-connect/core"
)

// shardLargeEntry decides how to carry a single entry's body within Feishu
// size limits:
//   - <= shardInline:                  one shard, unchanged
//   - shardInline < body <= attachAt:  N shards marked with PartIdx/PartTotal
//   - body > attachAt:                 one entry with Status="attach" so the
//                                      caller uploads the full body as a file
// Data is NEVER discarded.
func shardLargeEntry(entry core.ProgressCardEntry, shardInline, attachAt int) []core.ProgressCardEntry {
	n := utf8.RuneCountInString(entry.Text)
	if n <= shardInline {
		return []core.ProgressCardEntry{entry}
	}
	if n > attachAt {
		e := entry
		e.Status = "attach"
		return []core.ProgressCardEntry{e}
	}
	parts := (n + shardInline - 1) / shardInline
	chunkRunes := (n + parts - 1) / parts
	out := make([]core.ProgressCardEntry, 0, parts)
	runeStart := 0
	for i := 0; i < parts; i++ {
		end := runeStart + chunkRunes
		if end > n {
			end = n
		}
		slice := runeSlice(entry.Text, runeStart, end)
		e := entry
		e.Text = slice
		e.PartIdx = i + 1
		e.PartTotal = parts
		out = append(out, e)
		runeStart = end
	}
	return out
}

func runeSlice(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	i := 0
	var bStart, bEnd int
	bEnd = len(s)
	for pos := range s {
		if i == start {
			bStart = pos
		}
		if i == end {
			bEnd = pos
			break
		}
		i++
	}
	return s[bStart:bEnd]
}

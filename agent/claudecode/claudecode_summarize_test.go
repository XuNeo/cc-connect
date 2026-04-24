package claudecode

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSummarizeInput_NewToolsPreserveFields pins the contract that the tools
// added to Claude Code after the original implementation (WebFetch,
// WebSearch, Task, Skill, NotebookEdit, TodoWrite) still round-trip their
// semantic fields through summarizeInput so the Feishu tool_digest renderer
// can extract URLs, queries, descriptions, etc.
func TestSummarizeInput_NewToolsPreserveFields(t *testing.T) {
	cases := []struct {
		tool string
		in   string
		keys []string // substrings that must appear in the output
	}{
		{"WebFetch", `{"url":"https://a.com/x","prompt":"summarize"}`, []string{"https://a.com/x", "summarize"}},
		{"WebSearch", `{"query":"feishu limits"}`, []string{"feishu limits"}},
		{"Task", `{"description":"audit","prompt":"...","subagent_type":"general"}`, []string{"audit", "general"}},
		{"TodoWrite", `{"todos":[{"content":"a","status":"pending","activeForm":"A"}]}`, []string{"pending", "activeForm"}},
		{"NotebookEdit", `{"notebook_path":"/tmp/x.ipynb","new_source":"print(1)"}`, []string{"/tmp/x.ipynb", "print(1)"}},
		{"Skill", `{"skill":"brainstorming"}`, []string{"brainstorming"}},
	}
	for _, c := range cases {
		var m map[string]any
		if err := json.Unmarshal([]byte(c.in), &m); err != nil {
			t.Fatalf("%s: fixture unmarshal: %v", c.tool, err)
		}
		got := summarizeInput(c.tool, m)
		for _, k := range c.keys {
			if !strings.Contains(got, k) {
				t.Errorf("%s: summarizeInput missing %q; got %q", c.tool, k, got)
			}
		}
	}
}

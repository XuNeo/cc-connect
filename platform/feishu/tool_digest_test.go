package feishu

import (
	"strings"
	"testing"
)

func TestToolDigest_Bash(t *testing.T) {
	got := toolDigest("Bash", `{"command":"ls -la /tmp","description":"list tmp"}`)
	if got != "ls -la /tmp" {
		t.Errorf("Bash digest = %q, want %q", got, "ls -la /tmp")
	}
}

func TestToolDigest_Read(t *testing.T) {
	got := toolDigest("Read", `{"file_path":"/home/neo/foo/bar.go"}`)
	if got != "/home/neo/foo/bar.go" {
		t.Errorf("Read digest = %q", got)
	}
}

func TestToolDigest_WebFetch(t *testing.T) {
	got := toolDigest("WebFetch", `{"url":"https://example.com/a","prompt":"summarize"}`)
	if got != "https://example.com/a" {
		t.Errorf("WebFetch digest = %q", got)
	}
}

func TestToolDigest_WebSearch(t *testing.T) {
	got := toolDigest("WebSearch", `{"query":"feishu rate limit"}`)
	if got != "feishu rate limit" {
		t.Errorf("WebSearch digest = %q", got)
	}
}

func TestToolDigest_Task(t *testing.T) {
	got := toolDigest("Task",
		`{"description":"Audit branch","prompt":"Check uncommitted changes...","subagent_type":"general"}`)
	if got != "Audit branch" {
		t.Errorf("Task digest = %q", got)
	}
}

func TestToolDigest_TodoWrite(t *testing.T) {
	in := `{"todos":[{"content":"Write plan","status":"in_progress","activeForm":"Writing plan"},{"content":"Review","status":"pending","activeForm":"Reviewing"}]}`
	got := toolDigest("TodoWrite", in)
	if !strings.Contains(got, "1/2") && !strings.Contains(got, "1 / 2") {
		t.Errorf("TodoWrite digest should mention 1/2 done, got %q", got)
	}
}

func TestToolDigest_Grep(t *testing.T) {
	got := toolDigest("Grep", `{"pattern":"func New","path":"core/","output_mode":"files_with_matches"}`)
	if !strings.Contains(got, "func New") {
		t.Errorf("Grep digest missing pattern: %q", got)
	}
}

func TestToolDigest_UnknownFallback(t *testing.T) {
	got := toolDigest("NewTool", `{"foo":"bar baz"}`)
	if got == "" {
		t.Error("want non-empty digest fallback")
	}
	if len(got) > 60 {
		t.Errorf("digest length %d > 60", len(got))
	}
}

func TestToolDigest_TruncatesLongInput(t *testing.T) {
	long := strings.Repeat("a", 200)
	got := toolDigest("Bash", `{"command":"`+long+`"}`)
	if len([]rune(got)) > 60 {
		t.Errorf("digest length %d > 60: %q", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("digest should end with ellipsis: %q", got)
	}
}

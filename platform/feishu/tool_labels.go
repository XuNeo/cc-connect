package feishu

import (
	"fmt"
	"strings"
)

// toolLabel maps an upstream tool name (as emitted by Claude Code) to a
// user-facing label in the requested language. Unknown tools pass through
// unchanged so new Claude Code tools degrade gracefully.
func toolLabel(name, lang string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	if !isZhLikeProgressLang(lang) {
		switch key {
		case "shell", "run_shell_command":
			return "Bash"
		}
		return strings.TrimSpace(name)
	}
	switch key {
	case "bash", "shell", "run_shell_command":
		return "执行命令"
	case "read":
		return "读文件"
	case "write":
		return "写文件"
	case "edit":
		return "编辑文件"
	case "grep":
		return "搜索内容"
	case "glob":
		return "查找文件"
	case "webfetch":
		return "抓取网页"
	case "websearch":
		return "网络搜索"
	case "todowrite":
		return "任务清单"
	case "task", "agent":
		return "子任务"
	case "skill":
		return "技能"
	case "notebookedit":
		return "编辑笔记本"
	}
	return strings.TrimSpace(name)
}

// panelStatusEmoji returns the emoji shown at the head of a panel title.
func panelStatusEmoji(status string) string {
	switch strings.ToLower(status) {
	case "running", "pending":
		return "⏳"
	case "ok", "success", "completed":
		return "✅"
	case "fail", "failed", "error":
		return "❌"
	case "aborted", "cancelled", "canceled":
		return "⏸"
	}
	return "•"
}

// buildPanelTitle assembles the summary line shown in the panel header:
//
//	[🤖 ]{emoji} {label} · {digest} · {duration}
//
// The 🤖 prefix is drawn when isSubagent is true, meaning either the
// tool was emitted from inside a subagent (claudecode: parent_tool_use_id
// non-empty on the source event) or the tool is itself a subagent-entry
// tool (see isSubagentEntryTool). Duration is omitted when <=0.
func buildPanelTitle(status, toolName, digest string, durationMs int, lang string, isSubagent bool) string {
	label := toolLabel(toolName, lang)
	var b strings.Builder
	if isSubagent {
		b.WriteString("🤖 ")
	}
	b.WriteString(panelStatusEmoji(status))
	b.WriteByte(' ')
	b.WriteString(label)
	if digest = strings.TrimSpace(digest); digest != "" {
		b.WriteString(" · ")
		b.WriteString(digest)
	}
	if durationMs > 0 {
		b.WriteString(" · ")
		b.WriteString(formatDuration(durationMs))
	}
	return b.String()
}

// isSubagentEntryTool reports whether toolName is the name of a tool that
// starts or represents a subagent invocation — i.e. a tool that deserves
// the 🤖 marker even when it's called directly by the main agent.
//
// Matches case-insensitively:
//   - "Agent" (claudecode's internal name for the Task tool, observed in
//     stream-json output at /tmp/cctest/test_subagent.py)
//   - "Task"  (Anthropic user-facing name; future-proofing in case the
//     CLI switches)
//   - "task"  (opencode's subagent entrypoint; lowercase in its NDJSON)
func isSubagentEntryTool(toolName string) bool {
	return strings.EqualFold(toolName, "task") || strings.EqualFold(toolName, "agent")
}

func formatDuration(ms int) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms < 60_000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	return fmt.Sprintf("%dm%ds", ms/60_000, (ms%60_000)/1000)
}

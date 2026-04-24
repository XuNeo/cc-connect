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
	case "task":
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
//	{emoji} {label} · {digest} · {duration}
//
// Duration is omitted when <=0.
func buildPanelTitle(status, toolName, digest string, durationMs int, lang string) string {
	label := toolLabel(toolName, lang)
	var b strings.Builder
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

func formatDuration(ms int) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms < 60_000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	return fmt.Sprintf("%dm%ds", ms/60_000, (ms%60_000)/1000)
}

package feishu

import (
	"strings"
	"testing"
)

func TestToolLabel_EnglishFallback(t *testing.T) {
	if got := toolLabel("Bash", "en"); got != "Bash" {
		t.Errorf("en Bash = %q, want Bash", got)
	}
	if got := toolLabel("WebFetch", "en"); got != "WebFetch" {
		t.Errorf("en WebFetch = %q, want WebFetch", got)
	}
}

func TestToolLabel_Chinese(t *testing.T) {
	cases := map[string]string{
		"Bash":         "执行命令",
		"shell":        "执行命令",
		"Read":         "读文件",
		"Write":        "写文件",
		"Edit":         "编辑文件",
		"Grep":         "搜索内容",
		"Glob":         "查找文件",
		"WebFetch":     "抓取网页",
		"WebSearch":    "网络搜索",
		"TodoWrite":    "任务清单",
		"Task":         "子任务",
		"Skill":        "技能",
		"NotebookEdit": "编辑笔记本",
	}
	for in, want := range cases {
		if got := toolLabel(in, "zh-CN"); got != want {
			t.Errorf("zh %s = %q, want %q", in, got, want)
		}
	}
}

func TestToolLabel_UnknownPassthrough(t *testing.T) {
	if got := toolLabel("SomeNewTool", "zh"); got != "SomeNewTool" {
		t.Errorf("unknown = %q, want passthrough SomeNewTool", got)
	}
}

func TestPanelStatusEmoji(t *testing.T) {
	if panelStatusEmoji("running") != "⏳" {
		t.Error("running != hourglass")
	}
	if panelStatusEmoji("ok") != "✅" {
		t.Error("ok != check")
	}
	if panelStatusEmoji("fail") != "❌" {
		t.Error("fail != cross")
	}
	if panelStatusEmoji("aborted") != "⏸" {
		t.Error("aborted != pause")
	}
}

func TestBuildPanelTitle_AllParts(t *testing.T) {
	got := buildPanelTitle("ok", "Bash", "ls -la /tmp", 42, "zh")
	if !strings.HasPrefix(got, "✅") {
		t.Errorf("missing emoji prefix: %q", got)
	}
	if !strings.Contains(got, "执行命令") {
		t.Errorf("missing zh label: %q", got)
	}
	if !strings.Contains(got, "ls -la") {
		t.Errorf("missing digest: %q", got)
	}
	if !strings.Contains(got, "42ms") {
		t.Errorf("missing duration: %q", got)
	}
}

func TestBuildPanelTitle_OmitsZeroDuration(t *testing.T) {
	got := buildPanelTitle("running", "Read", "/etc/passwd", 0, "en")
	if strings.Contains(got, "0ms") {
		t.Errorf("should omit 0 duration, got %q", got)
	}
}

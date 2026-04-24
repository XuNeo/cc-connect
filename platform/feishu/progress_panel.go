package feishu

import (
	"fmt"

	core "github.com/chenhg5/cc-connect/core"
)

// buildToolPanel returns a collapsible_panel element that renders one tool
// invocation (tool_use merged with its matching tool_result when present).
// isLastRunning selects the only in-progress panel that stays expanded.
func buildToolPanel(use core.ProgressCardEntry, res *core.ProgressCardEntry, lang string, isLastRunning bool) map[string]any {
	status := use.Status
	if res != nil {
		status = res.Status
	}
	if status == "" {
		status = "running"
	}

	digest := toolDigest(use.Tool, use.Text)
	title := buildPanelTitle(status, use.Tool, digest, durationFrom(res), lang)
	if use.PartTotal > 0 {
		title += fmt.Sprintf(" (%d/%d)", use.PartIdx, use.PartTotal)
	}

	expanded := false
	switch status {
	case "running":
		expanded = isLastRunning
	case "fail", "failed", "error":
		expanded = true
	}

	elementID := sanitizeIDPrefix(use.ID)
	if elementID == "" {
		elementID = "p"
	}
	if len(elementID) > 20 {
		elementID = elementID[:20]
	}

	cmdBody := formatProgressToolInput(use.Tool, use.Text)
	if cmdBody == "" {
		cmdBody = "`" + inlineCodeText(use.Text) + "`"
	}

	elements := []map[string]any{
		{"tag": "markdown", "content": "**" + sectionLabel("command", lang) + "**"},
		{"tag": "markdown", "content": cmdBody},
	}

	if res != nil {
		dot := progressResultDot(*res)
		meta := dot
		if res.ExitCode != nil {
			meta += fmt.Sprintf(" exit `%d`", *res.ExitCode)
		}
		if res.DurationMs > 0 {
			meta += " · " + formatDuration(res.DurationMs)
		}
		elements = append(elements, map[string]any{
			"tag":     "markdown",
			"content": "**" + sectionLabel("result", lang) + "** " + meta,
		})
		if body := formatProgressToolResult(res.Text); body != "" {
			elements = append(elements, map[string]any{"tag": "markdown", "content": body})
		} else {
			elements = append(elements, map[string]any{
				"tag": "markdown", "content": "_" + progressNoOutputText(lang) + "_",
			})
		}
	} else {
		elements = append(elements, map[string]any{
			"tag": "markdown", "content": "**" + sectionLabel("result", lang) + "** _" + runningPlaceholderText(lang) + "_",
		})
	}

	return map[string]any{
		"tag":        "collapsible_panel",
		"expanded":   expanded,
		"element_id": elementID,
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": title,
			},
			"icon": map[string]any{
				"tag":   "standard_icon",
				"token": "down-small-ccm_outlined",
			},
			"icon_position":       "right",
			"icon_expanded_angle": -180,
		},
		"elements": elements,
	}
}

func durationFrom(res *core.ProgressCardEntry) int {
	if res == nil {
		return 0
	}
	return res.DurationMs
}

func sectionLabel(section, lang string) string {
	zh := isZhLikeProgressLang(lang)
	switch section {
	case "command":
		if zh {
			return "📋 命令"
		}
		return "📋 Command"
	case "result":
		if zh {
			return "📄 结果"
		}
		return "📄 Result"
	}
	return section
}

func runningPlaceholderText(lang string) string {
	if isZhLikeProgressLang(lang) {
		return "执行中…"
	}
	return "running…"
}

// buildThinkingPanel renders a reasoning/thinking block as a collapsible_panel.
// isLatest picks the single panel (usually the most recent) that stays
// expanded so the user can follow the in-flight reasoning.
func buildThinkingPanel(entry core.ProgressCardEntry, lang string, isLatest bool) map[string]any {
	label := "Thinking"
	if isZhLikeProgressLang(lang) {
		label = "思考"
	}
	digest := shrinkDigest(firstLine(entry.Text), toolDigestMaxRunes)
	title := "💭 " + label
	if digest != "" {
		title += " · " + digest
	}
	if entry.PartTotal > 0 {
		title += fmt.Sprintf(" (%d/%d)", entry.PartIdx, entry.PartTotal)
	}

	elementID := sanitizeIDPrefix(entry.ID)
	if elementID == "" {
		elementID = "t"
	}
	if len(elementID) > 20 {
		elementID = elementID[:20]
	}

	body := entry.Text
	if body == "" {
		body = "_empty_"
	} else {
		body = "```\n" + body + "\n```"
	}

	return map[string]any{
		"tag":        "collapsible_panel",
		"expanded":   isLatest,
		"element_id": elementID,
		"header": map[string]any{
			"title": map[string]any{"tag": "plain_text", "content": title},
			"icon": map[string]any{
				"tag":   "standard_icon",
				"token": "down-small-ccm_outlined",
			},
			"icon_position":       "right",
			"icon_expanded_angle": -180,
		},
		"elements": []map[string]any{
			{"tag": "markdown", "content": body},
		},
	}
}

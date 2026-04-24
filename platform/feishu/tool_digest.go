package feishu

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const toolDigestMaxRunes = 60

// toolDigest extracts a short summary of a tool_use input suitable for the
// collapsible_panel header. For well-known tools it pulls the semantic field
// (command, file_path, url, query, pattern, description...). For unknown
// tools it falls back to the first ~60 chars of the raw input's first line.
func toolDigest(toolName, rawInput string) string {
	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		return ""
	}
	key := strings.ToLower(strings.TrimSpace(toolName))
	var raw string
	switch key {
	case "bash", "shell", "run_shell_command":
		raw = jsonField(rawInput, "command")
	case "read", "write":
		raw = jsonField(rawInput, "file_path")
	case "edit":
		raw = jsonField(rawInput, "file_path")
	case "grep":
		p := jsonField(rawInput, "pattern")
		if path := jsonField(rawInput, "path"); path != "" {
			raw = p + " in " + path
		} else {
			raw = p
		}
	case "glob":
		raw = jsonField(rawInput, "pattern")
	case "webfetch":
		raw = jsonField(rawInput, "url")
	case "websearch":
		raw = jsonField(rawInput, "query")
	case "task":
		raw = jsonField(rawInput, "description")
	case "skill":
		raw = jsonField(rawInput, "skill")
	case "notebookedit":
		raw = jsonField(rawInput, "notebook_path")
	case "todowrite":
		raw = summarizeTodoWriteDigest(rawInput)
	default:
		raw = firstLine(rawInput)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = firstLine(rawInput)
	}
	return shrinkDigest(raw, toolDigestMaxRunes)
}

func jsonField(raw, key string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ""
	}
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func shrinkDigest(s string, maxRunes int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return runeHead(s, maxRunes-1) + "…"
}

// summarizeTodoWriteDigest returns "N/M done · <active>" style summary.
func summarizeTodoWriteDigest(raw string) string {
	var parsed struct {
		Todos []struct {
			Status     string `json:"status"`
			Content    string `json:"content"`
			ActiveForm string `json:"activeForm"`
		} `json:"todos"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return ""
	}
	total := len(parsed.Todos)
	done := 0
	active := ""
	for _, t := range parsed.Todos {
		if strings.EqualFold(t.Status, "completed") || strings.EqualFold(t.Status, "in_progress") {
			done++
		}
		if strings.EqualFold(t.Status, "in_progress") && active == "" {
			if t.ActiveForm != "" {
				active = t.ActiveForm
			} else {
				active = t.Content
			}
		}
	}
	if active != "" {
		return fmt.Sprintf("%d/%d · %s", done, total, active)
	}
	return fmt.Sprintf("%d/%d", done, total)
}

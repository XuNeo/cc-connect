package core

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	progressStyleLegacy  = "legacy"
	progressStyleCompact = "compact"
	progressStyleCard    = "card"

	// ProgressCardPayloadPrefix marks a structured payload for card-style progress.
	ProgressCardPayloadPrefix = "__cc_connect_progress_card_v1__:"

	// Keep a margin below platform hard limit for markdown wrappers/code fences.
	compactProgressMaxChars = maxPlatformMessageLen - 200

	// Bound each platform progress-card API call so a hung upstream request
	// does not block the whole turn forever.
	compactProgressAPITimeout = 15 * time.Second

	// writerMaxConsecutiveFailures is how many back-to-back Send/Update errors
	// tip the writer into disabled mode. Isolated transient errors (Feishu
	// tenant cache miss, rate limit, transient 5xx) do NOT disable the writer;
	// the next successful call resets the counter.
	writerMaxConsecutiveFailures = 5

	// writerCooldown is the idle gap after which consecutiveFailures resets
	// even without a success. Protects against slow-drip failures.
	writerCooldown = 60 * time.Second
)

// writerErrKind classifies a Feishu/platform error from the writer's point of
// view. Platforms optionally implement PermanentProgressError to mark known
// non-recoverable errors; everything else is transient and drop-framed.
type writerErrKind int

const (
	writerErrNone writerErrKind = iota
	writerErrTransient
	writerErrPermanent
)

// PermanentProgressError lets platform code flag errors that should *not* be
// retried — e.g. Feishu 230011 "message withdrawn" (target gone forever).
// Anything that doesn't implement it is treated as transient.
type PermanentProgressError interface {
	error
	IsPermanent() bool
}

func classifyWriterError(err error) writerErrKind {
	if err == nil {
		return writerErrNone
	}
	var pe PermanentProgressError
	if errors.As(err, &pe) && pe.IsPermanent() {
		return writerErrPermanent
	}
	return writerErrTransient
}

type ProgressCardState string

const (
	ProgressCardStateRunning   ProgressCardState = "running"
	ProgressCardStateCompleted ProgressCardState = "completed"
	ProgressCardStateFailed    ProgressCardState = "failed"
)

type ProgressCardEntryKind string

const (
	ProgressEntryInfo       ProgressCardEntryKind = "info"
	ProgressEntryThinking   ProgressCardEntryKind = "thinking"
	ProgressEntryToolUse    ProgressCardEntryKind = "tool_use"
	ProgressEntryToolResult ProgressCardEntryKind = "tool_result"
	ProgressEntryError      ProgressCardEntryKind = "error"
)

type ProgressCardEntry struct {
	Kind     ProgressCardEntryKind `json:"kind"`
	Text     string                `json:"text"`
	Tool     string                `json:"tool,omitempty"`
	Status   string                `json:"status,omitempty"`
	ExitCode *int                  `json:"exit_code,omitempty"`
	Success  *bool                 `json:"success,omitempty"`

	// Panel-level metadata (new in 2026-04 collapsible card redesign).
	// ID is the stable element_id used by Feishu collapsible_panel.
	ID string `json:"id,omitempty"`
	// DurationMs is the wall-clock time from tool_use to tool_result.
	// Zero for thinking/info/error.
	DurationMs int `json:"duration_ms,omitempty"`
	// PartIdx and PartTotal are set (>0) when one logical entry was
	// sharded across multiple panels due to the Feishu 30KB cap.
	PartIdx   int `json:"part_idx,omitempty"`
	PartTotal int `json:"part_total,omitempty"`
}

// ProgressCardPayload carries structured progress entries for platforms that
// render custom progress cards.
type ProgressCardPayload struct {
	Version   int                 `json:"version,omitempty"`
	Agent     string              `json:"agent,omitempty"`
	Lang      string              `json:"lang,omitempty"`
	State     ProgressCardState   `json:"state,omitempty"`
	Entries   []string            `json:"entries,omitempty"` // legacy fallback
	Items     []ProgressCardEntry `json:"items,omitempty"`   // ordered typed events
	Truncated bool                `json:"truncated"`
}

// BuildProgressCardPayload encodes progress entries into a transport string.
// This legacy builder keeps compatibility with old callers that only send text.
func BuildProgressCardPayload(entries []string, truncated bool) string {
	cleaned := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			cleaned = append(cleaned, entry)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	payload := ProgressCardPayload{
		Entries:   cleaned,
		Truncated: truncated,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return ProgressCardPayloadPrefix + string(b)
}

// BuildProgressCardPayloadV2 encodes ordered typed progress events.
func BuildProgressCardPayloadV2(items []ProgressCardEntry, truncated bool, agent string, lang Language, state ProgressCardState) string {
	cleaned := make([]ProgressCardEntry, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		kind := item.Kind
		if kind == "" {
			kind = ProgressEntryInfo
		}
		cleaned = append(cleaned, ProgressCardEntry{
			Kind:       kind,
			Text:       text,
			Tool:       strings.TrimSpace(item.Tool),
			Status:     strings.TrimSpace(item.Status),
			ExitCode:   item.ExitCode,
			Success:    item.Success,
			ID:         strings.TrimSpace(item.ID),
			DurationMs: item.DurationMs,
			PartIdx:    item.PartIdx,
			PartTotal:  item.PartTotal,
		})
	}
	if len(cleaned) == 0 {
		return ""
	}
	if state == "" {
		state = ProgressCardStateRunning
	}
	payload := ProgressCardPayload{
		Version:   2,
		Agent:     strings.TrimSpace(agent),
		Lang:      string(lang),
		State:     state,
		Items:     cleaned,
		Truncated: truncated,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return ProgressCardPayloadPrefix + string(b)
}

// ParseProgressCardPayload decodes a structured progress payload.
func ParseProgressCardPayload(content string) (*ProgressCardPayload, bool) {
	if !strings.HasPrefix(content, ProgressCardPayloadPrefix) {
		return nil, false
	}
	raw := strings.TrimPrefix(content, ProgressCardPayloadPrefix)
	var payload ProgressCardPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, false
	}
	legacy := make([]string, 0, len(payload.Entries))
	for _, entry := range payload.Entries {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			legacy = append(legacy, entry)
		}
	}
	items := make([]ProgressCardEntry, 0, len(payload.Items))
	for _, item := range payload.Items {
		item.Text = strings.TrimSpace(item.Text)
		item.Tool = strings.TrimSpace(item.Tool)
		item.Status = strings.TrimSpace(item.Status)
		if item.Text == "" {
			continue
		}
		if item.Kind == "" {
			item.Kind = ProgressEntryInfo
		}
		items = append(items, item)
	}
	if len(items) == 0 && len(legacy) > 0 {
		for _, entry := range legacy {
			items = append(items, ProgressCardEntry{
				Kind: inferLegacyEntryKind(entry),
				Text: entry,
			})
		}
	}
	if len(items) == 0 && len(legacy) == 0 {
		return nil, false
	}
	if payload.State == "" {
		payload.State = ProgressCardStateRunning
	}
	payload.Items = items
	payload.Entries = legacy
	if len(payload.Entries) == 0 && len(payload.Items) > 0 {
		payload.Entries = make([]string, 0, len(payload.Items))
		for _, item := range payload.Items {
			payload.Entries = append(payload.Entries, item.Text)
		}
	}
	return &payload, true
}

func inferLegacyEntryKind(entry string) ProgressCardEntryKind {
	switch {
	case strings.HasPrefix(entry, "💭"):
		return ProgressEntryThinking
	case strings.HasPrefix(entry, "🔧"), strings.Contains(entry, "**Tool #"):
		return ProgressEntryToolUse
	case strings.HasPrefix(entry, "🧾"):
		return ProgressEntryToolResult
	case strings.HasPrefix(entry, "❌"):
		return ProgressEntryError
	default:
		return ProgressEntryInfo
	}
}

// compactProgressWriter coalesces intermediate progress (thinking/tool-use)
// into one editable message for platforms that support message updates.
type compactProgressWriter struct {
	ctx       context.Context
	platform  Platform
	replyCtx  any
	transform func(string) string

	starter PreviewStarter
	updater MessageUpdater
	handle  any

	enabled    bool
	disabled   bool
	style      string
	usePayload bool

	// Failure accounting — replaces the binary failed flag.
	consecutiveFailures int
	lastFailureAt       time.Time

	content    string
	entries    []string
	items      []ProgressCardEntry
	state      ProgressCardState
	agentName  string
	lang       Language
	truncated  bool
	lastSent   string
	maxEntries int
}

func normalizeProgressStyle(style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "", progressStyleLegacy:
		return progressStyleLegacy
	case progressStyleCompact:
		return progressStyleCompact
	case progressStyleCard:
		return progressStyleCard
	default:
		return progressStyleLegacy
	}
}

func progressStyleForPlatform(p Platform) string {
	ps := progressStyleLegacy
	if sp, ok := p.(ProgressStyleProvider); ok {
		ps = normalizeProgressStyle(sp.ProgressStyle())
	}
	return ps
}

type progressStyleHintProvider interface {
	progressStyleHint() string
}

type progressCardPayloadHintProvider interface {
	supportsProgressCardPayloadHint() bool
}

func progressStyleForTarget(p Platform, replyCtx any) string {
	if hint, ok := replyCtx.(progressStyleHintProvider); ok {
		return normalizeProgressStyle(hint.progressStyleHint())
	}
	return progressStyleForPlatform(p)
}

func progressCardPayloadForTarget(p Platform, replyCtx any) bool {
	if hint, ok := replyCtx.(progressCardPayloadHintProvider); ok {
		return hint.supportsProgressCardPayloadHint()
	}
	if cap, ok := p.(ProgressCardPayloadSupport); ok {
		return cap.SupportsProgressCardPayload()
	}
	return false
}

// SuppressStandaloneToolResultEvent is true when a platform opts into progress
// styling (ProgressStyleProvider) but uses legacy mode. In that case tool_use
// lines are still shown, but a separate chat message for EventToolResult is
// skipped to avoid duplicate noise (e.g. Codex structured tool results on Feishu).
// Platforms without ProgressStyleProvider keep showing standalone tool results.
func SuppressStandaloneToolResultEvent(p Platform) bool {
	_, ok := p.(ProgressStyleProvider)
	if !ok {
		return false
	}
	return progressStyleForPlatform(p) == progressStyleLegacy
}

func newCompactProgressWriter(ctx context.Context, p Platform, replyCtx any, agentName string, lang Language, transform func(string) string) *compactProgressWriter {
	w := &compactProgressWriter{
		ctx:        ctx,
		platform:   p,
		replyCtx:   replyCtx,
		transform:  transform,
		style:      progressStyleForTarget(p, replyCtx),
		state:      ProgressCardStateRunning,
		agentName:  normalizeProgressAgentLabel(agentName),
		lang:       lang,
		maxEntries: 150,
	}
	if w.style != progressStyleCompact && w.style != progressStyleCard {
		slog.Debug("progress writer disabled: unsupported style", "platform", p.Name(), "style", w.style)
		return w
	}
	updater, ok := p.(MessageUpdater)
	if !ok {
		slog.Debug("progress writer disabled: platform has no MessageUpdater", "platform", p.Name(), "style", w.style)
		return w
	}
	w.enabled = true
	w.updater = updater
	if starter, ok := p.(PreviewStarter); ok {
		w.starter = starter
	}
	if w.style == progressStyleCard {
		if progressCardPayloadForTarget(p, replyCtx) {
			w.usePayload = true
			// Structured card payload is paginated across multiple cards by the
			// platform side; shrinking items here would shrink the card count
			// and cause bot-self-withdraw of earlier cards. Disable windowing
			// and let the paginator grow the card count monotonically.
			w.maxEntries = 0
		}
	}
	slog.Debug("progress writer enabled", "platform", p.Name(), "style", w.style, "use_payload", w.usePayload)
	return w
}

func normalizeProgressAgentLabel(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "agent":
		return "Agent"
	case "codex":
		return "Codex"
	case "claudecode", "claude-code", "cc":
		return "CC"
	case "gemini":
		return "Gemini"
	case "cursor":
		return "Cursor"
	case "qoder":
		return "Qoder"
	case "iflow":
		return "iFlow"
	case "opencode":
		return "OpenCode"
	case "pi":
		return "PI"
	default:
		n := strings.TrimSpace(name)
		if n == "" {
			return "Agent"
		}
		return strings.ToUpper(n[:1]) + n[1:]
	}
}

// recordFailure increments the failure counter and, depending on kind,
// either marks the writer disabled (permanent) or continues (transient below
// threshold). Returns true when the writer should stay alive.
func (w *compactProgressWriter) recordFailure(op string, err error) bool {
	kind := classifyWriterError(err)
	now := time.Now()
	// Cooldown window: a quiet gap resets the counter so slow-drip errors
	// don't silently accumulate over hours.
	if !w.lastFailureAt.IsZero() && now.Sub(w.lastFailureAt) >= writerCooldown {
		w.consecutiveFailures = 0
	}
	w.lastFailureAt = now
	w.consecutiveFailures++

	switch kind {
	case writerErrPermanent:
		w.disabled = true
		slog.Warn("progress writer: permanent error, disabling",
			"platform", w.platform.Name(), "style", w.style, "op", op, "error", err)
		return false
	case writerErrTransient:
		if w.consecutiveFailures >= writerMaxConsecutiveFailures {
			w.disabled = true
			slog.Warn("progress writer: disabled after consecutive failures",
				"platform", w.platform.Name(), "style", w.style, "op", op,
				"consecutive", w.consecutiveFailures, "error", err)
			return false
		}
		slog.Warn("progress writer: transient error, dropping frame",
			"platform", w.platform.Name(), "style", w.style, "op", op,
			"consecutive", w.consecutiveFailures, "error", err)
		return true
	}
	return true
}

// recordSuccess is called after every successful API write.
func (w *compactProgressWriter) recordSuccess() {
	w.consecutiveFailures = 0
}

// Append appends one progress item and updates the in-place message.
// Returns true when compact rendering handled this item; false means caller
// should fallback to legacy per-event send.
func (w *compactProgressWriter) Append(item string) bool {
	return w.AppendEvent(ProgressEntryInfo, item, "", item)
}

// AppendEvent appends one typed progress event and updates the in-place message.
// fallback is used for compact/plain rendering when style-specific rendering is not available.
func (w *compactProgressWriter) AppendEvent(kind ProgressCardEntryKind, text string, tool string, fallback string) bool {
	return w.AppendStructured(ProgressCardEntry{
		Kind: kind,
		Text: text,
		Tool: tool,
	}, fallback)
}

// AppendStructured appends one structured progress event and updates the in-place message.
func (w *compactProgressWriter) AppendStructured(item ProgressCardEntry, fallback string) bool {
	if !w.enabled || w.disabled {
		return false
	}
	text := strings.TrimSpace(item.Text)
	fallback = strings.TrimSpace(fallback)
	if text == "" && fallback == "" {
		return true
	}
	if text == "" {
		text = fallback
	}
	if fallback == "" {
		fallback = text
	}
	switch item.Kind {
	case ProgressEntryThinking, ProgressEntryError, ProgressEntryInfo:
		if w.transform != nil {
			text = w.transform(text)
			fallback = w.transform(fallback)
		}
	}
	kind := item.Kind
	if kind == "" {
		kind = ProgressEntryInfo
	}
	item.Kind = kind
	item.Text = text
	item.Tool = strings.TrimSpace(item.Tool)
	item.Status = strings.TrimSpace(item.Status)

	switch w.style {
	case progressStyleCard:
		w.items = append(w.items, item)
		w.entries = append(w.entries, fallback)
		truncated := false
		if w.maxEntries > 0 && len(w.items) > w.maxEntries {
			w.items = w.items[len(w.items)-w.maxEntries:]
			if len(w.entries) > w.maxEntries {
				w.entries = w.entries[len(w.entries)-w.maxEntries:]
			}
			truncated = true
		} else if w.maxEntries > 0 && len(w.entries) > w.maxEntries {
			w.entries = w.entries[len(w.entries)-w.maxEntries:]
			truncated = true
		}
		w.truncated = truncated
		if w.usePayload {
			w.content = BuildProgressCardPayloadV2(w.items, w.truncated, w.agentName, w.lang, w.state)
			if w.content == "" {
				slog.Warn("progress writer: failed to build structured payload", "platform", w.platform.Name())
				w.disabled = true
				return false
			}
		} else {
			w.content = renderCardProgressMarkdownFallback(w.entries, truncated)
			w.content = trimCompactProgressText(w.content, compactProgressMaxChars)
		}
	default:
		if w.content == "" {
			w.content = fallback
		} else {
			w.content += "\n\n" + fallback
		}
		w.content = trimCompactProgressText(w.content, compactProgressMaxChars)
	}

	if w.content == w.lastSent {
		return true
	}

	if w.handle == nil {
		if w.starter != nil {
			callCtx, cancel := w.withAPITimeout()
			handle, err := w.starter.SendPreviewStart(callCtx, w.replyCtx, w.content)
			cancel()
			if err != nil || handle == nil {
				slog.Warn("progress writer: SendPreviewStart failed", "platform", w.platform.Name(), "style", w.style, "error", err, "handle_nil", handle == nil)
				if err == nil {
					err = errors.New("SendPreviewStart returned nil handle")
				}
				w.recordFailure("SendPreviewStart", err)
				return false
			}
			w.handle = handle
			w.lastSent = w.content
			w.recordSuccess()
			return true
		}
		callCtx, cancel := w.withAPITimeout()
		err := w.platform.Send(callCtx, w.replyCtx, w.content)
		cancel()
		if err != nil {
			slog.Warn("progress writer: initial Send failed", "platform", w.platform.Name(), "style", w.style, "error", err)
			w.recordFailure("Send", err)
			return false
		}
		w.handle = w.replyCtx
		w.lastSent = w.content
		w.recordSuccess()
		return true
	}

	callCtx, cancel := w.withAPITimeout()
	err := w.updater.UpdateMessage(callCtx, w.handle, w.content)
	cancel()
	if err != nil {
		slog.Warn("progress writer: UpdateMessage failed", "platform", w.platform.Name(), "style", w.style, "error", err)
		w.recordFailure("UpdateMessage", err)
		return false
	}
	w.lastSent = w.content
	w.recordSuccess()
	return true
}

// Finalize updates card progress state (running/completed/failed) without
// appending a new progress entry.
func (w *compactProgressWriter) Finalize(state ProgressCardState) bool {
	if !w.enabled || w.disabled || w.style != progressStyleCard || !w.usePayload || w.handle == nil {
		return false
	}
	if state == "" {
		state = ProgressCardStateCompleted
	}
	if w.state == state {
		return true
	}
	w.state = state
	w.content = BuildProgressCardPayloadV2(w.items, w.truncated, w.agentName, w.lang, w.state)
	if w.content == "" || w.content == w.lastSent {
		return w.content != ""
	}
	callCtx, cancel := w.withAPITimeout()
	err := w.updater.UpdateMessage(callCtx, w.handle, w.content)
	cancel()
	if err != nil {
		slog.Warn("progress writer: Finalize UpdateMessage failed", "platform", w.platform.Name(), "style", w.style, "error", err)
		w.recordFailure("Finalize", err)
		return false
	}
	w.lastSent = w.content
	w.recordSuccess()
	return true
}

func (w *compactProgressWriter) withAPITimeout() (context.Context, context.CancelFunc) {
	if _, hasDeadline := w.ctx.Deadline(); hasDeadline {
		return w.ctx, func() {}
	}
	return context.WithTimeout(w.ctx, compactProgressAPITimeout)
}

func renderCardProgressMarkdownFallback(entries []string, truncated bool) string {
	var b strings.Builder
	b.WriteString("⏳ **Progress**\n")
	if truncated {
		b.WriteString("_Showing latest updates only._\n")
	}
	for i, entry := range entries {
		b.WriteString("\n")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(". ")
		b.WriteString(strings.ReplaceAll(entry, "\n", "\n   "))
	}
	return b.String()
}

func trimCompactProgressText(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	s = strings.TrimPrefix(s, "…\n")
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	rs := []rune(s)
	tail := strings.TrimLeft(string(rs[len(rs)-maxRunes:]), "\n")
	return "…\n" + tail
}

// toolPanelTracker assigns stable panel IDs and computes tool call durations
// by pairing tool_use events with their matching tool_result by tool_use_id.
type toolPanelTracker struct {
	seq     int
	pending map[string]pendingToolCall
}

type pendingToolCall struct {
	id    string
	start time.Time
}

func newToolPanelTracker() *toolPanelTracker {
	return &toolPanelTracker{pending: make(map[string]pendingToolCall)}
}

// onToolUse mutates entry to carry an ID + running status and records the
// start time under toolUseID for later pairing.
func (t *toolPanelTracker) onToolUse(toolUseID string, entry *ProgressCardEntry, now time.Time) {
	t.seq++
	entry.ID = makePanelID(entry.Tool, t.seq)
	if entry.Status == "" {
		entry.Status = "running"
	}
	if toolUseID != "" {
		t.pending[toolUseID] = pendingToolCall{id: entry.ID, start: now}
	}
}

// onToolResult mutates entry: copies the matching tool_use's ID (so the
// renderer merges them into one panel), computes duration, sets status.
func (t *toolPanelTracker) onToolResult(toolUseID string, entry *ProgressCardEntry, now time.Time) {
	if p, ok := t.pending[toolUseID]; ok {
		entry.ID = p.id
		if d := now.Sub(p.start); d > 0 {
			entry.DurationMs = int(d / time.Millisecond)
		}
		delete(t.pending, toolUseID)
	} else {
		// Orphan result (no matching tool_use in our window): give it a
		// fresh ID so it still renders as its own panel.
		t.seq++
		entry.ID = makePanelID(entry.Tool, t.seq)
	}
	if entry.Status == "" {
		if entry.Success != nil && !*entry.Success {
			entry.Status = "fail"
		} else if entry.ExitCode != nil && *entry.ExitCode != 0 {
			entry.Status = "fail"
		} else {
			entry.Status = "ok"
		}
	}
}

// makePanelID produces a stable, Feishu-legal element_id prefix + seq.
// Kept in core (not platform/feishu) to avoid an import cycle.
func makePanelID(tool string, seq int) string {
	var prefix string
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "bash", "shell", "run_shell_command":
		prefix = "bsh"
	case "read":
		prefix = "rd"
	case "write":
		prefix = "wr"
	case "edit":
		prefix = "ed"
	case "grep":
		prefix = "gp"
	case "glob":
		prefix = "gb"
	case "webfetch":
		prefix = "wf"
	case "websearch":
		prefix = "ws"
	case "todowrite":
		prefix = "td"
	case "task":
		prefix = "tk"
	case "skill":
		prefix = "sk"
	case "notebookedit":
		prefix = "nb"
	default:
		prefix = "tl"
	}
	return prefix + "_" + strconv.Itoa(seq)
}

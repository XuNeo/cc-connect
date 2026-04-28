package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"
)

type permanentErr struct{}

func (permanentErr) Error() string     { return "permanent" }
func (permanentErr) IsPermanent() bool { return true }

func TestClassifyWriterError_DefaultTransient(t *testing.T) {
	if k := classifyWriterError(errors.New("random upstream error")); k != writerErrTransient {
		t.Fatalf("default classify = %v, want transient", k)
	}
}

func TestClassifyWriterError_PermanentInterface(t *testing.T) {
	if k := classifyWriterError(permanentErr{}); k != writerErrPermanent {
		t.Fatalf("IsPermanent() wrapper = %v, want permanent", k)
	}
}

func TestClassifyWriterError_Nil(t *testing.T) {
	if k := classifyWriterError(nil); k != writerErrNone {
		t.Fatalf("nil = %v, want none", k)
	}
}

// flakyPlatform fails n times then succeeds forever.
type flakyPlatform struct {
	previewCapturePlatform
	remainingFails int
}

func (p *flakyPlatform) UpdateMessage(_ context.Context, _ any, content string) error {
	if p.remainingFails > 0 {
		p.remainingFails--
		return fmt.Errorf("transient kaboom %d", p.remainingFails)
	}
	p.updated = append(p.updated, content)
	return nil
}

func TestWriter_SingleTransientErrorDoesNotDisable(t *testing.T) {
	p := &flakyPlatform{remainingFails: 1}
	replyCtx := progressHintReplyCtx{style: progressStyleCard, payload: true}
	w := newCompactProgressWriter(context.Background(), p, replyCtx, "cc", LangEnglish, nil)

	if ok := w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "first"}, "first"); !ok {
		t.Fatal("first Append should succeed (SendPreviewStart path)")
	}
	// Second Append hits UpdateMessage → fails once.
	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "second"}, "second")
	if w.disabled {
		t.Fatal("writer disabled after ONE transient error — regression")
	}
	// Third Append must recover.
	if ok := w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "third"}, "third"); !ok {
		t.Fatal("writer did not recover after transient error cleared")
	}
	if len(p.updated) < 1 {
		t.Fatalf("expected at least one UpdateMessage success, got %d", len(p.updated))
	}
}

func TestWriter_ConsecutiveTransientThresholdDisables(t *testing.T) {
	p := &flakyPlatform{remainingFails: 100}
	replyCtx := progressHintReplyCtx{style: progressStyleCard, payload: true}
	w := newCompactProgressWriter(context.Background(), p, replyCtx, "cc", LangEnglish, nil)

	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "init"}, "init")

	for i := 0; i < writerMaxConsecutiveFailures; i++ {
		_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: fmt.Sprintf("msg%d", i)}, "x")
	}
	if !w.disabled {
		t.Fatalf("writer should disable after %d consecutive failures", writerMaxConsecutiveFailures)
	}
}

type permanentFailPlatform struct {
	previewCapturePlatform
}

func (p *permanentFailPlatform) UpdateMessage(_ context.Context, _ any, _ string) error {
	return permanentErr{}
}

func TestWriter_PermanentErrorDisablesImmediately(t *testing.T) {
	p := &permanentFailPlatform{}
	replyCtx := progressHintReplyCtx{style: progressStyleCard, payload: true}
	w := newCompactProgressWriter(context.Background(), p, replyCtx, "cc", LangEnglish, nil)

	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "init"}, "init")
	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "second"}, "second")
	if !w.disabled {
		t.Fatal("permanent error should disable writer immediately")
	}
}

func TestWriter_EmitsSingleDegradeWarnOnDisable(t *testing.T) {
	var captured []slog.Record
	h := &captureHandler{records: &captured}
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(oldLogger)

	p := &permanentFailPlatform{}
	replyCtx := progressHintReplyCtx{style: progressStyleCard, payload: true}
	w := newCompactProgressWriter(context.Background(), p, replyCtx, "cc", LangEnglish, nil)
	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "init"}, "init")
	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "boom"}, "boom")
	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "extra"}, "extra")

	seenDegrade := 0
	for _, r := range captured {
		if r.Message == "progress writer: degraded to legacy" {
			seenDegrade++
		}
	}
	if seenDegrade != 1 {
		t.Fatalf("expected exactly 1 degrade log, got %d", seenDegrade)
	}
}

type captureHandler struct {
	records *[]slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	*h.records = append(*h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func TestWriter_CooldownResetsCounter(t *testing.T) {
	p := &flakyPlatform{remainingFails: writerMaxConsecutiveFailures - 1}
	replyCtx := progressHintReplyCtx{style: progressStyleCard, payload: true}
	w := newCompactProgressWriter(context.Background(), p, replyCtx, "cc", LangEnglish, nil)
	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "init"}, "init")

	// Soak up almost-threshold failures, pause past the cooldown, then confirm
	// a new failure does NOT push us over (counter was reset).
	for i := 0; i < writerMaxConsecutiveFailures-1; i++ {
		_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "fail"}, "fail")
	}
	if w.disabled {
		t.Fatal("should not be disabled yet")
	}
	// Fake a cooldown gap by rewinding lastFailureAt manually.
	w.lastFailureAt = time.Now().Add(-writerCooldown - time.Second)
	p.remainingFails = 1
	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "isolated"}, "isolated")
	if w.disabled {
		t.Fatal("cooldown should have reset consecutiveFailures; writer should not disable")
	}
}

// feishuAPIErrorStub mirrors the real *feishuAPIError without importing the
// feishu package (avoid core→platform import cycle).
type feishuAPIErrorStub struct {
	Code int
	Msg  string
}

func (e *feishuAPIErrorStub) Error() string { return fmt.Sprintf("feishu: code=%d msg=%s", e.Code, e.Msg) }

// IsPermanent is NOT implemented here — so classifyWriterError treats these as transient.
// (The real feishu package marks 230011 permanent via its own IsPermanent method.)

type feishuCodePlatform struct {
	previewCapturePlatform
	remaining []int // codes to return in order; 0 means success
}

func (p *feishuCodePlatform) UpdateMessage(_ context.Context, _ any, content string) error {
	if len(p.remaining) == 0 {
		p.updated = append(p.updated, content)
		return nil
	}
	code := p.remaining[0]
	p.remaining = p.remaining[1:]
	if code == 0 {
		p.updated = append(p.updated, content)
		return nil
	}
	return fmt.Errorf("patch message: %w", &feishuAPIErrorStub{Code: code, Msg: "stub"})
}

func TestWriter_FeishuCode2200DoesNotLatch(t *testing.T) {
	p := &feishuCodePlatform{remaining: []int{2200, 0, 0}}
	replyCtx := progressHintReplyCtx{style: progressStyleCard, payload: true}
	w := newCompactProgressWriter(context.Background(), p, replyCtx, "cc", LangEnglish, nil)
	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "a"}, "a")
	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "b"}, "b") // 2200
	if w.disabled {
		t.Fatal("code=2200 must NOT disable the writer (transient tenant error)")
	}
	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "c"}, "c") // 0 = success
	if len(p.updated) < 1 {
		t.Fatalf("writer should have recovered; got %d updates", len(p.updated))
	}
}

func TestWriter_FeishuCode230099DoesNotLatch(t *testing.T) {
	p := &feishuCodePlatform{remaining: []int{230099, 0}}
	replyCtx := progressHintReplyCtx{style: progressStyleCard, payload: true}
	w := newCompactProgressWriter(context.Background(), p, replyCtx, "cc", LangEnglish, nil)
	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "a"}, "a")
	_ = w.AppendStructured(ProgressCardEntry{Kind: ProgressEntryInfo, Text: "b"}, "b") // 230099
	if w.disabled {
		t.Fatal("code=230099 must NOT disable the writer (transient schema error)")
	}
}

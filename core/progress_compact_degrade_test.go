package core

import (
	"context"
	"errors"
	"fmt"
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

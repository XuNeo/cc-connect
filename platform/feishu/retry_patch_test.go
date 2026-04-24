package feishu

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryPatch_SucceedsAfterRateLimit(t *testing.T) {
	attempts := 0
	doPatch := func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return &feishuAPIError{Code: 230020}
		}
		return nil
	}
	backoffs := []time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond}
	err := retryPatchWith(context.Background(), doPatch, backoffs)
	if err != nil {
		t.Errorf("want success after 3 tries, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts=%d, want 3", attempts)
	}
}

func TestRetryPatch_GivesUpAfterMaxAttempts(t *testing.T) {
	doPatch := func(ctx context.Context) error { return &feishuAPIError{Code: 230020} }
	backoffs := []time.Duration{1 * time.Millisecond, 1 * time.Millisecond, 1 * time.Millisecond}
	err := retryPatchWith(context.Background(), doPatch, backoffs)
	if err == nil {
		t.Error("want rate-limited error after exhausting retries")
	}
}

func TestRetryPatch_StopsOnNonRateLimitError(t *testing.T) {
	attempts := 0
	boom := errors.New("boom")
	doPatch := func(ctx context.Context) error { attempts++; return boom }
	err := retryPatchWith(context.Background(), doPatch, []time.Duration{1 * time.Millisecond})
	if !errors.Is(err, boom) {
		t.Errorf("want boom wrapped, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("attempts=%d, want 1 (non-rate-limit should not retry)", attempts)
	}
}

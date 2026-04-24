package feishu

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestRecoverPatchError_RateLimitRetriesThroughQueue asserts the rate-limit
// recovery path: a queued PATCH that returns code=230020 twice then succeeds
// should settle after 3 attempts when wrapped in retryPatchWith.
func TestRecoverPatchError_RateLimitRetriesThroughQueue(t *testing.T) {
	q := newUpdateQueue(50)
	defer q.stop()
	var calls int32
	doPatch := func(ctx context.Context) error {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return &feishuAPIError{Code: 230020, Msg: "rl"}
		}
		return nil
	}
	queued := func(c context.Context) error {
		return q.submit(c, "m", doPatch)
	}
	err := retryPatchWith(context.Background(), queued, []time.Duration{
		1 * time.Millisecond,
		1 * time.Millisecond,
		1 * time.Millisecond,
	})
	if err != nil {
		t.Errorf("want recovery nil, got %v", err)
	}
	if c := atomic.LoadInt32(&calls); c != 3 {
		t.Errorf("calls=%d, want 3", c)
	}
}

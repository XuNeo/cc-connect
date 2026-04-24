package feishu

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestUpdateQueue_SerializesPerMessageID(t *testing.T) {
	q := newUpdateQueue(10 /*qps per key*/)
	defer q.stop()

	var inflight int32
	var maxInflight int32
	worker := func(ctx context.Context) error {
		cur := atomic.AddInt32(&inflight, 1)
		for {
			m := atomic.LoadInt32(&maxInflight)
			if cur <= m || atomic.CompareAndSwapInt32(&maxInflight, m, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&inflight, -1)
		return nil
	}
	done := make(chan struct{}, 5)
	for i := 0; i < 5; i++ {
		go func() {
			_ = q.submit(context.Background(), "msgA", worker)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 5; i++ {
		<-done
	}
	if m := atomic.LoadInt32(&maxInflight); m > 1 {
		t.Errorf("max concurrent for same key = %d, want 1", m)
	}
}

func TestUpdateQueue_RespectsPerKeyQPS(t *testing.T) {
	q := newUpdateQueue(5 /*qps*/)
	defer q.stop()
	start := time.Now()
	for i := 0; i < 10; i++ {
		_ = q.submit(context.Background(), "msgB", func(ctx context.Context) error { return nil })
	}
	elapsed := time.Since(start)
	if elapsed < 1_500*time.Millisecond {
		t.Errorf("finished in %v, expected >=1.5s for 10 calls @5qps", elapsed)
	}
}

func TestUpdateQueue_PropagatesError(t *testing.T) {
	q := newUpdateQueue(10)
	defer q.stop()
	boom := errors.New("boom")
	err := q.submit(context.Background(), "msgC", func(ctx context.Context) error { return boom })
	if !errors.Is(err, boom) {
		t.Errorf("want wrapped boom, got %v", err)
	}
}

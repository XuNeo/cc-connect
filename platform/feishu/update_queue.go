package feishu

import (
	"context"
	"sync"
	"time"
)

// updateQueue serializes Feishu card PATCH calls per message_id and enforces
// a per-key QPS cap so we never exceed Feishu's 5 QPS single-card update
// limit. One lightweight goroutine per message_id consumes a channel of
// callbacks; the goroutine exits when idle to free memory on long sessions.
type updateQueue struct {
	qps     int
	mu      sync.Mutex
	queues  map[string]*msgQueue
	stopped bool
}

type msgQueue struct {
	ch   chan queueJob
	done chan struct{}
}

type queueJob struct {
	ctx  context.Context
	fn   func(context.Context) error
	resp chan error
}

func newUpdateQueue(qps int) *updateQueue {
	if qps <= 0 {
		qps = 5
	}
	return &updateQueue{
		qps:    qps,
		queues: make(map[string]*msgQueue),
	}
}

func (q *updateQueue) submit(ctx context.Context, key string, fn func(context.Context) error) error {
	mq := q.getOrStart(key)
	if mq == nil {
		return context.Canceled
	}
	resp := make(chan error, 1)
	select {
	case mq.ch <- queueJob{ctx: ctx, fn: fn, resp: resp}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-resp:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *updateQueue) getOrStart(key string) *msgQueue {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return nil
	}
	if mq, ok := q.queues[key]; ok {
		return mq
	}
	mq := &msgQueue{ch: make(chan queueJob, 16), done: make(chan struct{})}
	q.queues[key] = mq
	go q.runQueue(key, mq)
	return mq
}

func (q *updateQueue) runQueue(key string, mq *msgQueue) {
	interval := time.Second / time.Duration(q.qps)
	last := time.Now().Add(-interval)
	idleTimer := time.NewTimer(30 * time.Second)
	defer idleTimer.Stop()
	for {
		select {
		case job, ok := <-mq.ch:
			if !ok {
				return
			}
			if wait := interval - time.Since(last); wait > 0 {
				select {
				case <-time.After(wait):
				case <-job.ctx.Done():
					job.resp <- job.ctx.Err()
					continue
				}
			}
			err := job.fn(job.ctx)
			last = time.Now()
			job.resp <- err
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(30 * time.Second)
		case <-idleTimer.C:
			q.mu.Lock()
			if cur := q.queues[key]; cur == mq && len(mq.ch) == 0 {
				delete(q.queues, key)
				close(mq.done)
				q.mu.Unlock()
				return
			}
			q.mu.Unlock()
			idleTimer.Reset(30 * time.Second)
		}
	}
}

func (q *updateQueue) stop() {
	q.mu.Lock()
	q.stopped = true
	for k, mq := range q.queues {
		close(mq.ch)
		delete(q.queues, k)
	}
	q.mu.Unlock()
}

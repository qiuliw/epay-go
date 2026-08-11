// internal/worker/pool_test.go
package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolFixedConcurrency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const concurrency = 4
	const jobs = 20
	pool := NewPool(concurrency)
	pool.Start(ctx)

	var inFlight int32
	var maxInFlight int32
	var done sync.WaitGroup
	done.Add(jobs)

	for i := 0; i < jobs; i++ {
		ok := pool.Submit(ctx, func(context.Context) {
			defer done.Done()
			cur := atomic.AddInt32(&inFlight, 1)
			for {
				old := atomic.LoadInt32(&maxInFlight)
				if cur <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, cur) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			atomic.AddInt32(&inFlight, -1)
		})
		if !ok {
			t.Fatal("submit failed")
		}
	}

	done.Wait()
	cancel()
	pool.Wait()

	if maxInFlight > concurrency {
		t.Fatalf("max in-flight %d exceeds concurrency %d", maxInFlight, concurrency)
	}
	if maxInFlight < 1 {
		t.Fatal("expected some concurrency")
	}
}

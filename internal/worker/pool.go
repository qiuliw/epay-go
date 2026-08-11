// internal/worker/pool.go
package worker

import (
	"context"
	"sync"
)

// Job 池内执行的任务
type Job func(ctx context.Context)

// Pool 固定并发 worker 池
type Pool struct {
	concurrency int
	jobs        chan Job
	wg          sync.WaitGroup
}

// NewPool 创建固定并发池，jobs 缓冲为 concurrency*2
func NewPool(concurrency int) *Pool {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &Pool{
		concurrency: concurrency,
		jobs:        make(chan Job, concurrency*2),
	}
}

// Start 启动固定数量 worker，监听 ctx 退出
func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1) // 添加一个等待组计数器
		go func() {
			defer p.wg.Done() // 完成一个任务，减少等待组计数器
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-p.jobs:
					if !ok {
						return
					}
					job(ctx)
				}
			}
		}()
	}
}

// Submit 提交任务；池满时阻塞（背压）
func (p *Pool) Submit(ctx context.Context, job Job) bool {
	select {
	case <-ctx.Done():
		return false
	case p.jobs <- job:
		return true
	}
}

// Wait 等待在途 worker 退出（需先 cancel ctx）
func (p *Pool) Wait() {
	p.wg.Wait()
}

// Package worker provides a fixed-size worker pool for concurrent job
// execution. Jobs are submitted via Submit() and processed by a configurable
// number of goroutines. Integrates with the job package for retry/timeout
// and with sctx via Component and HubComponent for lifecycle management.
package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackdes93/fcontext/job"
	"github.com/jackdes93/fcontext/sctx"
)

type MetricsHook interface {
	IncJobStarted(name string)
	IncJobSuccess(name string, latency time.Duration)
	IncJobFailed(name string, err error, latency time.Duration)
	IncJobPermanentFailed(name string, err error)
}

type PoolOption func(*PoolConfig)

type PoolConfig struct {
	Name        string
	Size        int           // số worker goroutine
	QueueSize   int           // độ dài buffer queue
	StopTimeout time.Duration // timeout khi shutdown
}

func WithName(name string) PoolOption            { return func(c *PoolConfig) { c.Name = name } }
func WithSize(n int) PoolOption                  { return func(c *PoolConfig) { c.Size = n } }
func WithQueueSize(n int) PoolOption             { return func(c *PoolConfig) { c.QueueSize = n } }
func WithStopTimeout(d time.Duration) PoolOption { return func(c *PoolConfig) { c.StopTimeout = d } }

type PoolStats struct {
	Name        string
	WorkerCount int
	QueueSize   int
	Running     bool
	Stopped     bool
	// Runtime stats
	QueueDepth     int64
	ActiveWorkers  int64
	TotalSubmitted int64
	TotalProcessed int64
}

type Pool interface {
	Submit(j job.Job) bool                                // false nếu queue full hoặc pool chưa chạy
	SubmitFunc(fn job.Handler, opts ...job.Option) bool   // shortcut: tạo Job từ func và submit ngay
	Run(ctx context.Context)                              // blocking
	Stop(ctx context.Context)                             // graceful stop
	IsRunning() bool
	Stats() PoolStats
	Ready() <-chan struct{} // closed when workers are started and ready to accept jobs
}

type pool struct {
	cfg    PoolConfig
	log    sctx.Logger
	metric MetricsHook

	queue     chan job.Job
	wg        sync.WaitGroup
	once      sync.Once
	mu        sync.RWMutex
	running   bool
	stopped   bool
	// ready is closed by Run() after all workers are registered in wg.
	// Only Run() may close this channel — Stop() uses a non-blocking read
	// to detect whether Run() was ever called, avoiding the wg.Add vs
	// wg.Wait data race.
	ready chan struct{}

	activeWorkers  atomic.Int64
	totalSubmitted atomic.Int64
	totalProcessed atomic.Int64
}

func NewPool(log sctx.Logger, metric MetricsHook, opts ...PoolOption) Pool {
	p := &pool{
		cfg: PoolConfig{
			Name:        "worker",
			Size:        4,
			QueueSize:   1024,
			StopTimeout: 10 * time.Second,
		},
		log:    log,
		metric: metric,
	}
	for _, o := range opts {
		o(&p.cfg)
	}
	p.queue = make(chan job.Job, p.cfg.QueueSize)
	p.ready = make(chan struct{})
	return p
}

func (p *pool) Submit(j job.Job) bool {
	if j == nil {
		return false
	}

	p.mu.RLock()
	running := p.running
	stopped := p.stopped
	p.mu.RUnlock()

	if stopped {
		p.log.Warn("cannot submit job: %v", ErrPoolStopped)
		return false
	}
	if !running {
		p.log.Warn("cannot submit job: %v", ErrPoolNotReady)
		return false
	}

	select {
	case p.queue <- j:
		p.totalSubmitted.Add(1)
		return true
	default:
		p.log.Warn("job dropped: %v", ErrQueueFull)
		return false
	}
}

// SubmitFunc creates a Job from a handler function and submits it to the pool.
// Returns false if the pool is stopped, not running, or the queue is full.
func (p *pool) SubmitFunc(fn job.Handler, opts ...job.Option) bool {
	return p.Submit(job.New(fn, opts...))
}

func (p *pool) Run(ctx context.Context) {
	p.once.Do(func() {
		p.mu.Lock()
		p.running = true
		p.mu.Unlock()

		for i := 0; i < p.cfg.Size; i++ {
			p.wg.Add(1)
			go p.worker(ctx, i)
		}
		// Signal that all wg.Add calls are done. Only Run() closes this —
		// Stop() never closes it, preventing the wg.Add vs wg.Wait race.
		close(p.ready)
	})
	<-ctx.Done()
	p.Stop(ctx)
}

func (p *pool) Stop(ctx context.Context) {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	p.running = false
	p.mu.Unlock()

	stopCtx, cancel := context.WithTimeout(context.Background(), p.cfg.StopTimeout)
	defer cancel()

	// Non-blocking check: if Run() closed ready, all wg.Add calls are done
	// and wg.Wait() is safe. If ready is not closed, Run() never finished
	// setup (or was never called) — wg count is 0, skip Wait entirely.
	select {
	case <-p.ready:
		close(p.queue)
		done := make(chan struct{})
		go func() { p.wg.Wait(); close(done) }()
		select {
		case <-stopCtx.Done():
			p.log.Warn("worker pool stop timeout reached")
		case <-done:
			p.log.Info("worker pool stopped")
		}
	default:
		// Run() never completed — no workers to drain.
		p.log.Info("worker pool stopped (workers never started)")
	}
}

func (p *pool) Ready() <-chan struct{} { return p.ready }

func (p *pool) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

func (p *pool) Stats() PoolStats {
	p.mu.RLock()
	running := p.running
	stopped := p.stopped
	p.mu.RUnlock()
	return PoolStats{
		Name:           p.cfg.Name,
		WorkerCount:    p.cfg.Size,
		QueueSize:      p.cfg.QueueSize,
		Running:        running,
		Stopped:        stopped,
		QueueDepth:     int64(len(p.queue)),
		ActiveWorkers:  p.activeWorkers.Load(),
		TotalSubmitted: p.totalSubmitted.Load(),
		TotalProcessed: p.totalProcessed.Load(),
	}
}

func (p *pool) worker(ctx context.Context, idx int) {
	defer p.wg.Done()
	log := p.log.WithPrefix("worker")

	for j := range p.queue {
		name := j.Name()
		start := time.Now()
		p.activeWorkers.Add(1)
		if p.metric != nil {
			p.metric.IncJobStarted(name)
		}

		err := j.RunWithRetry(ctx)
		p.activeWorkers.Add(-1)
		p.totalProcessed.Add(1)
		lat := time.Since(start)

		if err == nil {
			log.Info("job success name=%s latency=%s", name, lat)
			if p.metric != nil {
				p.metric.IncJobSuccess(name, lat)
			}
			continue
		}
		log.Warn("job failed name=%s state=%s retry=%d err=%v", name, j.State(), j.RetryIndex(), err)
		if j.State() == job.StateRetryFailed && p.metric != nil {
			p.metric.IncJobPermanentFailed(name, err)
		}
		if p.metric != nil {
			p.metric.IncJobFailed(name, err, lat)
		}
	}
}

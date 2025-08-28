package worker

import (
	"context"
	"sync"
	"time"

	"github.com/jackdes93/fcontext"
	"github.com/jackdes93/fcontext/job"
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

type Pool interface {
	Submit(j job.Job) bool    // false nếu queue full
	Run(ctx context.Context)  // blocking
	Stop(ctx context.Context) // graceful stop
}

type pool struct {
	cfg    PoolConfig
	log    fcontext.Logger
	metric MetricsHook

	queue chan job.Job
	wg    sync.WaitGroup
	once  sync.Once
}

func NewPool(log fcontext.Logger, metric MetricsHook, opts ...PoolOption) Pool {
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
	return p
}

func (p *pool) Submit(j job.Job) bool {
	select {
	case p.queue <- j:
		return true
	default:
		p.log.Warn("queue full, drop job")
		return false
	}
}

func (p *pool) Run(ctx context.Context) {
	p.once.Do(func() {
		for i := 0; i < p.cfg.Size; i++ {
			p.wg.Add(1)
			go p.worker(ctx, i)
		}
	})
	<-ctx.Done()
	p.Stop(ctx)
}

func (p *pool) Stop(ctx context.Context) {
	stopCtx, cancel := context.WithTimeout(ctx, p.cfg.StopTimeout)
	defer cancel()

	// đóng queue để worker dọn dẹp
	close(p.queue)
	done := make(chan struct{})
	go func() { p.wg.Wait(); close(done) }()

	select {
	case <-stopCtx.Done():
		p.log.Warn("worker pool stop timeout reached")
	case <-done:
		p.log.Info("worker pool stopped")
	}
}

func (p *pool) worker(ctx context.Context, idx int) {
	defer p.wg.Done()
	log := p.log.WithPrefix("worker")

	for j := range p.queue {
		start := time.Now()
		if p.metric != nil {
			p.metric.IncJobStarted(nameOf(j))
		}

		err := j.RunWithRetry(ctx)
		lat := time.Since(start)

		if err == nil {
			log.Info("job success name=%s latency=%s", nameOf(j), lat)
			if p.metric != nil {
				p.metric.IncJobSuccess(nameOf(j), lat)
			}
			continue
		}
		log.Warn("job failed name=%s state=%s retry=%d err=%v", nameOf(j), j.State(), j.RetryIndex(), err)
		if j.State() == job.StateRetryFailed && p.metric != nil {
			p.metric.IncJobPermanentFailed(nameOf(j), err)
		}
		if p.metric != nil {
			p.metric.IncJobFailed(nameOf(j), err, lat)
		}
	}
}

func nameOf(j job.Job) string {
	// best-effort: dùng type name làm job name khi không có metadata;
	// nếu cần “tên” chính xác, bạn có thể mở rộng Job interface để expose Name().
	return "job"
}

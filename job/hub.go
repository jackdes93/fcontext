package job

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// JobHandler đại diện cho một handler xử lý job của một type cụ thể
type JobHandler interface {
	Handle(ctx context.Context) error
	Type() string // return job type identifier
}

// Hub quản lý multiple job types
type Hub interface {
	// Create tạo job từ một job handler
	Create(jobType string, handler JobHandler, opts ...Option) (Job, error)
	
	// Submit submit job trực tiếp
	Submit(job Job) bool
	
	// Stop dừng hub và cancel các job đang chạy
	Stop(ctx context.Context) error
	
	// IsRunning kiểm tra hub còn hoạt động không
	IsRunning() bool
}

// hub struct quản lý job submission
type hub struct {
	mu      sync.RWMutex
	poolFn  func(j Job) bool // function để submit job vào pool
	stopped bool
	jobs    map[string]Job   // tracking running jobs
}

// NewHub tạo hub mới
// poolFn là function để submit job vào pool
func NewHub(poolFn func(j Job) bool) Hub {
	if poolFn == nil {
		return &hub{
			poolFn:  func(j Job) bool { return true },
			jobs:    make(map[string]Job),
			stopped: false,
		}
	}
	return &hub{
		poolFn:  poolFn,
		jobs:    make(map[string]Job),
		stopped: false,
	}
}

func (h *hub) Create(jobType string, handler JobHandler, opts ...Option) (Job, error) {
	h.mu.RLock()
	stopped := h.stopped
	h.mu.RUnlock()
	
	if stopped {
		return nil, fmt.Errorf("hub is stopped")
	}
	
	if handler == nil {
		return nil, fmt.Errorf("job handler cannot be nil")
	}
	
	if jobType == "" {
		return nil, fmt.Errorf("job type cannot be empty")
	}
	
	cfg := Config{
		MaxTimeout: defaultTimeout,
		Name:       jobType,
	}
	for _, o := range opts {
		o(&cfg)
	}
	
	// Wrap JobHandler gói trong một adaptor Job
	wrappedJob := &hubJobAdapter{
		handler: handler,
		cfg:     cfg,
		state:   StateInit,
		retryIndex: -1,
	}
	
	return wrappedJob, nil
}

func (h *hub) Submit(job Job) bool {
	h.mu.RLock()
	stopped := h.stopped
	poolFn := h.poolFn
	h.mu.RUnlock()
	
	if stopped {
		return false
	}
	
	if job == nil {
		return false
	}
	
	return poolFn(job)
}

func (h *hub) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	h.stopped = true
	// Cleanup jobs tracking
	h.jobs = make(map[string]Job)
	return nil
}

func (h *hub) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return !h.stopped
}

// hubJobAdapter wraps JobHandler để implement Job interface
type hubJobAdapter struct {
	handler JobHandler
	cfg     Config
	mu      sync.Mutex
	state   State
	retryIndex int
	lastErr error
}

func (a *hubJobAdapter) Execute(ctx context.Context) error {
	a.setState(StateRunning)

	ctx2 := ctx
	var cancel context.CancelFunc
	if a.cfg.MaxTimeout > 0 {
		ctx2, cancel = context.WithTimeout(ctx, a.cfg.MaxTimeout)
		defer cancel()
	}
	
	err := a.handler.Handle(ctx2)
	if err != nil {
		a.setError(err)
		a.setState(StateFailed)
		return err
	}
	
	a.setState(StateCompleted)
	if a.cfg.OnComplete != nil {
		a.cfg.OnComplete()
	}
	return nil
}

func (a *hubJobAdapter) Retry(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.cfg.Retries) == 0 || a.retryIndex >= len(a.cfg.Retries)-1 {
		if a.cfg.OnPermanent != nil {
			a.cfg.OnPermanent(a.lastErr)
		}
		return a.lastErr
	}

	a.retryIndex++
	delay := applyJitter(a.cfg.Retries[a.retryIndex], a.cfg.JitterPct)
	timer := time.NewTimer(delay)
	a.mu.Unlock()
	
	select {
	case <-ctx.Done():
		timer.Stop()
		a.mu.Lock()
		a.lastErr = ctx.Err()
		a.state = StateFailed
		a.mu.Unlock()
		return ctx.Err()
	case <-timer.C:
	}
	
	a.mu.Lock()
	a.mu.Unlock()
	
	err := a.Execute(ctx)
	a.mu.Lock()
	
	if err == nil {
		a.state = StateCompleted
		a.mu.Unlock()
		return nil
	}
	
	if a.cfg.OnRetry != nil {
		var next time.Duration
		if a.retryIndex < len(a.cfg.Retries)-1 {
			next = applyJitter(a.cfg.Retries[a.retryIndex+1], a.cfg.JitterPct)
		}
		a.cfg.OnRetry(a.retryIndex, next, err)
	}
	
	if a.retryIndex >= len(a.cfg.Retries)-1 {
		a.state = StateRetryFailed
		if a.cfg.OnPermanent != nil {
			a.cfg.OnPermanent(err)
		}
	}
	a.mu.Unlock()
	return err
}

func (a *hubJobAdapter) RunWithRetry(ctx context.Context) error {
	if err := a.Execute(ctx); err == nil {
		return nil
	}
	
	for {
		if err := a.Retry(ctx); err == nil {
			return nil
		} else if a.RetryIndex() >= len(a.cfg.Retries)-1 {
			return a.LastError()
		}
	}
}

func (a *hubJobAdapter) State() State { 
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state 
}

func (a *hubJobAdapter) RetryIndex() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.retryIndex
}

func (a *hubJobAdapter) LastError() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastErr
}

func (a *hubJobAdapter) SetRetries(ds []time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.Retries = append([]time.Duration(nil), ds...)
	a.retryIndex = -1
}

func (a *hubJobAdapter) setState(s State) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = s
}

func (a *hubJobAdapter) setError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastErr = err
}

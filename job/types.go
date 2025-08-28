package job

import (
	"context"
	"errors"
	"sync"
	"time"
)

type State int

const (
	StateInit State = iota
	StateRunning
	StateFailed
	StateTimeout
	StateCompleted
	StateRetryFailed
)

func (s State) String() string {
	return [...]string{"Init", "Running", "Failed", "Timeout", "Completed", "RetryFailed"}[s]
}

type Handler func(ctx context.Context) error

type Option func(*Config)

type Config struct {
	Name        string
	MaxTimeout  time.Duration
	Retries     []time.Duration // lịch chờ retry: 0, 1s, 2s, 4s, ...
	JitterPct   float64         // 0..1, ví dụ 0.2 => ±20% delay
	OnRetry     func(idx int, nextDelay time.Duration, lastErr error)
	OnComplete  func()
	OnPermanent func(lastErr error)
}

const defaultTimeout = 10 * time.Second

func WithName(name string) Option        { return func(c *Config) { c.Name = name } }
func WithTimeout(d time.Duration) Option { return func(c *Config) { c.MaxTimeout = d } }
func WithRetries(ds []time.Duration) Option {
	return func(c *Config) { c.Retries = append([]time.Duration(nil), ds...) }
}
func WithJitter(p float64) Option { return func(c *Config) { c.JitterPct = p } }
func WithOnRetry(fn func(int, time.Duration, error)) Option {
	return func(c *Config) { c.OnRetry = fn }
}
func WithOnComplete(fn func()) Option       { return func(c *Config) { c.OnComplete = fn } }
func WithOnPermanent(fn func(error)) Option { return func(c *Config) { c.OnPermanent = fn } }

// ---- Job implementation ----

type Job interface {
	Execute(ctx context.Context) error
	Retry(ctx context.Context) error
	RunWithRetry(ctx context.Context) error

	State() State
	RetryIndex() int
	LastError() error
	SetRetries([]time.Duration)
}

type job struct {
	cfg        Config
	h          Handler
	mu         sync.Mutex
	state      State
	retryIndex int
	lastErr    error
}

func New(h Handler, opts ...Option) Job {
	j := &job{
		cfg: Config{
			MaxTimeout: defaultTimeout,
		},
		h:          h,
		retryIndex: -1,
		state:      StateInit,
	}
	for _, o := range opts {
		o(&j.cfg)
	}
	return j
}

func (j *job) Execute(ctx context.Context) error {
	j.setState(StateRunning)

	// Apply timeout if configured
	ctx2 := ctx
	var cancel context.CancelFunc
	if j.cfg.MaxTimeout > 0 {
		ctx2, cancel = context.WithTimeout(ctx, j.cfg.MaxTimeout)
		defer cancel()
	}

	errCh := make(chan error, 1)
	go func() { errCh <- j.h(ctx2) }()

	select {
	case <-ctx2.Done():
		err := ctx2.Err()
		j.setErr(err)
		if errors.Is(err, context.DeadlineExceeded) {
			j.setState(StateTimeout)
		} else {
			j.setState(StateFailed)
		}
		return err
	case err := <-errCh:
		if err != nil {
			j.setErr(err)
			j.setState(StateFailed)
			return err
		}
		j.setErr(nil)
		j.setState(StateCompleted)
		if j.cfg.OnComplete != nil {
			j.cfg.OnComplete()
		}
		return nil
	}
}

func (j *job) Retry(ctx context.Context) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if len(j.cfg.Retries) == 0 || j.retryIndex >= len(j.cfg.Retries)-1 {
		// no more retries
		if j.cfg.OnPermanent != nil {
			j.cfg.OnPermanent(j.lastErr)
		}
		return j.lastErr
	}

	j.retryIndex++
	delay := applyJitter(j.cfg.Retries[j.retryIndex], j.cfg.JitterPct)
	timer := time.NewTimer(delay)
	j.mu.Unlock()
	select {
	case <-ctx.Done():
		timer.Stop()
		j.mu.Lock()
		j.lastErr = ctx.Err()
		j.state = StateFailed
		return j.lastErr
	case <-timer.C:
		// proceed
	}
	j.mu.Lock()

	// unlock before execute
	j.mu.Unlock()
	err := j.Execute(ctx)
	j.mu.Lock()

	if err == nil {
		j.state = StateCompleted
		return nil
	}
	if j.cfg.OnRetry != nil {
		// next delay preview (if any)
		var next time.Duration
		if j.retryIndex < len(j.cfg.Retries)-1 {
			next = applyJitter(j.cfg.Retries[j.retryIndex+1], j.cfg.JitterPct)
		}
		j.cfg.OnRetry(j.retryIndex, next, err)
	}
	if j.retryIndex >= len(j.cfg.Retries)-1 {
		j.state = StateRetryFailed
		if j.cfg.OnPermanent != nil {
			j.cfg.OnPermanent(err)
		}
	}
	return err
}

func (j *job) RunWithRetry(ctx context.Context) error {
	if err := j.Execute(ctx); err == nil {
		return nil
	}
	for {
		if err := j.Retry(ctx); err == nil {
			return nil
		} else if j.retryIndex >= len(j.cfg.Retries)-1 {
			return j.lastErr
		}
	}
}

func (j *job) State() State     { j.mu.Lock(); defer j.mu.Unlock(); return j.state }
func (j *job) RetryIndex() int  { j.mu.Lock(); defer j.mu.Unlock(); return j.retryIndex }
func (j *job) LastError() error { j.mu.Lock(); defer j.mu.Unlock(); return j.lastErr }
func (j *job) SetRetries(rs []time.Duration) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.cfg.Retries = append([]time.Duration(nil), rs...)
	j.retryIndex = -1
}

// ---- helpers ----

func (j *job) setState(s State) { j.mu.Lock(); j.state = s; j.mu.Unlock() }
func (j *job) setErr(err error) { j.mu.Lock(); j.lastErr = err; j.mu.Unlock() }

func applyJitter(d time.Duration, pct float64) time.Duration {
	if pct <= 0 {
		return d
	}
	// simple ±pct jitter (deterministic-ish via time.Now().UnixNano())
	n := time.Now().UnixNano()
	sign := int64(1)
	if n&1 == 1 {
		sign = -1
	}
	j := time.Duration(float64(d) * pct)
	return d + time.Duration(sign)*j/2
}

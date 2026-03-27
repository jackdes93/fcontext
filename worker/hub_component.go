package worker

import (
	"context"
	"sync"

	"github.com/jackdes93/fcontext"
	"github.com/jackdes93/fcontext/job"
)

// HubComponent quản lý job hub + worker pool
type HubComponent struct {
	id     string
	log    fcontext.Logger
	pool   Pool
	hub    job.Hub
	opts   []PoolOption
	metric MetricsHook
	mu     sync.Mutex
}

func NewHubComponent(id string, metric MetricsHook, opts ...PoolOption) *HubComponent {
	return &HubComponent{
		id:     id,
		metric: metric,
		opts:   opts,
	}
}

func (c *HubComponent) ID() string { return c.id }
func (c *HubComponent) InitFlags() {}
func (c *HubComponent) Order() int { return 40 }

func (c *HubComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.pool != nil {
		c.pool.Stop(ctx)
	}
	if c.hub != nil {
		c.hub.Stop(ctx)
	}
	return nil
}

func (c *HubComponent) Activate(ctx context.Context, sv fcontext.ServiceContext) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.log = sv.Logger(c.ID())

	c.pool = NewPool(c.log, c.metric, c.opts...)
	
	// Tạo hub với pool
	c.hub = job.NewHub(func(j job.Job) bool {
		return c.pool.Submit(j)
	})
	
	// Chạy pool nền
	go c.pool.Run(ctx)

	c.log.Info("hub component started")
	return nil
}

// GetHub trả về hub để đăng ký job handlers
func (c *HubComponent) GetHub() job.Hub {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hub
}

// Submit submit job trực tiếp
func (c *HubComponent) Submit(j job.Job) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.pool == nil {
		return false
	}
	return c.pool.Submit(j)
}

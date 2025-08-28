package worker

import (
	"context"

	"github.com/jackdes93/fcontext"
	"github.com/jackdes93/fcontext/job"
)

type Component struct {
	id     string
	log    fcontext.Logger
	pool   Pool
	opts   []PoolOption
	metric MetricsHook
}

func NewComponent(id string, metric MetricsHook, opts ...PoolOption) *Component {
	return &Component{id: id, metric: metric, opts: opts}
}

func (c *Component) ID() string { return c.id }
func (c *Component) InitFlags() {}
func (c *Component) Order() int { return 40 } // khởi động trước HTTP/MQTT (tuỳ bạn)
func (c *Component) Stop(ctx context.Context) error {
	if c.pool != nil {
		c.pool.Stop(ctx)
	}
	return nil
}

func (c *Component) Activate(ctx context.Context, sv fcontext.ServiceContext) error {
	c.log = sv.Logger(c.ID())

	c.pool = NewPool(c.log, c.metric, c.opts...)
	// Chạy pool nền
	go c.pool.Run(ctx)

	c.log.Info("worker component started")
	return nil
}

// Expose API để submit job từ nơi khác
func (c *Component) Submit(j job.Job) bool {
	if c.pool == nil {
		return false
	}
	return c.pool.Submit(j)
}

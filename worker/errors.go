package worker

import "errors"

// Sentinel errors cho package worker — phân biệt lý do Submit() trả về false.
var (
	ErrQueueFull    = errors.New("worker: job queue is full")
	ErrPoolStopped  = errors.New("worker: pool is stopped")
	ErrPoolNotReady = errors.New("worker: pool is not running")
)

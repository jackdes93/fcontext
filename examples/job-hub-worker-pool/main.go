package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackdes93/fcontext/job"
)

// ============================================================================
// WORKER POOL - Quản lý số lượng goroutine cùng chạy
// ============================================================================

type WorkerPool struct {
	taskChan chan job.Job
	wg       sync.WaitGroup
	mu       sync.Mutex
	running  int32 // số goroutine đang chạy
	maxSize  int   // số goroutine tối đa
}

func NewWorkerPool(maxSize int) *WorkerPool {
	pool := &WorkerPool{
		taskChan: make(chan job.Job, maxSize*2), // buffer size
		maxSize:  maxSize,
	}

	// Khởi tạo worker goroutines
	for i := 0; i < maxSize; i++ {
		pool.wg.Add(1)
		go pool.worker(i)
	}

	return pool
}

func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()

	for j := range p.taskChan {
		atomic.AddInt32(&p.running, 1)
		log.Printf("[Worker-%d] Bắt đầu xử lý job", id)

		if err := j.RunWithRetry(context.Background()); err != nil {
			log.Printf("[Worker-%d] Job thất bại: %v", id, err)
		}

		atomic.AddInt32(&p.running, -1)
	}
}

// Submit thêm job vào queue
func (p *WorkerPool) Submit(j job.Job) bool {
	select {
	case p.taskChan <- j:
		return true
	default:
		// Queue đầy
		return false
	}
}

// GetRunningCount lấy số job đang chạy
func (p *WorkerPool) GetRunningCount() int {
	return int(atomic.LoadInt32(&p.running))
}

// Stop dừng pool
func (p *WorkerPool) Stop() {
	close(p.taskChan)
	p.wg.Wait()
}

// ============================================================================
// ENGINE - Dùng job.Hub + WorkerPool
// ============================================================================

type PubSubMessage struct {
	Topic string
	Data  string
}

type MessageHandler struct {
	Title string
	Fn    func(ctx context.Context, msg *PubSubMessage) error
}

// pubSubJobHandler adapts MessageHandler + a captured message to job.JobHandler.
type pubSubJobHandler struct {
	title string
	fn    func(ctx context.Context, msg *PubSubMessage) error
	msg   *PubSubMessage
}

func (h *pubSubJobHandler) Handle(ctx context.Context) error { return h.fn(ctx, h.msg) }
func (h *pubSubJobHandler) Type() string                     { return h.title }

type EngineWithJobHub struct {
	hub        job.Hub
	pool       *WorkerPool
	mu         sync.Mutex
	subscribed map[string][]MessageHandler
}

func NewEngineWithJobHub(poolSize int) *EngineWithJobHub {
	pool := NewWorkerPool(poolSize)

	// Tạo Hub với custom submission function
	hub := job.NewHub(func(j job.Job) bool {
		return pool.Submit(j)
	})

	return &EngineWithJobHub{
		hub:        hub,
		pool:       pool,
		subscribed: make(map[string][]MessageHandler),
	}
}

// SubscribeTopic đăng ký xử lý message từ topic
func (e *EngineWithJobHub) SubscribeTopic(topic string, handlers ...MessageHandler) {
	e.mu.Lock()
	e.subscribed[topic] = handlers
	e.mu.Unlock()

	log.Printf("✅ Subscribe topic: %s với %d handler(s)", topic, len(handlers))

	// Giả lập subscribe từ message broker
	go e.simulatePubSub(topic)
}

// simulatePubSub giả lập nhận message từ PubSub
func (e *EngineWithJobHub) simulatePubSub(topic string) {
	msgChan := make(chan *PubSubMessage)

	// Giả lập nhận message
	go func() {
		for i := 1; i <= 10; i++ {
			time.Sleep(500 * time.Millisecond)
			msgChan <- &PubSubMessage{
				Topic: topic,
				Data:  fmt.Sprintf("message-%d", i),
			}
		}
		close(msgChan)
	}()

	// Xử lý từng message
	for msg := range msgChan {
		e.mu.Lock()
		handlers := e.subscribed[topic]
		e.mu.Unlock()

		for _, h := range handlers {
			handler := h // Capture handler vào closure
			captured := msg

			// Tạo job từ handler
			jobObj, _ := e.hub.Create(
				handler.Title,
				&pubSubJobHandler{title: handler.Title, fn: handler.Fn, msg: captured},
				job.WithTimeout(5*time.Second),
				job.WithRetries([]time.Duration{1 * time.Second, 2 * time.Second}),
				job.WithJitter(0.2), // ±20% jitter
				job.WithOnRetry(func(idx int, nextDelay time.Duration, lastErr error) {
					log.Printf("⚠️  Retry job '%s' - lần %d, chờ %v, lỗi: %v",
						handler.Title, idx+1, nextDelay, lastErr)
				}),
				job.WithOnComplete(func() {
					log.Printf("✅ Hoàn thành job '%s'", handler.Title)
				}),
				job.WithOnPermanent(func(lastErr error) {
					log.Printf("❌ Job '%s' thất bại vĩnh viễn: %v", handler.Title, lastErr)
				}),
			)

			// Submit job vào hub → hub submit vào pool
			e.hub.Submit(jobObj)
		}

		// In trạng thái pool
		log.Printf("📊 Worker pool: %d job(s) đang chạy", e.pool.GetRunningCount())
	}
}

// Stop dừng engine
func (e *EngineWithJobHub) Stop() {
	e.pool.Stop()
}

// ============================================================================
// MAIN - Demo
// ============================================================================

func main() {
	fmt.Println("🚀 Job Hub + Worker Pool Example")
	fmt.Println("================================")

	engine := NewEngineWithJobHub(3) // Pool size = 3 workers

	// Handler xử lý license info
	licenseHandler := MessageHandler{
		Title: "ProcessLicense",
		Fn: func(ctx context.Context, msg *PubSubMessage) error {
			log.Printf("  🔍 Xử lý license từ message: %s", msg.Data)
			time.Sleep(2 * time.Second) // Giả lập xử lý
			return nil
		},
	}

	// Handler xử lý disconnect device
	disconnectHandler := MessageHandler{
		Title: "ProcessDisconnect",
		Fn: func(ctx context.Context, msg *PubSubMessage) error {
			log.Printf("  🔌 Xử lý disconnect từ message: %s", msg.Data)
			time.Sleep(1 * time.Second) // Giả lập xử lý
			return nil
		},
	}

	// Handler có retry logic
	webhookHandler := MessageHandler{
		Title: "SendWebhook",
		Fn: func(ctx context.Context, msg *PubSubMessage) error {
			log.Printf("  🌐 Gửi webhook cho message: %s", msg.Data)
			// Giả lập lỗi tạm thời (retry lần 1, 2 thành công)
			return nil
		},
	}

	// Subscribe topic
	engine.SubscribeTopic("license.response",
		licenseHandler,
		webhookHandler,
	)

	engine.SubscribeTopic("device.disconnect",
		disconnectHandler,
	)

	// Chờ processing
	time.Sleep(20 * time.Second)

	fmt.Println("\n🛑 Shutdown engine...")
	engine.Stop()
	fmt.Println("✨ Hoàn thành!")
}

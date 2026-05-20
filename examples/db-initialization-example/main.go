package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/jackdes93/fcontext/job"
)

// ============================================================================
// WORKER POOL
// ============================================================================

type WorkerPool struct {
	taskChan chan job.Job
	wg       sync.WaitGroup
	running  int32
	maxSize  int
}

func NewWorkerPool(maxSize int) *WorkerPool {
	pool := &WorkerPool{
		taskChan: make(chan job.Job, maxSize*2),
		maxSize:  maxSize,
	}

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
		log.Printf("[Worker-%d] ⚙️  Bắt đầu xử lý job", id)

		if err := j.RunWithRetry(context.Background()); err != nil {
			log.Printf("[Worker-%d] ❌ Job thất bại: %v", id, err)
		}

		atomic.AddInt32(&p.running, -1)
	}
}

func (p *WorkerPool) Submit(j job.Job) bool {
	select {
	case p.taskChan <- j:
		return true
	default:
		return false
	}
}

func (p *WorkerPool) GetRunningCount() int {
	return int(atomic.LoadInt32(&p.running))
}

func (p *WorkerPool) Stop() {
	close(p.taskChan)
	p.wg.Wait()
}

// ============================================================================
// DATABASE SERVICE
// ============================================================================

type DatabaseService struct {
	db *sql.DB
	mu sync.Mutex
}

func NewDatabaseService(dbPath string) (*DatabaseService, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, err
	}

	return &DatabaseService{db: db}, nil
}

// CheckAndCreateTable kiểm tra table tồn tại, nếu không thì tạo
func (ds *DatabaseService) CheckAndCreateTable(ctx context.Context) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	log.Println("🔍 Checking if 'users' table exists...")
	time.Sleep(1 * time.Second) // Giả lập I/O

	// Check table tồn tại
	var exists bool
	err := ds.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT name FROM sqlite_master 
			WHERE type='table' AND name='users'
		)`,
	).Scan(&exists)

	if err != nil {
		return fmt.Errorf("query error: %w", err)
	}

	if exists {
		log.Println("✅ Table 'users' already exists")
		return nil
	}

	log.Println("📋 Creating table 'users'...")

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		email TEXT NOT NULL UNIQUE,
		age INTEGER,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)
	`

	_, err = ds.db.ExecContext(ctx, createTableSQL)
	if err != nil {
		return fmt.Errorf("create table error: %w", err)
	}

	log.Println("✅ Table 'users' created successfully!")
	return nil
}

// InsertSampleData insert dữ liệu mẫu vào table
func (ds *DatabaseService) InsertSampleData(ctx context.Context) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	log.Println("📝 Inserting sample data...")
	time.Sleep(1 * time.Second) // Giả lập I/O

	sampleData := []map[string]interface{}{
		{
			"name":  "Alice",
			"email": "alice@example.com",
			"age":   28,
		},
		{
			"name":  "Bob",
			"email": "bob@example.com",
			"age":   32,
		},
		{
			"name":  "Charlie",
			"email": "charlie@example.com",
			"age":   25,
		},
	}

	for _, data := range sampleData {
		_, err := ds.db.ExecContext(ctx,
			`INSERT INTO users (name, email, age) VALUES (?, ?, ?)`,
			data["name"], data["email"], data["age"],
		)
		if err != nil {
			return fmt.Errorf("insert error: %w", err)
		}
		log.Printf("  ✓ Inserted: %s", data["name"])
	}

	log.Println("✅ Sample data inserted successfully!")
	return nil
}

// QueryAllUsers lấy tất cả user từ table
func (ds *DatabaseService) QueryAllUsers(ctx context.Context) error {
	ds.mu.Lock()
	rows, err := ds.db.QueryContext(ctx, "SELECT id, name, email, age FROM users")
	ds.mu.Unlock()

	if err != nil {
		return fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	log.Println("\n📊 Users in database:")
	for rows.Next() {
		var id int
		var name, email string
		var age int

		if err := rows.Scan(&id, &name, &email, &age); err != nil {
			return err
		}
		log.Printf("  [%d] %s (%s) - Age: %d", id, name, email, age)
	}

	return rows.Err()
}

// Close đóng database connection
func (ds *DatabaseService) Close() error {
	return ds.db.Close()
}

// ============================================================================
// JOB HANDLER ADAPTER
// ============================================================================

type funcHandler struct {
	jobType string
	fn      func(ctx context.Context) error
}

func (f *funcHandler) Handle(ctx context.Context) error { return f.fn(ctx) }
func (f *funcHandler) Type() string                     { return f.jobType }

// ============================================================================
// INITIALIZATION ENGINE
// ============================================================================

type InitializationEngine struct {
	hub   job.Hub
	pool  *WorkerPool
	db    *DatabaseService
	mu    sync.Mutex
	jobID int32
}

func NewInitializationEngine(poolSize int, dbService *DatabaseService) *InitializationEngine {
	pool := NewWorkerPool(poolSize)

	hub := job.NewHub(func(j job.Job) bool {
		return pool.Submit(j)
	})

	return &InitializationEngine{
		hub:  hub,
		pool: pool,
		db:   dbService,
	}
}

// RunCheckTableJob chạy job kiểm tra và tạo table
func (e *InitializationEngine) RunCheckTableJob(ctx context.Context) (job.Job, error) {
	atomic.AddInt32(&e.jobID, 1)
	jobName := fmt.Sprintf("CheckTable-%d", atomic.LoadInt32(&e.jobID))

	handler := &funcHandler{
		jobType: jobName,
		fn: func(ctxJob context.Context) error {
			return e.db.CheckAndCreateTable(ctxJob)
		},
	}

	j, err := e.hub.Create(
		jobName,
		handler,
		job.WithTimeout(10*time.Second),
		job.WithRetries([]time.Duration{1*time.Second, 2*time.Second}),
		job.WithJitter(0.2),
		job.WithOnRetry(func(idx int, nextDelay time.Duration, lastErr error) {
			log.Printf("⚠️  [%s] Retry lần %d, chờ %v - Lỗi: %v",
				jobName, idx+1, nextDelay, lastErr)
		}),
		job.WithOnComplete(func() {
			log.Printf("✅ [%s] Table check completed!", jobName)
		}),
		job.WithOnPermanent(func(lastErr error) {
			log.Printf("❌ [%s] Failed permanently: %v", jobName, lastErr)
		}),
	)

	if err != nil {
		return nil, err
	}

	e.hub.Submit(j)
	return j, nil
}

// RunInsertDataJob chạy job insert data (chạy sau check table job)
func (e *InitializationEngine) RunInsertDataJob(ctx context.Context) error {
	atomic.AddInt32(&e.jobID, 1)
	jobName := fmt.Sprintf("InsertData-%d", atomic.LoadInt32(&e.jobID))

	handler := &funcHandler{
		jobType: jobName,
		fn: func(ctxJob context.Context) error {
			return e.db.InsertSampleData(ctxJob)
		},
	}

	j, err := e.hub.Create(
		jobName,
		handler,
		job.WithTimeout(10*time.Second),
		job.WithRetries([]time.Duration{1*time.Second, 2*time.Second}),
		job.WithJitter(0.2),
		job.WithOnRetry(func(idx int, nextDelay time.Duration, lastErr error) {
			log.Printf("⚠️  [%s] Retry lần %d, chờ %v - Lỗi: %v",
				jobName, idx+1, nextDelay, lastErr)
		}),
		job.WithOnComplete(func() {
			log.Printf("✅ [%s] Data insertion completed!", jobName)
		}),
		job.WithOnPermanent(func(lastErr error) {
			log.Printf("❌ [%s] Failed permanently: %v", jobName, lastErr)
		}),
	)

	if err != nil {
		return err
	}

	e.hub.Submit(j)
	return nil
}

// Stop dừng engine
func (e *InitializationEngine) Stop() {
	e.pool.Stop()
}

// ============================================================================
// MAIN - Service Startup
// ============================================================================

func main() {
	fmt.Println("🚀 Database Initialization Service")
	fmt.Println("==================================")
	fmt.Println()

	// Khởi tạo database service
	dbService, err := NewDatabaseService("./app.db")
	if err != nil {
		log.Fatalf("❌ Failed to initialize database: %v", err)
	}
	defer dbService.Close()

	// Khởi tạo initialization engine
	engine := NewInitializationEngine(2, dbService)
	defer engine.Stop()

	fmt.Println("📍 Phase 1: Checking and creating tables...")
	fmt.Println()

	// Job 1: Check table
	checkTableJob, err := engine.RunCheckTableJob(context.Background())
	if err != nil {
		log.Fatalf("❌ Failed to create check table job: %v", err)
	}

	// Chờ check table job hoàn thành
	// Đợi job kết thúc bằng cách chờ trạng thái
	for {
		state := checkTableJob.State()
		if state == job.StateCompleted || state == job.StateRetryFailed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Kiểm tra kết quả
	if checkTableJob.State() != job.StateCompleted {
		log.Fatalf("❌ Check table job failed: %v", checkTableJob.LastError())
	}

	fmt.Println()
	fmt.Println("📍 Phase 2: Inserting sample data...")
	fmt.Println()

	// Job 2: Insert data (chạy sau job 1 thành công)
	err = engine.RunInsertDataJob(context.Background())
	if err != nil {
		log.Fatalf("❌ Failed to create insert data job: %v", err)
	}

	// Chờ insert job hoàn thành
	time.Sleep(5 * time.Second)

	fmt.Println()
	fmt.Println("📍 Phase 3: Querying data...")
	fmt.Println()

	// Query all users
	if err := dbService.QueryAllUsers(context.Background()); err != nil {
		log.Fatalf("❌ Query failed: %v", err)
	}

	fmt.Println("\n✨ Service initialization completed successfully!")
}

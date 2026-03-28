# Giải Thích Chi Tiết Về `Order()` trong sctx

## Khái Niệm Cơ Bản

`Order()` là một phương thức trong interface `Component` quyết định **thứ tự khởi tạo các component**. Component có giá trị `Order()` thấp hơn sẽ được khởi tạo trước.

```go
type Component interface {
	ID() string
	InitFlags()
	Activate(ctx context.Context, service ServiceContext) error
	Stop(ctx context.Context) error
	Order() int  // ← Xác định thứ tự khởi tạo
}
```

## Ví Dụ Cụ Thể

### Scenario: Microservice với Database → Cache → API

```go
// 1. Database phải khởi động trước
type DatabaseComponent struct{}

func (d *DatabaseComponent) Order() int {
	return 10  // Khởi động TRƯỚC TIÊN
}

// 2. Cache có thể khởi động sau Database
type CacheComponent struct{}

func (c *CacheComponent) Order() int {
	return 20  // Khởi động THỨ HAI
}

// 3. API phụ thuộc vào Database và Cache
type APIComponent struct{}

func (a *APIComponent) Order() int {
	return 100  // Khởi động CÓ SAU CÙNG
}
```

**Thứ tự khởi tạo**:
```
1. Database (Order: 10)
   ↓
2. Cache (Order: 20)
   ↓
3. API (Order: 100)
```

## Tại Sao Order Lại Quan Trọng?

### Vấn đề Nếu Không Có Order

```go
// ❌ BUG: API khởi động trước Database
type APIComponent struct {
	db *sql.DB
}

func (a *APIComponent) Activate(ctx context.Context, sv ServiceContext) error {
	db, ok := sctx.GetAs[*sql.DB](sv, "database")
	if !ok {
		return fmt.Errorf("Database not available yet!") // LỖI!
	}
	a.db = db
	return nil
}
```

### Giải pháp Với Order

```go
// ✅ ĐÚNG: Xử lý thứ tự phụ thuộc
type APIComponent struct {
	db *sql.DB
}

func (a *APIComponent) Activate(ctx context.Context, sv ServiceContext) error {
	db, ok := sctx.GetAs[*sql.DB](sv, "database")
	if !ok {
		return fmt.Errorf("Database not available!")
	}
	a.db = db
	return nil
}

func (a *APIComponent) Order() int {
	return 100  // Chắc chắn khởi động SAU Database (Order: 10)
}
```

## Các Mức Order Thường Dùng

```go
const (
	OrderConfig      = 1      // Cấu hình phải đầu tiên
	OrderDatabase    = 10     // Database là nền tảng
	OrderCache       = 20     // Cache phụ thuộc vào config
	OrderMigrations  = 30     // Migration phụ thuộc vào database
	OrderAPI         = 100    // API phụ thuộc vào mọi thứ
	OrderWebServer   = 110    // Web server cuối cùng
	OrderDefault     = 100    // Mặc định nếu không implement Order()
)
```

## Cơ Chế Hoạt Động Bên Trong

### Từ context.go

```go
// Sắp xếp các component theo Order() trước khi khởi động
sort.SliceStable(s.components, func(i, j int) bool {
	return componentOrder(s.components[i]) < componentOrder(s.components[j])
})

// Khởi động lần lượt
for _, c := range s.components {
	if err := c.Activate(ctx, s); err != nil {
		// Nếu lỗi, quay lại stop các component đã khởi động
		for k := len(activated) - 1; k >= 0; k-- {
			_ = activated[k].Stop(ctx)
		}
		return err
	}
	activated = append(activated, c)
}
```

## Shutdown Là Thứ Tự Ngược Lại

Khi shutdown, các component được dừng **theo thứ tự ngược lại**:

```go
// Dừng từ cuối về đầu (thứ tự ngược)
for i := len(s.components) - 1; i >= 0; i-- {
	if err := s.components[i].Stop(ctx); err != nil {
		s.logger.Error("Stop %s error: %v", s.components[i].ID(), err)
	}
}
```

**Ví dụ shutdown**:
```
3. API dừng (Order: 100)
   ↓
2. Cache dừng (Order: 20)
   ↓
1. Database dừng (Order: 10)
```

## Lỗi Rollback Đơn Giản

Nếu **một component khởi tạo thất bại**, các component đã khởi tạo trước sẽ **tự động dừng**:

```go
// Senarior:
// Database (10) ✓ Khởi tạo thành công
// Cache (20) ✗ LỖI
// API (100) - Chưa chạy

// Kết quả:
// → Database được dừng tự động (rollback)
// → Cache không được khởi tạo
// → API không được khởi tạo
// → Trả về lỗi
```

## Thực Hành: So Sánh Order

```go
func main() {
	app := sctx.New(
		sctx.WithName("service"),
		sctx.WithComponent(&DatabaseComponent{}),    // Order: 10
		sctx.WithComponent(&APIComponent{}),         // Order: 100
		sctx.WithComponent(&CacheComponent{}),       // Order: 20
	)

	// ✅ Sẽ khởi tạo theo thứ tự:
	// 1. Database (10)
	// 2. Cache (20)
	// 3. API (100)
	// Không phải theo thứ tự thêm!

	err := app.Load()
	if err != nil {
		panic(err)
	}
}
```

## Khi Nào Không Cần Order() Cao?

```go
// Component độc lập (không phụ thuộc gì)
type LoggerComponent struct{}

func (l *LoggerComponent) Order() int {
	return 1  // Sớm nhất - không phụ thuộc resource khác
}

// Component tùy chọn
type MetricsComponent struct{}

func (m *MetricsComponent) Order() int {
	return 150  // Muộn - có thể khởi tạo sau hệ thống chính
}
```

## Tóm Tắt

| Khái Niệm | Chi Tiết |
|-----------|---------|
| **Giá trị thấp** | Khởi tạo sớm (ví dụ: 10) |
| **Giá trị cao** | Khởi tạo muộn (ví dụ: 100) |
| **Mặc định** | 100 nếu không implement Order() |
| **Shutdown** | Thứ tự NGƯỢC (từ cao xuống thấp) |
| **Lỗi rollback** | Tự động dừng component khởi tạo trước |

## Quy Tắc Vàng

1. **Component phụ thuộc** → Order cao hơn
2. **Component độc lập** → Order thấp hơn
3. **Luôn kiểm tra phụ thuộc** trước khi sử dụng component khác
4. **Shutdown tự động xảy ra** theo thứ tự ngược
5. **Lỗi tự động rollback** - không cần lo lắng cleanup

Hiểu rõ `Order()` giúp bạn **tránh lỗi phụ thuộc** và **quản lý lifecycle component** một cách chính xác! 🎯

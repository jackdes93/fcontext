package job

import "errors"

// Sentinel errors cho package job — caller dùng errors.Is() để phân biệt loại lỗi.
var (
	ErrHubStopped      = errors.New("job: hub is stopped")
	ErrHandlerNil      = errors.New("job: handler cannot be nil")
	ErrJobTypeEmpty    = errors.New("job: job type cannot be empty")
	ErrInvalidState    = errors.New("job: cannot execute in current state")
)

.PHONY: run clean help install-deps

help:
	@echo "Database Initialization Example"
	@echo "==============================="
	@echo ""
	@echo "Commands:"
	@echo "  make install-deps - Cài đặt dependencies"
	@echo "  make run          - Chạy example"
	@echo "  make clean        - Xóa database + binaries"

install-deps:
	@echo "📦 Installing dependencies..."
	@go get github.com/mattn/go-sqlite3
	@go mod tidy
	@echo "✅ Dependencies installed"

run: install-deps
	@echo "🚀 Running Database Initialization Example..."
	@go run main.go

clean:
	@rm -f app.db
	@rm -f db-initialization-example
	@echo "✨ Cleaned"

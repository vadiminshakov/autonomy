.PHONY: test build lint install help

# Build settings
BINARY_NAME=autonomy
BUILD_DIR=bin

# Default target
all: test build

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) .

# Run tests
test:
	@echo "Running tests..."
	@go test -v -race ./...

# Run linter
lint:
	@echo "Running linter..."
	@golangci-lint run

# Install binary to system
install: build
	@echo "Installing $(BINARY_NAME)..."
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

# Development setup
dev-setup:
	@echo "Setting up development environment..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Help
help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  test         - Run tests"
	@echo "  lint         - Run golangci-lint"
	@echo "  install      - Install binary to system"
	@echo "  dev-setup    - Setup development environment"
	@echo "  help         - Show this help" 
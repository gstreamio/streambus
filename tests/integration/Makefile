.PHONY: test lint clean test-short test-unit test-integration check deps

# Run all tests
test:
	@echo "Running all tests..."
	@go test -v -race ./...

# Run short tests (skip integration tests that need external broker)
test-short:
	@echo "Running short tests..."
	@go test -v -race -short ./...

# Run unit tests only
test-unit: test-short

# Run linter
lint:
	@echo "Running linter..."
	@golangci-lint run --timeout=10m

# Clean build artifacts and caches
clean:
	@echo "Cleaning..."
	@go clean -testcache
	@rm -rf bin/

# Run all checks (lint + test)
check: lint test-short

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Run integration tests (requires StreamBus broker at localhost:9092)
test-integration:
	@echo "Running integration tests (requires StreamBus broker)..."
	@go test -v ./tests/integration/...

.DEFAULT_GOAL := test-short

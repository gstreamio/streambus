# StreamBus Makefile

.PHONY: help build test clean lint fmt vet install benchmark docker run

# Variables
BINARY_NAME=streambus
BROKER_BINARY=streambus-broker
CLI_BINARY=streambus-cli
VERSION?=dev
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOVET=$(GOCMD) vet
GOFMT=$(GOCMD) fmt

# Directories
BUILD_DIR=bin
COVERAGE_DIR=coverage

help: ## Display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

all: clean lint test build ## Run clean, lint, test, and build

build: ## Build all binaries
	@echo "Building $(BROKER_BINARY)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BROKER_BINARY) ./cmd/broker
	@echo "Building $(CLI_BINARY)..."
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY) ./cmd/cli
	@echo "Build complete!"

build-linux: ## Build Linux binaries
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BROKER_BINARY)-linux-amd64 ./cmd/broker
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-linux-amd64 ./cmd/cli

build-darwin: ## Build macOS binaries
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BROKER_BINARY)-darwin-amd64 ./cmd/broker
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BROKER_BINARY)-darwin-arm64 ./cmd/broker
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-darwin-amd64 ./cmd/cli
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-darwin-arm64 ./cmd/cli

install: ## Install binaries to $GOPATH/bin
	@echo "Installing binaries..."
	$(GOCMD) install $(LDFLAGS) ./cmd/broker
	$(GOCMD) install $(LDFLAGS) ./cmd/cli

test: ## Run tests
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.txt -covermode=atomic ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@mkdir -p $(COVERAGE_DIR)
	$(GOTEST) -v -race -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report generated at $(COVERAGE_DIR)/coverage.html"

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	$(GOTEST) -v -tags=integration ./...

benchmark: ## Run benchmarks
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem -run=^$$ ./...

benchmark-compare: ## Run benchmarks and compare with baseline
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem -run=^$$ ./... | tee benchmarks/new.txt
	@if [ -f benchmarks/baseline.txt ]; then \
		benchstat benchmarks/baseline.txt benchmarks/new.txt; \
	else \
		echo "No baseline found. Run 'make benchmark-baseline' first."; \
	fi

benchmark-baseline: ## Set current benchmark as baseline
	@echo "Setting benchmark baseline..."
	@mkdir -p benchmarks
	$(GOTEST) -bench=. -benchmem -run=^$$ ./... | tee benchmarks/baseline.txt

lint: ## Run linters
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin"; \
	fi

fmt: ## Format code
	@echo "Formatting code..."
	$(GOFMT) ./...

vet: ## Run go vet
	@echo "Running go vet..."
	$(GOVET) ./...

tidy: ## Tidy go modules
	@echo "Tidying modules..."
	$(GOMOD) tidy

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOGET) -v -t -d ./...
	$(GOMOD) download

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -rf $(COVERAGE_DIR)
	@rm -f coverage.txt
	@rm -f *.prof

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t streambus:$(VERSION) .
	docker tag streambus:$(VERSION) streambus:latest

docker-build-dev: ## Build Docker dev image
	@echo "Building Docker dev image..."
	docker build -f Dockerfile.dev -t streambus:dev .

docker-push: ## Push Docker image
	@echo "Pushing Docker image..."
	docker push streambus:$(VERSION)
	docker push streambus:latest

run-broker: build ## Run broker locally
	@echo "Starting broker..."
	$(BUILD_DIR)/$(BROKER_BINARY) --config config/broker.yaml

run-cluster: ## Run 3-node cluster with docker-compose
	@echo "Starting cluster..."
	docker-compose up -d

stop-cluster: ## Stop docker-compose cluster
	@echo "Stopping cluster..."
	docker-compose down

proto: ## Generate protobuf code
	@echo "Generating protobuf code..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		pkg/protocol/*.proto

mocks: ## Generate mocks
	@echo "Generating mocks..."
	@if command -v mockery >/dev/null 2>&1; then \
		mockery --all --dir pkg --output pkg/mocks; \
	else \
		echo "mockery not installed. Install with: go install github.com/vektra/mockery/v2@latest"; \
	fi

docs: ## Build documentation
	@echo "Building documentation..."
	@if command -v mkdocs >/dev/null 2>&1; then \
		mkdocs build; \
	else \
		echo "mkdocs not installed. Install with: pip install mkdocs"; \
	fi

docs-serve: ## Serve documentation locally
	@echo "Serving documentation at http://localhost:8000..."
	@if command -v mkdocs >/dev/null 2>&1; then \
		mkdocs serve; \
	else \
		echo "mkdocs not installed. Install with: pip install mkdocs"; \
	fi

tools: ## Install development tools
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/vektra/mockery/v2@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install golang.org/x/perf/cmd/benchstat@latest

version: ## Print version
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

.DEFAULT_GOAL := help

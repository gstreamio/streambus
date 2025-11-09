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

build-operator: ## Build operator binary
	@echo "Building operator..."
	@mkdir -p $(BUILD_DIR)
	cd deploy/kubernetes/operator && CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o ../../../$(BUILD_DIR)/streambus-operator ./main.go
	@echo "Operator build complete!"

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

benchmark: ## Run all benchmarks
	@echo "Running all benchmarks..."
	$(GOTEST) -bench=. -benchmem -run=^$$ ./...

benchmark-storage: ## Run storage layer benchmarks
	@echo "Running storage benchmarks..."
	$(GOTEST) -bench=. -benchmem -benchtime=2s -run=^$$ ./pkg/storage/...

benchmark-protocol: ## Run protocol layer benchmarks
	@echo "Running protocol benchmarks..."
	$(GOTEST) -bench=. -benchmem -benchtime=2s -run=^$$ ./pkg/protocol/...

benchmark-client: ## Run client layer benchmarks
	@echo "Running client benchmarks..."
	$(GOTEST) -bench=. -benchmem -benchtime=2s -run=^$$ ./pkg/client/...

benchmark-server: ## Run server layer benchmarks
	@echo "Running server benchmarks..."
	$(GOTEST) -bench=. -benchmem -benchtime=2s -run=^$$ ./pkg/server/...

benchmark-full: ## Run comprehensive benchmark suite
	@echo "=== StreamBus Benchmark Suite ==="
	@echo ""
	@echo "=== Storage Layer ==="
	@$(GOTEST) -bench=. -benchmem -benchtime=2s -run=^$$ ./pkg/storage/... 2>&1 | grep -E "(^Benchmark|ns/op)"
	@echo ""
	@echo "=== Protocol Layer ==="
	@$(GOTEST) -bench=. -benchmem -benchtime=2s -run=^$$ ./pkg/protocol/... 2>&1 | grep -E "(^Benchmark|ns/op)"
	@echo ""
	@echo "=== Client Layer (End-to-End) ==="
	@$(GOTEST) -bench=. -benchmem -benchtime=2s -run=^$$ ./pkg/client/... 2>&1 | grep -E "(^Benchmark|ns/op)" || true
	@echo ""
	@echo "=== Benchmark Complete ==="

benchmark-report: ## Generate formatted benchmark report
	@echo "Generating benchmark report..."
	@mkdir -p benchmarks
	@echo "# StreamBus Performance Benchmark Report" > benchmarks/report.md
	@echo "" >> benchmarks/report.md
	@echo "Generated: $$(date)" >> benchmarks/report.md
	@echo "" >> benchmarks/report.md
	@echo "## Storage Layer" >> benchmarks/report.md
	@echo '```' >> benchmarks/report.md
	@$(GOTEST) -bench=. -benchmem -benchtime=2s -run=^$$ ./pkg/storage/... 2>&1 | grep -E "(^Benchmark|^goos|^goarch|^cpu)" >> benchmarks/report.md
	@echo '```' >> benchmarks/report.md
	@echo "" >> benchmarks/report.md
	@echo "## Protocol Layer" >> benchmarks/report.md
	@echo '```' >> benchmarks/report.md
	@$(GOTEST) -bench=. -benchmem -benchtime=2s -run=^$$ ./pkg/protocol/... 2>&1 | grep -E "(^Benchmark|^goos|^goarch|^cpu)" >> benchmarks/report.md
	@echo '```' >> benchmarks/report.md
	@echo "" >> benchmarks/report.md
	@echo "## Client Layer" >> benchmarks/report.md
	@echo '```' >> benchmarks/report.md
	@$(GOTEST) -bench=. -benchmem -benchtime=2s -run=^$$ ./pkg/client/... 2>&1 | grep -E "(^Benchmark|^goos|^goarch|^cpu)" >> benchmarks/report.md || true
	@echo '```' >> benchmarks/report.md
	@echo "Report saved to benchmarks/report.md"

benchmark-compare: ## Run benchmarks and compare with baseline
	@echo "Running benchmarks..."
	@mkdir -p benchmarks
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

docker-build: docker-broker docker-operator ## Build all Docker images

docker-broker: ## Build broker Docker image
	@echo "Building broker Docker image..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t streambus/broker:$(VERSION) \
		-t streambus/broker:latest \
		-f Dockerfile .

docker-operator: ## Build operator Docker image
	@echo "Building operator Docker image..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t streambus/operator:$(VERSION) \
		-t streambus/operator:latest \
		-f deploy/kubernetes/operator/Dockerfile \
		deploy/kubernetes/operator

docker-build-dev: ## Build Docker dev image
	@echo "Building Docker dev image..."
	docker build -f Dockerfile.dev -t streambus:dev .

docker-push: ## Push all Docker images
	@echo "Pushing broker image..."
	docker push streambus/broker:$(VERSION)
	docker push streambus/broker:latest
	@echo "Pushing operator image..."
	docker push streambus/operator:$(VERSION)
	docker push streambus/operator:latest

run-broker: build ## Run broker locally
	@echo "Starting broker..."
	$(BUILD_DIR)/$(BROKER_BINARY) --config config/broker.yaml

run-cluster: ## Run 3-node cluster with docker-compose
	@echo "Starting cluster..."
	docker-compose up -d

stop-cluster: ## Stop docker-compose cluster
	@echo "Stopping cluster..."
	docker-compose down -v

restart-cluster: stop-cluster run-cluster ## Restart docker-compose cluster

cluster-logs: ## Show cluster logs
	docker-compose logs -f

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

k8s-install-operator: ## Install operator to Kubernetes cluster
	@echo "Installing operator..."
	kubectl apply -f deploy/kubernetes/operator/config/crd/
	kubectl apply -f deploy/kubernetes/operator/config/rbac/
	kubectl apply -f deploy/kubernetes/operator/config/manager/

k8s-uninstall-operator: ## Uninstall operator from Kubernetes cluster
	@echo "Uninstalling operator..."
	kubectl delete -f deploy/kubernetes/operator/config/manager/ || true
	kubectl delete -f deploy/kubernetes/operator/config/rbac/ || true
	kubectl delete -f deploy/kubernetes/operator/config/crd/ || true

k8s-deploy-minimal: ## Deploy minimal cluster to Kubernetes
	@echo "Deploying minimal cluster..."
	kubectl apply -f deploy/kubernetes/examples/minimal-cluster.yaml

helm-package: ## Package Helm chart
	@echo "Packaging Helm chart..."
	helm package deploy/kubernetes/helm/streambus-operator

helm-install: ## Install Helm chart locally
	@echo "Installing Helm chart..."
	helm install streambus-operator deploy/kubernetes/helm/streambus-operator \
		--namespace streambus-system \
		--create-namespace

helm-uninstall: ## Uninstall Helm chart
	@echo "Uninstalling Helm chart..."
	helm uninstall streambus-operator --namespace streambus-system

version: ## Print version
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

.DEFAULT_GOAL := help

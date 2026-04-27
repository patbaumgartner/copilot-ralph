.PHONY: build test lint clean tidy install help all vuln release-snapshot release-check

# Binary name
BINARY_NAME=ralph

# Build output directory
BUILD_DIR=bin

# Version information (can be overridden; derived from the most recent git tag)
VERSION?=$(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.0.0-dev")
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GO_VERSION?=$(shell go version | awk '{print $$3}')

# Linker flags for version information
LDFLAGS=-ldflags "-X github.com/patbaumgartner/copilot-ralph/pkg/version.Version=$(VERSION) \
                  -X github.com/patbaumgartner/copilot-ralph/pkg/version.Commit=$(COMMIT) \
                  -X github.com/patbaumgartner/copilot-ralph/pkg/version.BuildDate=$(BUILD_DATE) \
                  -X github.com/patbaumgartner/copilot-ralph/pkg/version.GoVersion=$(GO_VERSION)"

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the ralph binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/ralph
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

install: ## Install ralph to $GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	go install $(LDFLAGS) ./cmd/ralph
	@echo "Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

test: ## Run tests
	@echo "Running tests..."
	go test -v -race -cover ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

bench: ## Run benchmarks
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

lint: ## Run linter
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with:"; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

fmt: ## Format code
	@echo "Formatting code..."
	gofmt -w -s .
	@echo "Code formatted"

vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

tidy: ## Tidy dependencies
	@echo "Tidying dependencies..."
	go mod tidy
	go mod verify
	@echo "Dependencies tidied"

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR) dist
	rm -f coverage.out coverage.html coverage_*
	go clean
	@echo "Clean complete"

vuln: ## Scan for known vulnerabilities (govulncheck)
	@if ! command -v govulncheck >/dev/null 2>&1; then \
		echo "Installing govulncheck..."; \
		go install golang.org/x/vuln/cmd/govulncheck@v1.3.0; \
	fi
	@govulncheck ./...

release-snapshot: ## Build a goreleaser snapshot into ./dist (no publish)
	@if ! command -v goreleaser >/dev/null 2>&1; then \
		echo "goreleaser not installed: https://goreleaser.com/install/"; exit 1; \
	fi
	goreleaser release --clean --snapshot --skip=publish

release-check: ## Validate the goreleaser configuration
	@if ! command -v goreleaser >/dev/null 2>&1; then \
		echo "goreleaser not installed: https://goreleaser.com/install/"; exit 1; \
	fi
	goreleaser check

all: tidy fmt vet lint test build ## Run all checks and build

# Development helpers
dev-deps: ## Install development dependencies
	@echo "Installing development dependencies..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.11.4
	go install golang.org/x/vuln/cmd/govulncheck@v1.3.0
	go install golang.org/x/tools/cmd/goimports@v0.27.0
	@echo "Development dependencies installed"

run: build ## Build and run ralph
	@$(BUILD_DIR)/$(BINARY_NAME)

run-version: build ## Build and run ralph version
	@$(BUILD_DIR)/$(BINARY_NAME) version

# Quick commands
.PHONY: b t l c
b: build   ## Alias for build
t: test    ## Alias for test
l: lint    ## Alias for lint
c: clean   ## Alias for clean

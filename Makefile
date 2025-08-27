# Blockchain Health Dynamic Upstream Module for Caddy
# Makefile for building, testing, and development

.PHONY: help build test test-all test-coverage test-integration benchmark clean deps lint fmt vet \
        xcaddy-build xcaddy-install docker-build docker-test release-check \
        example-start example-stop example-restart

# Default target
help: ## Show this help message
	@echo "Blockchain Health Dynamic Upstream Module for Caddy"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build variables
GO_VERSION := 1.21
MODULE_NAME := github.com/chalabi2/caddy-blockchain-health
BINARY_NAME := caddy-blockchain-health
COVERAGE_FILE := coverage.out
BENCHMARK_FILE := benchmark.out

# Build targets
build: ## Build the Go module
	@echo "Building Go module..."
	go build -v ./...

test: ## Run unit tests
	@echo "Running unit tests..."
	go test -v ./...

test-all: test test-integration ## Run all tests (unit + integration)

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	go tool cover -html=$(COVERAGE_FILE) -o coverage.html
	@echo "Coverage report generated: coverage.html"
	go tool cover -func=$(COVERAGE_FILE)

test-integration: ## Run integration tests with real blockchain nodes (requires Docker)
	@echo "Running integration tests..."
	@if command -v docker >/dev/null 2>&1; then \
		docker-compose -f test/docker-compose.yml up -d; \
		sleep 10; \
		go test -v -tags=integration ./test/integration/...; \
		docker-compose -f test/docker-compose.yml down; \
	else \
		echo "Docker not found. Skipping integration tests."; \
	fi

benchmark: ## Run performance benchmarks
	@echo "Running benchmarks..."
	go test -bench=. -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof ./... | tee $(BENCHMARK_FILE)
	@echo "Benchmark results saved to: $(BENCHMARK_FILE)"

# Code quality targets
lint: ## Run golangci-lint
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Installing..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
		golangci-lint run ./...; \
	fi

fmt: ## Format Go code
	@echo "Formatting code..."
	go fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	else \
		echo "goimports not found. Installing..."; \
		go install golang.org/x/tools/cmd/goimports@latest; \
		goimports -w .; \
	fi

vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

# Dependencies
deps: ## Install/update dependencies
	@echo "Installing dependencies..."
	go mod download
	go mod tidy
	go mod verify

deps-tools: ## Install development tools
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest

# xcaddy targets
xcaddy-build: ## Build Caddy with this plugin using xcaddy
	@echo "Building Caddy with blockchain health plugin..."
	@if command -v xcaddy >/dev/null 2>&1; then \
		xcaddy build --with $(MODULE_NAME)=.; \
	else \
		echo "xcaddy not found. Installing..."; \
		go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest; \
		xcaddy build --with $(MODULE_NAME)=.; \
	fi

xcaddy-install: xcaddy-build ## Build and install Caddy binary to $GOPATH/bin
	@echo "Installing Caddy binary..."
	mv caddy $(shell go env GOPATH)/bin/caddy-blockchain
	@echo "Caddy with blockchain health plugin installed as: caddy-blockchain"

# Docker targets
docker-build: ## Build Docker image for testing
	@echo "Building Docker image..."
	docker build -t caddy-blockchain-health:latest -f Dockerfile .

docker-test: docker-build ## Run tests in Docker container
	@echo "Running tests in Docker..."
	docker run --rm -v $(PWD):/workspace -w /workspace caddy-blockchain-health:latest make test

# Example targets
example-start: xcaddy-build ## Start example configuration
	@echo "Starting example Caddy configuration..."
	@if [ -f ./caddy ]; then \
		./caddy run --config example_configs/Caddyfile --adapter caddyfile; \
	else \
		echo "Caddy binary not found. Run 'make xcaddy-build' first."; \
	fi

example-stop: ## Stop running Caddy instance
	@echo "Stopping Caddy..."
	@pkill -f "caddy run" || echo "No Caddy process found"

example-restart: example-stop example-start ## Restart example configuration

example-validate: ## Validate example configurations
	@echo "Validating Caddyfile configuration..."
	@if [ -f ./caddy ]; then \
		./caddy validate --config example_configs/Caddyfile --adapter caddyfile; \
		./caddy validate --config example_configs/config.json --adapter json; \
	else \
		echo "Caddy binary not found. Run 'make xcaddy-build' first."; \
	fi

# Release targets
release-check: test-all lint vet ## Run all checks before release
	@echo "Running pre-release checks..."
	@echo "✅ All tests passed"
	@echo "✅ Linting passed"
	@echo "✅ Vet checks passed"
	@echo "Ready for release!"

version: ## Show current version from go.mod
	@echo "Module: $(MODULE_NAME)"
	@echo "Go version: $(shell go version)"
	@echo "Module version: $(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')"

# Cleanup targets
clean: ## Clean build artifacts and cache
	@echo "Cleaning up..."
	go clean -cache -testcache -modcache
	rm -f caddy caddy.exe $(COVERAGE_FILE) coverage.html $(BENCHMARK_FILE)
	rm -f cpu.prof mem.prof
	rm -rf dist/

clean-all: clean ## Clean everything including vendor directory
	rm -rf vendor/

# Development helpers
dev-setup: deps deps-tools ## Set up development environment
	@echo "Development environment ready!"
	@echo "Available commands:"
	@echo "  make test           - Run tests"
	@echo "  make xcaddy-build   - Build Caddy with plugin"
	@echo "  make example-start  - Start example config"
	@echo "  make lint           - Run linter"

watch: ## Watch for changes and run tests (requires entr)
	@if command -v entr >/dev/null 2>&1; then \
		find . -name "*.go" | entr -c make test; \
	else \
		echo "entr not found. Install with: brew install entr (macOS) or apt-get install entr (Ubuntu)"; \
	fi

# Performance testing
perf-test: xcaddy-build ## Run performance tests with real load
	@echo "Starting performance test setup..."
	@echo "This will start test servers and run load tests"
	@./scripts/perf-test.sh

# Documentation
docs: ## Generate documentation
	@echo "Generating documentation..."
	@if command -v godoc >/dev/null 2>&1; then \
		echo "Starting godoc server at http://localhost:6060"; \
		godoc -http=:6060; \
	else \
		echo "godoc not found. Installing..."; \
		go install golang.org/x/tools/cmd/godoc@latest; \
		echo "Starting godoc server at http://localhost:6060"; \
		godoc -http=:6060; \
	fi

# Git hooks
install-hooks: ## Install git pre-commit hooks
	@echo "Installing git hooks..."
	@echo '#!/bin/sh\nmake fmt lint test' > .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed"

# CI/CD helpers
ci-test: deps test-coverage lint vet ## Run all CI tests
	@echo "Running CI test suite..."

# Show environment info
env: ## Show development environment information
	@echo "Development Environment Information:"
	@echo "====================================="
	@echo "Go version: $(shell go version)"
	@echo "Module: $(MODULE_NAME)"
	@echo "GOPATH: $(shell go env GOPATH)"
	@echo "GOOS: $(shell go env GOOS)"
	@echo "GOARCH: $(shell go env GOARCH)"
	@echo "Git branch: $(shell git branch --show-current 2>/dev/null || echo 'unknown')"
	@echo "Git commit: $(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
	@echo ""
	@echo "Available tools:"
	@command -v xcaddy >/dev/null 2>&1 && echo "✅ xcaddy" || echo "❌ xcaddy (run 'make deps-tools')"
	@command -v golangci-lint >/dev/null 2>&1 && echo "✅ golangci-lint" || echo "❌ golangci-lint (run 'make deps-tools')"
	@command -v docker >/dev/null 2>&1 && echo "✅ docker" || echo "❌ docker (optional for integration tests)"
# Makefile for tinkerdown CLI

# Variables
BINARY_NAME=tinkerdown
INSTALL_PATH=$(HOME)/go/bin
CMD_PATH=./cmd/tinkerdown
GO=go
GOFLAGS=-ldflags="-s -w"

# Get version from git tag or use dev
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: help
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build-client
build-client: ## Build the TypeScript client
	@echo "Building client..."
	@cd client && npm ci && npm run build
	@echo "Copying client assets..."
	@mkdir -p internal/assets/client
	@cp client/dist/tinkerdown-client.browser.* internal/assets/client/
	@echo "Client build complete"

.PHONY: build
build: build-client ## Build the tinkerdown binary
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(LDFLAGS) -o $(BINARY_NAME) $(CMD_PATH)
	@echo "Build complete: ./$(BINARY_NAME)"

.PHONY: install
install: build ## Build and install tinkerdown to ~/go/bin
	@echo "Installing $(BINARY_NAME) to $(INSTALL_PATH)..."
	@mkdir -p $(INSTALL_PATH)
	@install -m 755 $(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Installation complete: $(INSTALL_PATH)/$(BINARY_NAME)"
	@echo "Run 'tinkerdown version' to verify"

.PHONY: uninstall
uninstall: ## Remove tinkerdown from ~/go/bin
	@echo "Uninstalling $(BINARY_NAME) from $(INSTALL_PATH)..."
	@rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Uninstall complete"

.PHONY: clean
clean: ## Remove build artifacts
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY_NAME)
	@rm -rf client/dist
	@rm -rf internal/assets/client
	@echo "Clean complete"

.PHONY: test
test: ## Run all tests
	@echo "Running tests..."
	$(GO) test -v ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: fmt
fmt: ## Format Go code
	@echo "Formatting code..."
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	$(GO) vet ./...

.PHONY: lint
lint: vet ## Run linters
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping..."; \
		echo "Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

.PHONY: deps
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

.PHONY: dev
dev: build ## Build and run in development mode
	@echo "Running $(BINARY_NAME) in development mode..."
	./$(BINARY_NAME) serve

.PHONY: all
all: clean deps fmt vet test build ## Run all checks and build

.DEFAULT_GOAL := help

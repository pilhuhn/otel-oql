# Makefile for otel-oql project
# Compatible with GNU Make (Linux, macOS)

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Binary names
BINARY_OTEL_OQL=otel-oql
BINARY_OQL_CLI=oql-cli
BINARY_SEND_TEST_DATA=send-test-data

# Build directories
BUILD_DIR=bin
CMD_DIR=cmd

# Version information (can be overridden)
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME?=$(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Detect OS for cross-compilation
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
    PLATFORM=linux
endif
ifeq ($(UNAME_S),Darwin)
    PLATFORM=darwin
endif

# Default target
.PHONY: all
all: build

# Build all binaries
.PHONY: build
build: otel-oql oql-cli

# Build otel-oql service
.PHONY: otel-oql
otel-oql:
	@echo "Building otel-oql..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_OTEL_OQL) ./$(CMD_DIR)/otel-oql

# Build oql-cli tool
.PHONY: oql-cli
oql-cli:
	@echo "Building oql-cli..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_OQL_CLI) ./$(CMD_DIR)/oql-cli

# Build send-test-data tool
.PHONY: send-test-data
send-test-data:
	@echo "Building send-test-data..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_SEND_TEST_DATA) ./$(CMD_DIR)/send-test-data

# Build all tools (including test data generator)
.PHONY: build-all
build-all: otel-oql oql-cli send-test-data

# Install binaries to $GOPATH/bin
.PHONY: install
install:
	@echo "Installing binaries to $(GOPATH)/bin..."
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH)/bin/$(BINARY_OTEL_OQL) ./$(CMD_DIR)/otel-oql
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH)/bin/$(BINARY_OQL_CLI) ./$(CMD_DIR)/oql-cli

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests with race detector
.PHONY: test-race
test-race:
	@echo "Running tests with race detector..."
	$(GOTEST) -v -race ./...

# Run specific package tests
.PHONY: test-promql
test-promql:
	@echo "Running PromQL tests..."
	$(GOTEST) -v ./pkg/promql/...

.PHONY: test-logql
test-logql:
	@echo "Running LogQL tests..."
	$(GOTEST) -v ./pkg/logql/...

.PHONY: test-integration
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v ./pkg/integration/...

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

# Run go vet
.PHONY: vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

# Run go mod tidy
.PHONY: tidy
tidy:
	@echo "Tidying go modules..."
	$(GOMOD) tidy

# Download dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download

# Update dependencies to latest versions
.PHONY: deps-update
deps-update:
	@echo "Updating dependencies to latest versions..."
	$(GOCMD) get -u ./...
	$(GOMOD) tidy
	@echo "Dependencies updated. Review changes with 'git diff go.mod go.sum'"

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Deep clean (includes mod cache)
.PHONY: clean-all
clean-all: clean
	@echo "Deep cleaning..."
	$(GOMOD) clean -cache

# Container image targets
.PHONY: images
images:
	@echo "Building and pushing multi-arch container images..."
	./c-build.sh

# Build container images only (no push)
.PHONY: images-build
images-build:
	@echo "Building multi-arch container images (local only)..."
	podman build -f container/Containerfile -t quay.io/pilhuhn/otel-oql:aarch64 --platform linux/aarch64 .
	podman build -f container/Containerfile -t quay.io/pilhuhn/otel-oql:amd64 --platform linux/amd64 .

# Build and push images (alias for images)
.PHONY: images-push
images-push: images

# Setup Pinot schema
.PHONY: setup-schema
setup-schema: otel-oql
	@echo "Setting up Pinot schema..."
	./$(BUILD_DIR)/$(BINARY_OTEL_OQL) setup-schema --pinot-url=http://localhost:9000

# Run the service in test mode
.PHONY: run
run: otel-oql
	@echo "Running otel-oql in test mode..."
	./$(BUILD_DIR)/$(BINARY_OTEL_OQL) --test-mode

# Run the service with observability
.PHONY: run-observability
run-observability: otel-oql
	@echo "Running otel-oql with observability enabled..."
	./$(BUILD_DIR)/$(BINARY_OTEL_OQL) --test-mode --observability-enabled

# Start infrastructure (Pinot, Kafka)
.PHONY: infra-up
infra-up:
	@echo "Starting infrastructure with podman-compose..."
	podman-compose up -d

# Stop infrastructure
.PHONY: infra-down
infra-down:
	@echo "Stopping infrastructure..."
	podman-compose down

# Restart infrastructure
.PHONY: infra-restart
infra-restart: infra-down infra-up

# Development setup (start infra + setup schema)
.PHONY: dev-setup
dev-setup: infra-up
	@echo "Waiting for Pinot to be ready..."
	@sleep 10
	@$(MAKE) setup-schema

# Quick development cycle: build + run
.PHONY: dev
dev: build run

# CI target (format, vet, test)
.PHONY: ci
ci: fmt vet test

# Show version information
.PHONY: version
version:
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Platform: $(PLATFORM)"

# Help target
.PHONY: help
help:
	@echo "OTEL-OQL Makefile targets:"
	@echo ""
	@echo "Build targets:"
	@echo "  make build           - Build otel-oql and oql-cli (default)"
	@echo "  make otel-oql        - Build otel-oql service only"
	@echo "  make oql-cli         - Build oql-cli tool only"
	@echo "  make send-test-data  - Build test data generator"
	@echo "  make build-all       - Build all binaries"
	@echo "  make install         - Install binaries to \$$GOPATH/bin"
	@echo ""
	@echo "Test targets:"
	@echo "  make test            - Run all tests"
	@echo "  make test-coverage   - Run tests with coverage report"
	@echo "  make test-race       - Run tests with race detector"
	@echo "  make test-promql     - Run PromQL tests only"
	@echo "  make test-logql      - Run LogQL tests only"
	@echo "  make test-integration - Run integration tests only"
	@echo ""
	@echo "Code quality:"
	@echo "  make fmt             - Format code with go fmt"
	@echo "  make vet             - Run go vet"
	@echo "  make tidy            - Tidy go modules"
	@echo "  make deps            - Download dependencies"
	@echo "  make deps-update     - Update dependencies to latest versions"
	@echo "  make ci              - Run fmt, vet, and test"
	@echo ""
	@echo "Development:"
	@echo "  make run             - Run otel-oql in test mode"
	@echo "  make run-observability - Run with observability enabled"
	@echo "  make dev             - Quick dev cycle (build + run)"
	@echo "  make setup-schema    - Setup Pinot schema"
	@echo ""
	@echo "Infrastructure:"
	@echo "  make infra-up        - Start Pinot/Kafka with podman-compose"
	@echo "  make infra-down      - Stop infrastructure"
	@echo "  make infra-restart   - Restart infrastructure"
	@echo "  make dev-setup       - Start infra + setup schema"
	@echo ""
	@echo "Container images:"
	@echo "  make images          - Build and push multi-arch container images"
	@echo "  make images-build    - Build images locally (no push)"
	@echo "  make images-push     - Build and push images (alias for images)"
	@echo ""
	@echo "Cleanup:"
	@echo "  make clean           - Remove build artifacts"
	@echo "  make clean-all       - Deep clean (includes mod cache)"
	@echo ""
	@echo "Info:"
	@echo "  make version         - Show version information"
	@echo "  make help            - Show this help message"

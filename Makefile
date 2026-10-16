# hax - Makefile
# Provides build targets with version injection

# Version information
VERSION := 1.0.0
PRERELEASE := testnet
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build paths
BINARY_NAME := hax
BUILD_DIR := ./build
CMD_PATH := ./cmd/hax

# Go build settings
GO := go
GOFLAGS := -trimpath
LDFLAGS := -s -w \
	-X 'github.com/BTCBap/hax/internal/version.Version=$(VERSION)' \
	-X 'github.com/BTCBap/hax/internal/version.Prerelease=$(PRERELEASE)' \
	-X 'github.com/BTCBap/hax/internal/version.GitCommit=$(GIT_COMMIT)' \
	-X 'github.com/BTCBap/hax/internal/version.GitBranch=$(GIT_BRANCH)' \
	-X 'github.com/BTCBap/hax/internal/version.BuildDate=$(BUILD_DATE)'

.PHONY: all build build-release build-all checksums clean run test version help

# Default target
all: build

# Development build (no version injection, faster)
build:
	$(GO) build -o $(BINARY_NAME) $(CMD_PATH)

# Release build with version injection
build-release:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) $(CMD_PATH)

# Build for multiple platforms
build-all: clean
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_PATH)
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_PATH)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_PATH)
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_PATH)
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_PATH)

# Generate SHA256 checksums for local release artifacts
checksums: build-all
	@cd $(BUILD_DIR) && { \
		if command -v sha256sum >/dev/null 2>&1; then \
			sha256sum * > checksums.txt; \
		else \
			shasum -a 256 * > checksums.txt; \
		fi; \
	}

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)

# Run the wallet
run: build
	./$(BINARY_NAME)

# Run in debug mode
run-debug: build
	DEBUG=1 ./$(BINARY_NAME)

# Run tests
test:
	$(GO) test -v ./...

# Run tests with race detection
test-race:
	$(GO) test -race -v ./...

# Show version info
version:
	@echo "Version:    $(VERSION)-$(PRERELEASE)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Git Branch: $(GIT_BRANCH)"
	@echo "Build Date: $(BUILD_DATE)"

# Lint code
lint:
	golangci-lint run ./...

# Format code
fmt:
	$(GO) fmt ./...

# Tidy dependencies
tidy:
	$(GO) mod tidy

# Help
help:
	@echo "hax - Build Targets"
	@echo ""
	@echo "  make build         - Quick development build"
	@echo "  make build-release - Release build with version info"
	@echo "  make build-all     - Build for all platforms"
	@echo "  make checksums     - Build release artifacts and write SHA256 checksums"
	@echo "  make run           - Build and run"
	@echo "  make run-debug     - Build and run with DEBUG=1"
	@echo "  make test          - Run tests"
	@echo "  make test-race     - Run tests with race detection"
	@echo "  make clean         - Remove build artifacts"
	@echo "  make version       - Show version information"
	@echo "  make lint          - Run linter"
	@echo "  make fmt           - Format code"
	@echo "  make tidy          - Tidy go.mod"
	@echo ""

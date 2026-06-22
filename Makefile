# ==============================================================================
# ShiftLaunch Build Configuration
# ==============================================================================

APP_NAME := shiftlaunch
MAIN_FILE := main.go
BUILD_DIR := bin

# Go build flags
# -s: Disable symbol table
# -w: Disable DWARF generation (reduces binary size)
LDFLAGS := -s -w

.PHONY: all build build-linux-ppc64le clean install test vet fmt help

all: clean fmt vet build

# ------------------------------------------------------------------------------
# Build Targets
# ------------------------------------------------------------------------------

## build: Build the binary for the host's native architecture
build:
	@echo "==> Building $(APP_NAME) for native host architecture..."
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_FILE)
	@echo "==> Build complete: $(BUILD_DIR)/$(APP_NAME)"

## build-ppc64le: Cross-compile specifically for IBM Power Systems (Linux ppc64le)
build-ppc64le:
	@echo "==> Cross-compiling $(APP_NAME) for Linux ppc64le..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=ppc64le go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-linux-ppc64le $(MAIN_FILE)
	@echo "==> Cross-compilation complete: $(BUILD_DIR)/$(APP_NAME)-linux-ppc64le"

# ------------------------------------------------------------------------------
# Development & Quality Assurance
# ------------------------------------------------------------------------------

## fmt: Format Go source code
fmt:
	@echo "==> Formatting code..."
	go fmt ./...

## vet: Run go vet to catch common logical bugs
vet:
	@echo "==> Running go vet..."
	go vet ./...

## test: Run all unit tests
test:
	@echo "==> Running tests..."
	go test -v -race ./...

# ------------------------------------------------------------------------------
# System Integration
# ------------------------------------------------------------------------------

## install: Install the binary to /usr/local/bin (requires sudo)
install: build
	@echo "==> Installing $(APP_NAME) to /usr/local/bin..."
	sudo cp $(BUILD_DIR)/$(APP_NAME) /usr/local/bin/
	@echo "==> Installation complete. You can now run '$(APP_NAME)' from anywhere."

## clean: Remove build artifacts and compiled binaries
clean:
	@echo "==> Cleaning build directory..."
	@rm -rf $(BUILD_DIR)
	@echo "==> Clean complete."

# ------------------------------------------------------------------------------
# Help
# ------------------------------------------------------------------------------

## help: Display this help message
help:
	@echo "ShiftLaunch Makefile Commands:"
	@echo ""
	@echo "Build Targets:"
	@echo "  make build              - Build binary for native architecture"
	@echo "  make build-ppc64le      - Cross-compile for IBM Power Systems (Linux ppc64le)"
	@echo ""
	@echo "Development:"
	@echo "  make fmt                - Format Go source code"
	@echo "  make vet                - Run go vet for static analysis"
	@echo "  make test               - Run all unit tests with race detector"
	@echo ""
	@echo "System Integration:"
	@echo "  make install            - Install binary to /usr/local/bin (requires sudo)"
	@echo "  make clean              - Remove build artifacts"
	@echo ""
	@echo "Workflows:"
	@echo "  make all                - Run clean, fmt, vet, and build (default)"
	@echo "  make help               - Display this help message"

# Made with Bob

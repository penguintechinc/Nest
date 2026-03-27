# ArticDBM Proxy Makefile with XDP/eBPF Support

# Build configuration
GO_VERSION := 1.21
KERNEL_VERSION := $(shell uname -r)
ARCH := $(shell uname -m)

# Directories
BPF_DIR := internal/xdp/bpf
BUILD_DIR := bpf-build
BIN_DIR := bin

# eBPF/XDP configuration
CLANG := clang
LLC := llc
BPFTOOL := bpftool
BPF_CFLAGS := -O2 -g -Wall
BPF_TARGET := bpf
BPF_INCLUDES := -I/usr/include/$(shell uname -m)-linux-gnu -I.

# Source files
BPF_SOURCES := $(wildcard $(BPF_DIR)/*.c)
BPF_OBJECTS := $(BPF_SOURCES:$(BPF_DIR)/%.c=$(BUILD_DIR)/%.o)

# Go build flags
GO_BUILD_FLAGS := -ldflags="-s -w" -trimpath

.PHONY: all build clean deps check-deps bpf build-xdp test install

all: deps bpf build

# Check for required dependencies
check-deps:
	@echo "Checking dependencies..."
	@which clang > /dev/null || (echo "Error: clang not found. Install: sudo apt-get install clang" && exit 1)
	@which llvm-objcopy > /dev/null || (echo "Error: llvm-objcopy not found. Install: sudo apt-get install llvm" && exit 1)
	@test -d /usr/include/linux || (echo "Error: Linux headers not found. Install: sudo apt-get install linux-headers-$(shell uname -r)" && exit 1)
	@echo "All dependencies satisfied"

# Install system dependencies
deps:
	@echo "Installing system dependencies..."
	sudo apt-get update
	sudo apt-get install -y \
		clang \
		llvm \
		libbpf-dev \
		linux-headers-$(shell uname -r) \
		build-essential \
		libnuma-dev \
		pkg-config

# Create build directories
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

# Compile eBPF programs
$(BUILD_DIR)/%.o: $(BPF_DIR)/%.c | $(BUILD_DIR) check-deps
	@echo "Compiling eBPF program: $<"
	$(CLANG) $(BPF_CFLAGS) $(BPF_INCLUDES) -target $(BPF_TARGET) -c $< -o $@

# Build all eBPF programs
bpf: $(BPF_OBJECTS)
	@echo "eBPF programs compiled successfully"

# Alias used by Dockerfile
build-xdp: bpf

# Build Go application
build: bpf | $(BIN_DIR)
	@echo "Building ArticDBM proxy with XDP support..."
	go build $(GO_BUILD_FLAGS) -o $(BIN_DIR)/articdbm-proxy .

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) $(BIN_DIR)

# Install to system (requires root)
install: build
	@echo "Installing ArticDBM proxy..."
	sudo cp $(BIN_DIR)/articdbm-proxy /usr/local/bin/
	sudo chmod +x /usr/local/bin/articdbm-proxy
	@echo "Installation complete"

# Development helpers
dev-setup:
	@echo "Setting up development environment..."
	sudo sysctl -w net.core.bpf_jit_enable=1
	sudo sysctl -w net.core.bpf_jit_harden=0
	@echo "Development environment configured"

# Load XDP programs (for testing)
load-xdp: bpf
	@echo "Loading XDP programs..."
	sudo ip link set dev lo xdpgeneric obj $(BUILD_DIR)/ip_blocklist.o sec xdp_ip_filter || true

# Unload XDP programs
unload-xdp:
	@echo "Unloading XDP programs..."
	sudo ip link set dev lo xdpgeneric off || true

# Show eBPF program info
show-bpf:
	@echo "Loaded eBPF programs:"
	sudo bpftool prog show

# Show XDP programs
show-xdp:
	@echo "Loaded XDP programs:"
	sudo bpftool net show

# Benchmark
benchmark:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

help:
	@echo "Available targets:"
	@echo "  all          - Build everything (default)"
	@echo "  build        - Build Go application with eBPF"
	@echo "  bpf          - Compile eBPF programs only"
	@echo "  test         - Run tests"
	@echo "  clean        - Clean build artifacts"
	@echo "  deps         - Install system dependencies"
	@echo "  install      - Install to system"
	@echo "  dev-setup    - Configure development environment"
	@echo "  load-xdp     - Load XDP programs for testing"
	@echo "  unload-xdp   - Unload XDP programs"
	@echo "  show-bpf     - Show loaded eBPF programs"
	@echo "  show-xdp     - Show loaded XDP programs"
	@echo "  benchmark    - Run performance benchmarks"
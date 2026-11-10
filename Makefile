## apic — Makefile

APP      := apic
MAIN_DIR := ./cmd/apic
GOEXE    = $(shell go env GOEXE)
GOPATH   = $(shell go env GOPATH)
BINARY   := $(APP)$(GOEXE)
DIST_DIR := dist

# Build metadata
BUILD_TIME := $(shell date +%Y-%m-%dT%H:%M:%S)
GIT_HASH   := $(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "unknown")
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo "dev-$(GIT_HASH)")

LDFLAGS := -s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_HASH) \
	-X 'main.BuildDate=$(BUILD_TIME)'

# upx is optional: binaries are compressed only when it is installed.
UPX := $(shell command -v upx 2>/dev/null)
# Written into latest.yaml via echo; keep it free of shell/YAML metacharacters.
DESCRIPTION := call any HTTP API with commands generated from OpenAPI or simple definitions

.PHONY: all build install test lint clean clean-dist help dump-info latest-yaml build-all \
	build-linux build-linux-arm64 build-darwin build-darwin-arm64 build-windows

## all: build (default)
all: build

## build: build binary for the current platform into dist/
build:
	@mkdir -p $(DIST_DIR)
	@echo "Building $(APP) $(VERSION) @ $(GIT_HASH)..."
	go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY) $(MAIN_DIR)
	@echo "Binary: $(DIST_DIR)/$(BINARY)"

## install: build and copy the binary to $GOPATH/bin
install: build
	@mkdir -p $(GOPATH)/bin
	@cp $(DIST_DIR)/$(BINARY) $(GOPATH)/bin/$(BINARY)
	@echo "Installed to $(GOPATH)/bin/$(BINARY)"

## test: run vet and all unit tests
test:
	go vet ./...
	go test -count=1 -cover ./...

## lint: check formatting (and run staticcheck when installed)
lint:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "gofmt needed" && exit 1)
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; fi

# ─── Cross Compilation ────────────────────────────────────────────────────────

## build-all: cross-compile for all platforms
build-all: clean-dist dump-info build-linux build-linux-arm64 build-darwin build-darwin-arm64 build-windows latest-yaml
	ls -lh $(DIST_DIR)

## dump-info: print build info
dump-info:
	@echo "Build Info:"
	@echo "  VERSION: $(VERSION)"
	@echo "  GIT_HASH: $(GIT_HASH)"
	@echo "  BUILD_TIME: $(BUILD_TIME)"

## latest-yaml: generate latest.yaml release metadata
latest-yaml:
	@mkdir -p $(DIST_DIR)
	@{ \
		echo "name: $(APP)"; \
		echo "version: $(VERSION)"; \
		echo "released_at: $(BUILD_TIME)"; \
		echo "description: $(DESCRIPTION)"; \
	} > $(DIST_DIR)/latest.yaml
	@echo "   -> $(DIST_DIR)/latest.yaml"

# cross_build GOOS,GOARCH,OUTPUT,upx-or-empty
define cross_build
	@echo "$(1)/$(2)..."
	@mkdir -p $(DIST_DIR)
	@GOOS=$(1) GOARCH=$(2) go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(3) $(MAIN_DIR)
	@if [ -n "$(UPX)" ] && [ "$(4)" = "upx" ]; then upx -6 --no-progress $(DIST_DIR)/$(3) >/dev/null; fi
	@echo "   -> $(DIST_DIR)/$(3)"
endef

## build-linux: compile for Linux amd64
build-linux:
	$(call cross_build,linux,amd64,$(APP)-linux-amd64,upx)

## build-linux-arm64: compile for Linux arm64
build-linux-arm64:
	$(call cross_build,linux,arm64,$(APP)-linux-arm64,upx)

## build-darwin: compile for macOS amd64
build-darwin:
	$(call cross_build,darwin,amd64,$(APP)-darwin-amd64,)

## build-darwin-arm64: compile for macOS Apple Silicon (upx breaks binaries on macOS 12+)
build-darwin-arm64:
	$(call cross_build,darwin,arm64,$(APP)-darwin-arm64,)

## build-windows: compile for Windows amd64
build-windows:
	$(call cross_build,windows,amd64,$(APP)-windows-amd64.exe,upx)

## clean: remove build artifacts
clean:
	rm -rf ./$(DIST_DIR)

## clean-dist: recreate an empty dist dir
clean-dist: clean
	@mkdir -p $(DIST_DIR)

## help: show this help
help:
	@echo "apic Build System"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

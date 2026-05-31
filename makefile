# ---- Project ----------------------------------------------------------------
BINARY_NAME = Votan
PKG         = ./cmd/server
DIST_DIR    = dist

# Version is taken from git tags (falls back to "dev"), injected into the binary.
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_FLAGS = -trimpath -ldflags="-s -w -X main.version=$(VERSION)"

# Single self-contained binaries (the web UI is embedded). Windows + macOS.
RELEASE_TARGETS = windows/amd64 darwin/amd64 darwin/arm64

# Local binary name (.exe on Windows).
ifeq ($(OS),Windows_NT)
	LOCAL_BIN = $(BINARY_NAME).exe
else
	LOCAL_BIN = $(BINARY_NAME)
endif

.PHONY: all build run test race fmt vet lint check dist tidy clean help

all: check build

## build: compile a self-contained binary for this machine
build:
	@echo "Building $(LOCAL_BIN) ($(VERSION))..."
	@go build $(BUILD_FLAGS) -o $(LOCAL_BIN) $(PKG)
	@echo "Done."

## run: run without producing a binary
run:
	@go run $(PKG)

## test: run all unit tests
test:
	@go test ./...

## race: run all tests under the race detector
race:
	@go test -race -count=1 ./...

## fmt: format all Go files in place
fmt:
	@gofmt -w .

## vet: run go vet
vet:
	@go vet ./...

## lint: run golangci-lint (install: https://golangci-lint.run)
lint:
	@golangci-lint run

## check: what CI enforces — gofmt, vet and race tests (needs a Unix shell)
check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt: needs formatting:"; gofmt -l .; exit 1; }
	@go vet ./...
	@go test -race -count=1 ./...

## dist: cross-compile single binaries for Windows + macOS into ./dist
##       (needs a Unix-like shell: bash/zsh, or Git Bash on Windows)
dist: clean
	@mkdir -p $(DIST_DIR)
	@for t in $(RELEASE_TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; \
		out=$(DIST_DIR)/$(BINARY_NAME)_$(VERSION)_$${os}_$${arch}; \
		[ "$$os" = "windows" ] && out=$$out.exe; \
		echo "  $$os/$$arch -> $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build $(BUILD_FLAGS) -o $$out $(PKG) || exit 1; \
	done
	@cd $(DIST_DIR) && { command -v sha256sum >/dev/null 2>&1 && sha256sum * > checksums.txt || shasum -a 256 * > checksums.txt; }
	@echo "Artifacts in $(DIST_DIR)/. For a published release: git tag vX.Y.Z && git push --tags"

## tidy: tidy go.mod / go.sum
tidy:
	@go mod tidy

## clean: remove build artifacts
clean:
ifeq ($(OS),Windows_NT)
	@if exist $(LOCAL_BIN) del /q $(LOCAL_BIN)
	@if exist $(DIST_DIR) rd /s /q $(DIST_DIR)
else
	@rm -f $(LOCAL_BIN)
	@rm -rf $(DIST_DIR)
endif
	@go clean
	@echo "Cleaned."

## help: list available commands
help:
	@echo "Available commands:"
	@echo "  make build   - self-contained binary for this machine"
	@echo "  make run     - run without building"
	@echo "  make test    - unit tests"
	@echo "  make race    - tests under the race detector"
	@echo "  make fmt     - gofmt -w ."
	@echo "  make vet     - go vet"
	@echo "  make lint    - golangci-lint run"
	@echo "  make check   - gofmt + vet + race (what CI runs)"
	@echo "  make dist    - cross-compile Windows + macOS binaries into ./dist"
	@echo "  make tidy    - go mod tidy"
	@echo "  make clean   - remove build artifacts"
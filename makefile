# Project
PROJECT_NAME = Votan
MAIN_PATH    = cmd/server/main.go
RELEASE_DIR  = $(PROJECT_NAME)_Release
ZIP_NAME     = $(PROJECT_NAME)_v1.0.zip

# Strip symbol/debug info to shrink the binary.
LDFLAGS = -ldflags="-s -w"

# Binary name per OS (.exe on Windows).
ifeq ($(OS),Windows_NT)
	BINARY_NAME = $(PROJECT_NAME).exe
else
	BINARY_NAME = $(PROJECT_NAME)
endif

.PHONY: all build test run clean release help

all: test build

## build: compile the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "Done."

## test: run the unit tests
test:
	@echo "Running tests..."
	@go test ./internal/engine/...
	@echo "Tests passed."

## run: run without building
run:
	@go run $(MAIN_PATH)

## clean: remove build artifacts and temp files
clean:
ifeq ($(OS),Windows_NT)
	@if exist $(BINARY_NAME) del /q $(BINARY_NAME)
	@if exist $(RELEASE_DIR) rd /s /q $(RELEASE_DIR)
	@if exist $(ZIP_NAME) del /q $(ZIP_NAME)
else
	@rm -f $(BINARY_NAME)
	@rm -rf $(RELEASE_DIR)
	@rm -f $(ZIP_NAME)
endif
	@go clean
	@echo "Cleaned."

## release: build and package a distributable zip (binary + assets + sample .env)
## note: on Unix this needs `zip` installed (preinstalled on macOS).
release: clean test
	@echo "Building release..."
ifeq ($(OS),Windows_NT)
	@mkdir $(RELEASE_DIR)
	@mkdir $(RELEASE_DIR)\web
	@mkdir $(RELEASE_DIR)\web\public
	@go build $(LDFLAGS) -o $(RELEASE_DIR)\$(BINARY_NAME) $(MAIN_PATH)
	@xcopy /E /I /Y web\public $(RELEASE_DIR)\web\public > nul
	@echo OBS_ADDR=localhost:4455 > $(RELEASE_DIR)\.env
	@echo OBS_PASS=your_password >> $(RELEASE_DIR)\.env
	@echo ADMIN_SECRET=your_secret_token >> $(RELEASE_DIR)\.env
	@echo YOUTUBE_VIDEO_ID= >> $(RELEASE_DIR)\.env
	@powershell Compress-Archive -Path $(RELEASE_DIR) -DestinationPath $(ZIP_NAME) -Force
else
	@mkdir -p $(RELEASE_DIR)/web
	@go build $(LDFLAGS) -o $(RELEASE_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@cp -r web/public $(RELEASE_DIR)/web/
	@printf 'OBS_ADDR=localhost:4455\nOBS_PASS=your_password\nADMIN_SECRET=your_secret_token\nYOUTUBE_VIDEO_ID=\n' > $(RELEASE_DIR)/.env
	@zip -r $(ZIP_NAME) $(RELEASE_DIR) > /dev/null
endif
	@echo "--------------------------------------------------"
	@echo "Release ready: $(ZIP_NAME)"
	@echo "--------------------------------------------------"

## help: list available commands
help:
	@echo "Available commands:"
	@echo "  make build   - build the binary"
	@echo "  make test    - run the tests"
	@echo "  make run     - run without building"
	@echo "  make release - build a distributable zip"
	@echo "  make clean   - remove temporary files"
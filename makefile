# Назва проєкту
PROJECT_NAME=Votan
BINARY_NAME=$(PROJECT_NAME).exe
RELEASE_DIR=$(PROJECT_NAME)_Release
ZIP_NAME=$(PROJECT_NAME)_v1.0.zip

# Шлях до головного файлу
MAIN_PATH=cmd/server/main.go

# Прапорці збірки для мінімізації розміру
LDFLAGS=-ldflags="-s -w"

.PHONY: all build test clean run release help

all: test build

## build: Компіляція проєкту
build:
	@echo "Збірка $(BINARY_NAME)..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "Готово!"

## test: Запуск усіх Unit-тестів
test:
	@echo "Запуск тестів..."
	@go test ./internal/engine/...
	@echo "Тести пройдено!"

## run: Запуск без збірки
run:
	@go run $(MAIN_PATH)

## clean: Видалення бінарних файлів та тимчасових папок
clean:
	@echo "Очищення..."
	@if exist $(BINARY_NAME) del /q $(BINARY_NAME)
	@if exist $(RELEASE_DIR) rd /s /q $(RELEASE_DIR)
	@if exist $(ZIP_NAME) del /q $(ZIP_NAME)
	@go clean
	@echo "Очищено!"

## release: Збірка проєкту та пакування в ZIP-архів для релізу
release: clean test
	@echo "Підготовка релізу..."
	@mkdir $(RELEASE_DIR)
	@mkdir $(RELEASE_DIR)\web
	@mkdir $(RELEASE_DIR)\web\public
	
	@echo "Компіляція бойової версії..."
	@go build $(LDFLAGS) -o $(RELEASE_DIR)\$(BINARY_NAME) $(MAIN_PATH)
	
	@echo "Копіювання асетів та фронтенду..."
	@xcopy /E /I /Y web\public $(RELEASE_DIR)\web\public > nul
	
	@echo "Створення шаблону .env..."
	@echo OBS_ADDR=localhost:4455 > $(RELEASE_DIR)\.env
	@echo OBS_PASS=your_password >> $(RELEASE_DIR)\.env
	@echo ADMIN_SECRET=your_secret_token >> $(RELEASE_DIR)\.env
	@echo YOUTUBE_VIDEO_ID= >> $(RELEASE_DIR)\.env
	
	@echo "Пакування в $(ZIP_NAME)..."
	@powershell Compress-Archive -Path $(RELEASE_DIR) -DestinationPath $(ZIP_NAME) -Force
	
	@echo "--------------------------------------------------"
	@echo "РЕЛІЗ ГОТОВИЙ: $(ZIP_NAME)"
	@echo "--------------------------------------------------"

## help: Показати доступні команди
help:
	@echo "Доступні команди:"
	@echo "  make build   - Зібрати Votan.exe"
	@echo "  make test    - Запустити тести"
	@echo "  make release - Створити готовий до відправки ZIP-архів"
	@echo "  make clean   - Видалити всі тимчасові файли"
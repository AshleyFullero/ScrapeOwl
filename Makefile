# ScrapeOwl Makefile

BINARY_NAME := scrapeowl
BUILD_DIR := ./dist
CMD := ./cmd/scrapeowl

# Build info
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -ldflags "-s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)"

.PHONY: all build run test clean lint docker docker-run help

## help: Show this help message
help:
	@echo "ScrapeOwl Build System"
	@echo ""
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/  /'

## all: Build the binary
all: build

## build: Compile the binary for the current platform
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD)
	@echo "Binary: $(BUILD_DIR)/$(BINARY_NAME)"

## build-linux: Cross-compile for Linux (amd64)
build-linux:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD)

## build-mac: Cross-compile for macOS (arm64)
build-mac:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD)

## build-windows: Cross-compile for Windows (amd64)
build-windows:
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD)

## build-all: Build for all platforms
build-all: build-linux build-mac build-windows

## run: Run the dashboard server
run: build
	$(BUILD_DIR)/$(BINARY_NAME) serve --addr :8080 --debug

## test: Run all tests
test:
	go test -v -race -coverprofile=coverage.out ./...

## test-coverage: Run tests and show coverage report
test-coverage: test
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run go vet and staticcheck
lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "Install staticcheck: go install honnef.co/go/tools/cmd/staticcheck@latest"

## tidy: Tidy go modules
tidy:
	go mod tidy

## clean: Remove build artifacts
clean:
	@rm -rf $(BUILD_DIR) coverage.out coverage.html scrapeowl.db

## docker: Build the Docker image
docker:
	docker build -t scrapeowl:latest -t scrapeowl:$(VERSION) .

## docker-run: Run with Docker Compose
docker-run:
	docker compose up -d

## docker-stop: Stop Docker Compose services
docker-stop:
	docker compose down

## docker-logs: Follow Docker Compose logs
docker-logs:
	docker compose logs -f

## validate-example: Validate the example job files
validate-example:
	$(BUILD_DIR)/$(BINARY_NAME) validate --file ./examples/product-scraper.yaml
	$(BUILD_DIR)/$(BINARY_NAME) validate --file ./examples/news-scraper.yaml
	$(BUILD_DIR)/$(BINARY_NAME) validate --file ./examples/ecommerce.yaml

## install: Install the binary to GOPATH/bin
install:
	go install $(LDFLAGS) $(CMD)

.DEFAULT_GOAL := help

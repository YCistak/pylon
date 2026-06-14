# Pylon — build & dev tasks.
# `make` builds ./pylon; `make run ARGS="say merhaba"` runs it.

BIN     := pylon
PKG     := ./cmd/pylon
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build run test vet tidy clean install dist

build: ## Build ./pylon for the host
	go build -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

run: build ## Build then run: make run ARGS="status"
	./$(BIN) $(ARGS)

test: ## Run all tests
	go test ./...

vet: ## Static checks
	go vet ./...

tidy: ## Sync go.mod/go.sum
	go mod tidy

install: ## Install pylon into GOBIN/PATH
	go install -ldflags "$(LDFLAGS)" $(PKG)

clean: ## Remove build artifacts
	rm -f $(BIN) pylon-linux-* pylon-windows-* pylon-darwin-*

# Cross-compile release binaries (CGo-free, so this just works).
dist: ## Build linux/amd64, windows/amd64, darwin/arm64
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o pylon-linux-amd64     $(PKG)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o pylon-windows-amd64.exe $(PKG)
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o pylon-darwin-arm64     $(PKG)

# Pylon — build & dev tasks.
# `make` builds ./pylon; `make run ARGS="say merhaba"` runs it.

BIN     := pylon
PKG     := ./cmd/pylon
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GPKG    := github.com/YCistak/pylon/internal/services/google
SPKG    := github.com/YCistak/pylon/internal/services/spotify

# Bake the project's OAuth clients into the build so end users only sign in /
# connect (no Google Cloud / Spotify Dashboard setup for them). Set once, e.g.:
#   make build GOOGLE_CLIENT_ID=xxx.apps.googleusercontent.com GOOGLE_CLIENT_SECRET=yyy SPOTIFY_CLIENT_ID=zzz
# Or persist them in a gitignored `build.env` next to this Makefile:
#   GOOGLE_CLIENT_ID = xxx.apps.googleusercontent.com
#   GOOGLE_CLIENT_SECRET = yyy
#   SPOTIFY_CLIENT_ID = zzz
# Spotify uses Authorization Code + PKCE — no client secret needed.
-include build.env
GOOGLE_CLIENT_ID     ?=
GOOGLE_CLIENT_SECRET ?=
SPOTIFY_CLIENT_ID    ?=

LDFLAGS := -s -w -X main.version=$(VERSION) \
  -X $(GPKG).embeddedClientID=$(GOOGLE_CLIENT_ID) \
  -X $(GPKG).embeddedClientSecret=$(GOOGLE_CLIENT_SECRET) \
  -X $(SPKG).embeddedClientID=$(SPOTIFY_CLIENT_ID)

# The GUI needs Wails' own build tags, not just the webkit one. Without
# `desktop,production` the binary compiles and then refuses to start with
# "Wails applications will not build without the correct build tags" — CI only
# type-checks it (`go build -tags webkit2_41`), so nothing catches a binary
# built the wrong way until you try to open it. webkit2_41 is Linux-only;
# override UI_TAGS on other platforms.
UI_TAGS ?= desktop,production,webkit2_41

.PHONY: build gui run test vet tidy clean install install-user dist

build: ## Build ./pylon for the host
	go build -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

gui: ## Build the Wails GUI into pylon-ui/build/bin/pylon-ui
	cd pylon-ui/frontend && { [ -d node_modules ] || npm ci; } && npm run build
	cd pylon-ui && go build -tags "$(UI_TAGS)" -ldflags "-s -w -X main.version=$(VERSION)" -o build/bin/pylon-ui .

run: build ## Build then run: make run ARGS="status"
	./$(BIN) $(ARGS)

test: ## Run all tests
	go test ./...

vet: ## Static checks
	go vet ./...

tidy: ## Sync go.mod/go.sum
	go mod tidy

install: ## Install the daemon into GOBIN/PATH (Go developers)
	go install -ldflags "$(LDFLAGS)" $(PKG)

# The whole application for one user: both binaries, the icon, a desktop entry
# and — only if you have none — the example config. Nothing needs root, and
# ~/.local/bin is deliberately absent from selfupdate's packaged-prefix list, so
# a Pylon installed this way can still update itself. Run with ARGS=--dry-run to
# see the file operations first.
install-user: build gui ## Install Pylon for the current user (~/.local)
	sh scripts/install-user.sh $(ARGS)

clean: ## Remove build artifacts
	rm -f $(BIN) pylon-linux-* pylon-windows-* pylon-darwin-*

# Cross-compile release binaries (CGo-free, so this just works).
dist: ## Build linux/amd64, windows/amd64, darwin/arm64
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o pylon-linux-amd64     $(PKG)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o pylon-windows-amd64.exe $(PKG)
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o pylon-darwin-arm64     $(PKG)

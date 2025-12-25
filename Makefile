.PHONY: all build clean install uninstall test deps

BINARY_NAME=typtel
MENUBAR_NAME=typtel-menubar
DAEMON_NAME=typtel-daemon
VERSION?=0.1.0
BUILD_DIR=build
PREFIX?=/usr/local

# Go build flags
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION)"

all: deps build

deps:
	go mod tidy
	go mod download

build: build-cli build-menubar build-daemon

build-cli:
	@echo "Building CLI..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/typtel

build-menubar:
	@echo "Building menu bar app..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(MENUBAR_NAME) ./cmd/typtel-menubar

build-daemon:
	@echo "Building daemon..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(DAEMON_NAME) ./cmd/daemon

install: build
	@echo "Installing to $(PREFIX)/bin..."
	@mkdir -p $(PREFIX)/bin
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(PREFIX)/bin/
	@cp $(BUILD_DIR)/$(MENUBAR_NAME) $(PREFIX)/bin/
	@cp $(BUILD_DIR)/$(DAEMON_NAME) $(PREFIX)/bin/
	@echo "Installing LaunchAgent..."
	@mkdir -p ~/Library/LaunchAgents
	@sed 's|BINARY_PATH|$(PREFIX)/bin/$(MENUBAR_NAME)|g' scripts/com.typtel.menubar.plist > ~/Library/LaunchAgents/com.typtel.menubar.plist
	@echo "Done! Run 'typtel' to see stats or 'launchctl load ~/Library/LaunchAgents/com.typtel.menubar.plist' to start menu bar app"

uninstall:
	@echo "Uninstalling..."
	@launchctl unload ~/Library/LaunchAgents/com.typtel.menubar.plist 2>/dev/null || true
	@rm -f ~/Library/LaunchAgents/com.typtel.menubar.plist
	@rm -f $(PREFIX)/bin/$(BINARY_NAME)
	@rm -f $(PREFIX)/bin/$(MENUBAR_NAME)
	@rm -f $(PREFIX)/bin/$(DAEMON_NAME)
	@echo "Done!"

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@go clean

test:
	go test -v ./...

# Development helpers
run-cli: build-cli
	./$(BUILD_DIR)/$(BINARY_NAME)

run-menubar: build-menubar
	./$(BUILD_DIR)/$(MENUBAR_NAME)

run-daemon: build-daemon
	./$(BUILD_DIR)/$(DAEMON_NAME)

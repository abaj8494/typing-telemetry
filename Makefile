.PHONY: all build clean install uninstall test deps start stop app

BINARY_NAME=typtel
MENUBAR_NAME=typtel-menubar
DAEMON_NAME=typtel-daemon
APP_NAME=Typtel.app
VERSION?=0.1.0
BUILD_DIR=build
PREFIX?=/usr/local
APP_DIR=/Applications

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

app: build-menubar
	@echo "Creating $(APP_NAME) bundle..."
	@mkdir -p $(BUILD_DIR)/$(APP_NAME)/Contents/MacOS
	@mkdir -p $(BUILD_DIR)/$(APP_NAME)/Contents/Resources
	@cp $(BUILD_DIR)/$(MENUBAR_NAME) $(BUILD_DIR)/$(APP_NAME)/Contents/MacOS/
	@cp scripts/Info.plist $(BUILD_DIR)/$(APP_NAME)/Contents/
	@echo "App bundle created at $(BUILD_DIR)/$(APP_NAME)"

install: build app
	@echo "Installing CLI to $(PREFIX)/bin..."
	@mkdir -p $(PREFIX)/bin
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(PREFIX)/bin/
	@cp $(BUILD_DIR)/$(DAEMON_NAME) $(PREFIX)/bin/
	@echo "Installing $(APP_NAME) to $(APP_DIR)..."
	@rm -rf $(APP_DIR)/$(APP_NAME)
	@cp -r $(BUILD_DIR)/$(APP_NAME) $(APP_DIR)/
	@echo "Installing LaunchAgent..."
	@mkdir -p ~/Library/LaunchAgents
	@sed -e 's|APP_PATH|$(APP_DIR)/$(APP_NAME)|g' scripts/com.typtel.menubar.plist > ~/Library/LaunchAgents/com.typtel.menubar.plist
	@echo ""
	@echo "Done! To start (run WITHOUT sudo):"
	@echo "  make start"
	@echo ""
	@echo "Or manually: open -a Typtel"
	@echo ""
	@echo "IMPORTANT: Grant Accessibility permissions to Typtel.app in:"
	@echo "  System Settings > Privacy & Security > Accessibility"

start:
	@launchctl unload ~/Library/LaunchAgents/com.typtel.menubar.plist 2>/dev/null || true
	@launchctl load ~/Library/LaunchAgents/com.typtel.menubar.plist
	@echo "Started Typtel. Check your menu bar!"

stop:
	@launchctl unload ~/Library/LaunchAgents/com.typtel.menubar.plist 2>/dev/null || true
	@echo "Stopped Typtel."

uninstall:
	@echo "Uninstalling..."
	@launchctl unload ~/Library/LaunchAgents/com.typtel.menubar.plist 2>/dev/null || true
	@rm -f ~/Library/LaunchAgents/com.typtel.menubar.plist
	@rm -f $(PREFIX)/bin/$(BINARY_NAME)
	@rm -f $(PREFIX)/bin/$(DAEMON_NAME)
	@rm -rf $(APP_DIR)/$(APP_NAME)
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

.PHONY: all build clean install uninstall test deps start stop app install-app install-app-user start-app stop-app uninstall-app

BINARY_NAME=typtel
MENUBAR_NAME=typtel-menubar
APP_NAME=Typtel.app
VERSION?=1.1.7
BUILD_DIR=build
PREFIX?=/usr/local

# Go build flags
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION)"

all: deps build

deps:
	go mod tidy
	go mod download

build: build-cli build-menubar

build-cli:
	@echo "Building CLI..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/typtel

build-menubar:
	@echo "Building menu bar app..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(MENUBAR_NAME) ./cmd/typtel-menubar

install: build
	@echo "Installing to $(PREFIX)/bin..."
	@mkdir -p $(PREFIX)/bin
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(PREFIX)/bin/
	@cp $(BUILD_DIR)/$(MENUBAR_NAME) $(PREFIX)/bin/
	@echo "Installing LaunchAgent..."
	@mkdir -p ~/Library/LaunchAgents
	@sed 's|BINARY_PATH|$(PREFIX)/bin/$(MENUBAR_NAME)|g' scripts/com.typtel.menubar.plist > ~/Library/LaunchAgents/com.typtel.menubar.plist
	@echo ""
	@echo "Done! To start (run WITHOUT sudo):"
	@echo "  make start"
	@echo ""
	@echo "IMPORTANT: Grant Accessibility permissions to $(PREFIX)/bin/$(MENUBAR_NAME) in:"
	@echo "  System Settings > Privacy & Security > Accessibility"

start:
	@launchctl unload ~/Library/LaunchAgents/com.typtel.menubar.plist 2>/dev/null || true
	@launchctl load ~/Library/LaunchAgents/com.typtel.menubar.plist
	@echo "Started typtel-menubar. Check your menu bar!"

stop:
	@launchctl unload ~/Library/LaunchAgents/com.typtel.menubar.plist 2>/dev/null || true
	@echo "Stopped typtel-menubar."

uninstall:
	@echo "Uninstalling..."
	@launchctl unload ~/Library/LaunchAgents/com.typtel.menubar.plist 2>/dev/null || true
	@rm -f ~/Library/LaunchAgents/com.typtel.menubar.plist
	@rm -f $(PREFIX)/bin/$(BINARY_NAME)
	@rm -f $(PREFIX)/bin/$(MENUBAR_NAME)
	@echo "Done!"

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@go clean

test:
	go test -v ./...

test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@open coverage.html

# Update word lists from external sources
update-wordlists:
	@echo "Updating word lists..."
	@mkdir -p internal/tui/wordlists
	@curl -sL "https://raw.githubusercontent.com/first20hours/google-10000-english/master/google-10000-english-no-swears.txt" -o internal/tui/wordlists/english_common.txt
	@curl -sL "https://www.eff.org/files/2016/07/18/eff_large_wordlist.txt" | awk '{print $$2}' > internal/tui/wordlists/eff_words.txt
	@echo "Word lists updated successfully!"
	@wc -l internal/tui/wordlists/*.txt

# Development helpers
run-cli: build-cli
	./$(BUILD_DIR)/$(BINARY_NAME)

run-menubar: build-menubar
	./$(BUILD_DIR)/$(MENUBAR_NAME)

# Build macOS .app bundle (openable from Finder/Spotlight)
app: build-menubar
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)/$(APP_NAME)/Contents/MacOS
	@mkdir -p $(BUILD_DIR)/$(APP_NAME)/Contents/Resources
	@cp $(BUILD_DIR)/$(MENUBAR_NAME) $(BUILD_DIR)/$(APP_NAME)/Contents/MacOS/
	@cp assets/AppIcon.icns $(BUILD_DIR)/$(APP_NAME)/Contents/Resources/AppIcon.icns
	@sed 's/__VERSION__/$(VERSION)/g' scripts/Info.plist > $(BUILD_DIR)/$(APP_NAME)/Contents/Info.plist
	@echo "Built $(BUILD_DIR)/$(APP_NAME)"

# Install app to /Applications (requires sudo for system-wide, or use ~/Applications)
install-app: app
	@echo "Installing $(APP_NAME) to /Applications..."
	@rm -rf /Applications/$(APP_NAME)
	@cp -R $(BUILD_DIR)/$(APP_NAME) /Applications/
	@echo "Done! Typtel is now available in Finder and Spotlight."
	@echo ""
	@echo "IMPORTANT: Grant Accessibility permissions to Typtel in:"
	@echo "  System Settings > Privacy & Security > Accessibility"

# Install app to ~/Applications (no sudo required)
install-app-user: app
	@echo "Installing $(APP_NAME) to ~/Applications..."
	@mkdir -p ~/Applications
	@rm -rf ~/Applications/$(APP_NAME)
	@cp -R $(BUILD_DIR)/$(APP_NAME) ~/Applications/
	@echo "Installing LaunchAgent for auto-start..."
	@mkdir -p ~/Library/LaunchAgents
	@launchctl unload ~/Library/LaunchAgents/com.typtel.menubar.plist 2>/dev/null || true
	@sed 's|APP_BINARY_PATH|$(HOME)/Applications/$(APP_NAME)/Contents/MacOS/$(MENUBAR_NAME)|g' scripts/com.typtel.app.plist > ~/Library/LaunchAgents/com.typtel.menubar.plist
	@echo ""
	@echo "Done! Typtel is now available in Finder and Spotlight."
	@echo ""
	@echo "To start now: make start-app"
	@echo ""
	@echo "IMPORTANT: Grant Accessibility permissions to Typtel.app in:"
	@echo "  System Settings > Privacy & Security > Accessibility"
	@echo ""
	@echo "This is the ONLY app you need to grant permissions to."

# Start the app via LaunchAgent
start-app:
	@launchctl unload ~/Library/LaunchAgents/com.typtel.menubar.plist 2>/dev/null || true
	@launchctl load ~/Library/LaunchAgents/com.typtel.menubar.plist
	@echo "Started Typtel. Check your menu bar!"

# Stop the app
stop-app:
	@launchctl unload ~/Library/LaunchAgents/com.typtel.menubar.plist 2>/dev/null || true
	@echo "Stopped Typtel."

# Uninstall the app completely
uninstall-app:
	@echo "Uninstalling Typtel..."
	@launchctl unload ~/Library/LaunchAgents/com.typtel.menubar.plist 2>/dev/null || true
	@rm -f ~/Library/LaunchAgents/com.typtel.menubar.plist
	@rm -rf ~/Applications/$(APP_NAME)
	@rm -rf /Applications/$(APP_NAME)
	@echo "Done!"

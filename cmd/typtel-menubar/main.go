//go:build darwin
// +build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// We need to ensure we're on the main thread for all AppKit operations
static void ensureMainThread() {
    if (![NSThread isMainThread]) {
        dispatch_sync(dispatch_get_main_queue(), ^{});
    }
}
*/
import "C"

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/aayushbajaj/typing-telemetry/internal/keylogger"
	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/caseymrm/menuet"
)

var (
	store       *storage.Store
	appStarted  = make(chan struct{})
)

func init() {
	// Ensure main goroutine runs on the main OS thread (required for macOS UI)
	runtime.LockOSThread()
}

func main() {
	// Ensure we're on main thread
	C.ensureMainThread()

	// Set up logging
	logDir, err := getLogDir()
	if err != nil {
		log.Fatalf("Failed to get log directory: %v", err)
	}

	logFile, err := os.OpenFile(filepath.Join(logDir, "menubar.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	log.Println("Starting typtel menu bar app...")

	// Check accessibility permissions
	if !keylogger.CheckAccessibilityPermissions() {
		showPermissionAlert()
		os.Exit(1)
	}

	// Initialize storage
	store, err = storage.New()
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	// Start keylogger in background
	keystrokeChan, err := keylogger.Start()
	if err != nil {
		log.Fatalf("Failed to start keylogger: %v", err)
	}
	defer keylogger.Stop()

	// Process keystrokes in background
	go func() {
		for keycode := range keystrokeChan {
			if err := store.RecordKeystroke(keycode); err != nil {
				log.Printf("Failed to record keystroke: %v", err)
			}
		}
	}()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down...")
		os.Exit(0)
	}()

	// Configure menu bar app
	app := menuet.App()
	app.Label = "com.typtel.menubar"
	app.Children = menuItems

	// Start the update loop AFTER app.RunApplication starts
	// Use a timer to delay initial update
	go func() {
		// Wait for app to fully start
		time.Sleep(3 * time.Second)
		close(appStarted)

		// Now we can safely update
		updateMenuBarTitle()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			updateMenuBarTitle()
		}
	}()

	log.Println("Menu bar app starting...")

	// This blocks and runs the macOS event loop
	// All UI must happen from here on the main thread
	app.RunApplication()
}

func updateMenuBarTitle() {
	stats, err := store.GetTodayStats()
	if err != nil {
		menuet.App().SetMenuState(&menuet.MenuState{
			Title: "⌨️ --",
		})
		return
	}

	title := fmt.Sprintf("⌨️ %s", formatCompact(stats.Keystrokes))
	menuet.App().SetMenuState(&menuet.MenuState{
		Title: title,
	})
}

func menuItems() []menuet.MenuItem {
	stats, _ := store.GetTodayStats()
	weekStats, _ := store.GetWeekStats()

	var weekTotal int64
	if weekStats != nil {
		for _, day := range weekStats {
			weekTotal += day.Keystrokes
		}
	}

	keystrokeCount := int64(0)
	if stats != nil {
		keystrokeCount = stats.Keystrokes
	}

	return []menuet.MenuItem{
		{
			Text: fmt.Sprintf("Today: %s keystrokes", formatNumber(keystrokeCount)),
		},
		{
			Text: fmt.Sprintf("This Week: %s keystrokes", formatNumber(weekTotal)),
		},
		{
			Type: menuet.Separator,
		},
		{
			Text:    "Quit",
			Clicked: quit,
		},
	}
}

func quit() {
	keylogger.Stop()
	if store != nil {
		store.Close()
	}
	os.Exit(0)
}

func showPermissionAlert() {
	fmt.Println("ERROR: Accessibility permissions not granted.")
	fmt.Println("")
	fmt.Println("To enable:")
	fmt.Println("1. Open System Preferences > Privacy & Security > Accessibility")
	fmt.Println("2. Click the lock to make changes")
	fmt.Println("3. Add this application to the list")
	fmt.Println("4. Restart the application")
}

func getLogDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	logDir := filepath.Join(home, ".local", "share", "typtel", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", err
	}
	return logDir, nil
}

func formatNumber(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func formatCompact(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.0fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

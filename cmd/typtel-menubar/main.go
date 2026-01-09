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

// Check if we're on the main thread
static bool isMainThread() {
    return [NSThread isMainThread];
}

// Force a menubar refresh - helps when display configuration changes
static void refreshMenuBar() {
    dispatch_async(dispatch_get_main_queue(), ^{
        // Touch the status bar to force a redraw
        [[NSApp mainMenu] update];
    });
}
*/
import "C"

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aayushbajaj/typing-telemetry/internal/inertia"
	"github.com/aayushbajaj/typing-telemetry/internal/keylogger"
	"github.com/aayushbajaj/typing-telemetry/internal/mousetracker"
	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/caseymrm/menuet"
)

var (
	store          *storage.Store
	appStarted     = make(chan struct{})
	lastMenuTitle  string // Track last title to avoid unnecessary updates
	menuTitleMutex sync.Mutex
)

func init() {
	// Ensure main goroutine runs on the main OS thread (required for macOS UI)
	runtime.LockOSThread()
}

func main() {
	// Ensure we're on main thread
	C.ensureMainThread()

	// Ensure HOME is set (needed when launched via launchctl/open)
	if os.Getenv("HOME") == "" {
		if u, err := user.Current(); err == nil {
			os.Setenv("HOME", u.HomeDir)
		}
	}

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
	// Track word boundaries: space (49), return (36), tab (48) indicate end of word
	go func() {
		for keycode := range keystrokeChan {
			if err := store.RecordKeystroke(keycode); err != nil {
				log.Printf("Failed to record keystroke: %v", err)
			}
			// Detect word boundaries - increment word count when word-ending keys are pressed
			if isWordBoundary(keycode) {
				date := time.Now().Format("2006-01-02")
				if err := store.IncrementWordCount(date); err != nil {
					log.Printf("Failed to increment word count: %v", err)
				}
			}
		}
	}()

	// Start mouse tracker in background if enabled (uses same accessibility permissions)
	mouseTrackingEnabled := store.IsMouseTrackingEnabled()
	if mouseTrackingEnabled {
		mouseChan, clickChan, err := mousetracker.Start()
		if err != nil {
			log.Printf("Warning: Failed to start mouse tracker: %v", err)
			// Continue without mouse tracking - keylogger is more important
		} else {
			defer mousetracker.Stop()

			// Set initial midnight position for today
			pos := mousetracker.GetCurrentPosition()
			date := time.Now().Format("2006-01-02")
			if err := store.SetMidnightPosition(date, pos.X, pos.Y); err != nil {
				log.Printf("Failed to set midnight position: %v", err)
			}

			// Process mouse movements in background
			go func() {
				currentDate := time.Now().Format("2006-01-02")
				for movement := range mouseChan {
					// Check if we've crossed midnight
					newDate := time.Now().Format("2006-01-02")
					if newDate != currentDate {
						// New day - reset and set new midnight position
						currentDate = newDate
						if err := store.SetMidnightPosition(currentDate, movement.X, movement.Y); err != nil {
							log.Printf("Failed to set midnight position: %v", err)
						}
					}

					if err := store.RecordMouseMovement(movement.X, movement.Y, movement.Distance); err != nil {
						log.Printf("Failed to record mouse movement: %v", err)
					}
				}
			}()

			// Process mouse clicks in background
			go func() {
				for range clickChan {
					if err := store.RecordMouseClick(); err != nil {
						log.Printf("Failed to record mouse click: %v", err)
					}
				}
			}()
		}
	} else {
		log.Println("Mouse tracking is disabled")
	}

	// Start inertia system if enabled
	inertiaSettings := store.GetInertiaSettings()
	if inertiaSettings.Enabled {
		inertiaCfg := inertia.Config{
			Enabled:   true,
			MaxSpeed:  inertiaSettings.MaxSpeed,
			Threshold: inertiaSettings.Threshold,
			AccelRate: inertiaSettings.AccelRate,
		}
		if err := inertia.Start(inertiaCfg); err != nil {
			log.Printf("Warning: Failed to start inertia: %v", err)
		} else {
			log.Println("Inertia system started")
			defer inertia.Stop()
		}
	} else {
		log.Println("Inertia is disabled")
	}

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
		setMenuTitle("⌨️ --")
		return
	}

	// Get settings
	settings := store.GetMenubarSettings()

	// Get mouse stats
	mouseStats, _ := store.GetTodayMouseStats()

	// Build title based on settings
	var parts []string

	if settings.ShowKeystrokes {
		parts = append(parts, fmt.Sprintf("⌨️%s", formatAbsolute(stats.Keystrokes)))
	}
	if settings.ShowWords {
		parts = append(parts, fmt.Sprintf("%sw", formatAbsolute(stats.Words)))
	}
	if settings.ShowClicks && mouseStats != nil {
		parts = append(parts, fmt.Sprintf("🖱️%s", formatAbsolute(mouseStats.ClickCount)))
	}
	if settings.ShowDistance && mouseStats != nil && mouseStats.TotalDistance > 0 {
		parts = append(parts, formatDistance(mouseStats.TotalDistance))
	}

	title := "⌨️"
	if len(parts) > 0 {
		title = strings.Join(parts, " | ")
	}

	setMenuTitle(title)
}

// setMenuTitle updates the menu title only if it has changed
// This prevents unnecessary UI updates that can cause flickering
func setMenuTitle(title string) {
	menuTitleMutex.Lock()
	defer menuTitleMutex.Unlock()

	// Skip update if title hasn't changed
	if title == lastMenuTitle {
		return
	}

	lastMenuTitle = title

	// Use dispatch_async to ensure UI update happens on main thread
	// The menuet library should handle this, but we add extra safety
	menuet.App().SetMenuState(&menuet.MenuState{
		Title: title,
	})
}

// Version is set at build time via ldflags: -X main.Version=$(VERSION)
var Version = "dev"

func menuItems() []menuet.MenuItem {
	stats, _ := store.GetTodayStats()
	weekStats, _ := store.GetWeekStats()
	mouseStats, _ := store.GetTodayMouseStats()
	weekMouseStats, _ := store.GetWeekMouseStats()

	var weekKeystrokes, weekWords int64
	var weekMouseDistance float64
	var weekClicks int64
	if weekStats != nil {
		for _, day := range weekStats {
			weekKeystrokes += day.Keystrokes
			weekWords += day.Words
		}
	}
	if weekMouseStats != nil {
		for _, day := range weekMouseStats {
			weekMouseDistance += day.TotalDistance
			weekClicks += day.ClickCount
		}
	}

	keystrokeCount := int64(0)
	todayWords := int64(0)
	if stats != nil {
		keystrokeCount = stats.Keystrokes
		todayWords = stats.Words
	}

	todayMouseDistance := float64(0)
	todayClicks := int64(0)
	if mouseStats != nil {
		todayMouseDistance = mouseStats.TotalDistance
		todayClicks = mouseStats.ClickCount
	}

	return []menuet.MenuItem{
		{
			Text: fmt.Sprintf("Today: %s keystrokes (%s words)", formatAbsolute(keystrokeCount), formatAbsolute(todayWords)),
		},
		{
			Text: fmt.Sprintf("Today: 🖱️ %s clicks, %s distance", formatAbsolute(todayClicks), formatDistance(todayMouseDistance)),
		},
		{
			Type: menuet.Separator,
		},
		{
			Text: fmt.Sprintf("This Week: %s keystrokes (%s words)", formatAbsolute(weekKeystrokes), formatAbsolute(weekWords)),
		},
		{
			Text: fmt.Sprintf("This Week: 🖱️ %s clicks, %s distance", formatAbsolute(weekClicks), formatDistance(weekMouseDistance)),
		},
		{
			Type: menuet.Separator,
		},
		{
			Text:    "View Charts",
			Clicked: openCharts,
		},
		{
			Text:     "🏆 Stillness Leaderboard",
			Clicked:  showLeaderboard,
			Children: leaderboardMenuItems,
		},
		{
			Type: menuet.Separator,
		},
		{
			Text:     "⚙️ Settings",
			Children: settingsMenuItems,
		},
		{
			Text:    "About",
			Clicked: showAbout,
		},
		{
			Text:    "Quit",
			Clicked: quit,
		},
	}
}

func showAbout() {
	// Open GitHub page in browser
	go func() {
		cmd := exec.Command("open", "https://github.com/abaj8494/typing-telemetry")
		cmd.Run()
	}()

	// Show alert with version info
	menuet.App().Alert(menuet.Alert{
		MessageText:     "Typing Telemetry",
		InformativeText: fmt.Sprintf("Version %s\n\nTrack your keystrokes and typing speed.\n\nGitHub: github.com/abaj8494/typing-telemetry", Version),
		Buttons:         []string{"OK"},
	})
}

func quit() {
	response := menuet.App().Alert(menuet.Alert{
		MessageText:     "Quit Typing Telemetry",
		InformativeText: "Choose how to quit:",
		Buttons:         []string{"Cancel", "Hide Menu Bar Only", "Stop Tracking & Quit"},
	})

	switch response.Button {
	case 0: // Cancel
		return
	case 1: // Hide Menu Bar Only
		// Just exit the menubar app, keylogger keeps running in the background
		os.Exit(0)
	case 2: // Stop Tracking & Quit
		keylogger.Stop()
		if store != nil {
			store.Close()
		}
		os.Exit(0)
	}
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

// formatAbsolute formats a number with comma separators for readability
func formatAbsolute(n int64) string {
	// Convert to string and add commas
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return s
	}

	// Add commas every 3 digits from the right
	result := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

// formatDistance formats mouse distance based on the selected unit setting
// Pixels are converted to real-world units using the average display PPI
func formatDistance(pixels float64) string {
	// Convert pixels to feet using actual display PPI
	feet := mousetracker.PixelsToFeet(pixels)

	unit := store.GetDistanceUnit()
	switch unit {
	case storage.DistanceUnitCars:
		// Average car length ~15 feet
		cars := feet / 15.0
		if cars >= 1000 {
			return fmt.Sprintf("%.1fk cars", cars/1000)
		} else if cars >= 1 {
			return fmt.Sprintf("%.0f cars", cars)
		}
		return fmt.Sprintf("%.1f cars", cars)

	case storage.DistanceUnitFrisbee:
		// Ultimate frisbee field = 110 yards = 330 feet
		fields := feet / 330.0
		if fields >= 100 {
			return fmt.Sprintf("%.0f fields", fields)
		} else if fields >= 1 {
			return fmt.Sprintf("%.1f fields", fields)
		}
		return fmt.Sprintf("%.2f fields", fields)

	default: // feet/miles
		if feet >= 5280 { // 1 mile = 5280 feet
			return fmt.Sprintf("%.1fmi", feet/5280)
		} else if feet >= 1 {
			return fmt.Sprintf("%.0fft", feet)
		}
		inches := feet * 12
		return fmt.Sprintf("%.0fin", inches)
	}
}

// formatDistanceShort formats mouse distance for compact display
func formatDistanceShort(pixels float64) string {
	feet := mousetracker.PixelsToFeet(pixels)
	if feet >= 5280 {
		return fmt.Sprintf("%.1fmi", feet/5280)
	} else if feet >= 1 {
		return fmt.Sprintf("%.0fft", feet)
	}
	return fmt.Sprintf("%.0fin", feet*12)
}

func chartMenuItems() []menuet.MenuItem {
	return []menuet.MenuItem{
		{
			Text:    "Open Charts",
			Clicked: openCharts,
		},
	}
}

func settingsMenuItems() []menuet.MenuItem {
	settings := store.GetMenubarSettings()
	mouseTrackingEnabled := store.IsMouseTrackingEnabled()
	inertiaSettings := store.GetInertiaSettings()

	checkmark := func(enabled bool) string {
		if enabled {
			return "✓ "
		}
		return "   "
	}

	return []menuet.MenuItem{
		{
			Text: "Menu Bar Display:",
		},
		{
			Text: checkmark(settings.ShowKeystrokes) + "Show Keystrokes",
			Clicked: func() {
				s := store.GetMenubarSettings()
				s.ShowKeystrokes = !s.ShowKeystrokes
				store.SaveMenubarSettings(s)
				updateMenuBarTitle()
			},
		},
		{
			Text: checkmark(settings.ShowWords) + "Show Words",
			Clicked: func() {
				s := store.GetMenubarSettings()
				s.ShowWords = !s.ShowWords
				store.SaveMenubarSettings(s)
				updateMenuBarTitle()
			},
		},
		{
			Text: checkmark(settings.ShowClicks) + "Show Mouse Clicks",
			Clicked: func() {
				s := store.GetMenubarSettings()
				s.ShowClicks = !s.ShowClicks
				store.SaveMenubarSettings(s)
				updateMenuBarTitle()
			},
		},
		{
			Text: checkmark(settings.ShowDistance) + "Show Mouse Distance",
			Clicked: func() {
				s := store.GetMenubarSettings()
				s.ShowDistance = !s.ShowDistance
				store.SaveMenubarSettings(s)
				updateMenuBarTitle()
			},
		},
		{
			Text:     "   Distance Unit",
			Children: distanceUnitMenuItems,
		},
		{
			Type: menuet.Separator,
		},
		{
			Text: "Tracking:",
		},
		{
			Text: checkmark(mouseTrackingEnabled) + "Enable Mouse Distance",
			Clicked: func() {
				enabled := store.IsMouseTrackingEnabled()
				store.SetMouseTrackingEnabled(!enabled)
				// Show restart message
				menuet.App().Alert(menuet.Alert{
					MessageText:     "Mouse Distance " + map[bool]string{true: "Disabled", false: "Enabled"}[enabled],
					InformativeText: "Restart the app for changes to take effect.",
					Buttons:         []string{"OK"},
				})
			},
		},
		{
			Type: menuet.Separator,
		},
		{
			Text: "⚡ Inertia (Key Acceleration):",
		},
		{
			Text: checkmark(inertiaSettings.Enabled) + "Enable Inertia",
			Clicked: func() {
				s := store.GetInertiaSettings()
				newEnabled := !s.Enabled
				store.SetInertiaEnabled(newEnabled)

				// Update inertia system in real-time
				if newEnabled {
					cfg := inertia.Config{
						Enabled:   true,
						MaxSpeed:  s.MaxSpeed,
						Threshold: s.Threshold,
						AccelRate: s.AccelRate,
					}
					inertia.Start(cfg)
				} else {
					inertia.Stop()
				}

				menuet.App().Alert(menuet.Alert{
					MessageText:     "Inertia " + map[bool]string{true: "Enabled", false: "Disabled"}[newEnabled],
					InformativeText: "Key acceleration is now " + map[bool]string{true: "active", false: "inactive"}[newEnabled] + ".\n\nHold any key to accelerate repeat speed.\nPressing any other key resets acceleration.",
					Buttons:         []string{"OK"},
				})
			},
		},
		{
			Text:     "   Max Speed",
			Children: inertiaMaxSpeedMenuItems,
		},
		{
			Text:     "   Threshold (ms)",
			Children: inertiaThresholdMenuItems,
		},
		{
			Text:     "   Acceleration Rate",
			Children: inertiaAccelRateMenuItems,
		},
		{
			Type: menuet.Separator,
		},
		{
			Text: "🔧 Debug:",
		},
		{
			Text:    "   Show Display Info",
			Clicked: showDisplayDebugInfo,
		},
	}
}

func showDisplayDebugInfo() {
	ppi := mousetracker.GetAveragePPI()
	displayCount := mousetracker.GetDisplayCount()

	info := fmt.Sprintf("Displays Detected: %d\nAverage PPI: %.1f\n\n", displayCount, ppi)

	if ppi == mousetracker.DefaultPPI {
		info += "Note: Using fallback PPI (100).\nYour displays may not report physical dimensions."
	} else {
		info += fmt.Sprintf("Mouse distance is calculated using %.1f pixels per inch.", ppi)
	}

	menuet.App().Alert(menuet.Alert{
		MessageText:     "Display Information",
		InformativeText: info,
		Buttons:         []string{"OK"},
	})
}

// inertiaMaxSpeedMenuItems returns the max speed selection submenu
func inertiaMaxSpeedMenuItems() []menuet.MenuItem {
	currentSpeed := store.GetInertiaSettings().MaxSpeed

	checkmark := func(speed string) string {
		if currentSpeed == speed {
			return "✓ "
		}
		return "   "
	}

	return []menuet.MenuItem{
		{
			Text: checkmark(storage.InertiaSpeedUltraFast) + "Ultra Fast (~140 keys/sec)",
			Clicked: func() {
				store.SetInertiaMaxSpeed(storage.InertiaSpeedUltraFast)
				updateInertiaConfig()
			},
		},
		{
			Text: checkmark(storage.InertiaSpeedVeryFast) + "Very Fast (~125 keys/sec)",
			Clicked: func() {
				store.SetInertiaMaxSpeed(storage.InertiaSpeedVeryFast)
				updateInertiaConfig()
			},
		},
		{
			Text: checkmark(storage.InertiaSpeedFast) + "Fast (~83 keys/sec)",
			Clicked: func() {
				store.SetInertiaMaxSpeed(storage.InertiaSpeedFast)
				updateInertiaConfig()
			},
		},
		{
			Text: checkmark(storage.InertiaSpeedMedium) + "Medium (~50 keys/sec)",
			Clicked: func() {
				store.SetInertiaMaxSpeed(storage.InertiaSpeedMedium)
				updateInertiaConfig()
			},
		},
		{
			Text: checkmark(storage.InertiaSpeedSlow) + "Slow (~20 keys/sec)",
			Clicked: func() {
				store.SetInertiaMaxSpeed(storage.InertiaSpeedSlow)
				updateInertiaConfig()
			},
		},
	}
}

// inertiaThresholdMenuItems returns the threshold selection submenu
func inertiaThresholdMenuItems() []menuet.MenuItem {
	currentThreshold := store.GetInertiaSettings().Threshold

	checkmark := func(threshold int) string {
		if currentThreshold == threshold {
			return "✓ "
		}
		return "   "
	}

	return []menuet.MenuItem{
		{
			Text: checkmark(100) + "100ms (instant)",
			Clicked: func() {
				store.SetInertiaThreshold(100)
				updateInertiaConfig()
			},
		},
		{
			Text: checkmark(150) + "150ms (fast)",
			Clicked: func() {
				store.SetInertiaThreshold(150)
				updateInertiaConfig()
			},
		},
		{
			Text: checkmark(200) + "200ms (default)",
			Clicked: func() {
				store.SetInertiaThreshold(200)
				updateInertiaConfig()
			},
		},
		{
			Text: checkmark(250) + "250ms (slow)",
			Clicked: func() {
				store.SetInertiaThreshold(250)
				updateInertiaConfig()
			},
		},
		{
			Text: checkmark(350) + "350ms (very slow)",
			Clicked: func() {
				store.SetInertiaThreshold(350)
				updateInertiaConfig()
			},
		},
	}
}

// inertiaAccelRateMenuItems returns the acceleration rate selection submenu
func inertiaAccelRateMenuItems() []menuet.MenuItem {
	currentRate := store.GetInertiaSettings().AccelRate

	checkmark := func(rate float64) string {
		if currentRate == rate {
			return "✓ "
		}
		return "   "
	}

	return []menuet.MenuItem{
		{
			Text: checkmark(0.25) + "0.25x (very gentle)",
			Clicked: func() {
				store.SetInertiaAccelRate(0.25)
				updateInertiaConfig()
			},
		},
		{
			Text: checkmark(0.5) + "0.5x (gentle)",
			Clicked: func() {
				store.SetInertiaAccelRate(0.5)
				updateInertiaConfig()
			},
		},
		{
			Text: checkmark(1.0) + "1.0x (default)",
			Clicked: func() {
				store.SetInertiaAccelRate(1.0)
				updateInertiaConfig()
			},
		},
		{
			Text: checkmark(1.5) + "1.5x (faster)",
			Clicked: func() {
				store.SetInertiaAccelRate(1.5)
				updateInertiaConfig()
			},
		},
		{
			Text: checkmark(2.0) + "2.0x (aggressive)",
			Clicked: func() {
				store.SetInertiaAccelRate(2.0)
				updateInertiaConfig()
			},
		},
	}
}

// updateInertiaConfig updates the running inertia system with new settings
func updateInertiaConfig() {
	s := store.GetInertiaSettings()
	if s.Enabled {
		cfg := inertia.Config{
			Enabled:   true,
			MaxSpeed:  s.MaxSpeed,
			Threshold: s.Threshold,
			AccelRate: s.AccelRate,
		}
		inertia.UpdateConfig(cfg)
	}
}

// distanceUnitMenuItems returns the distance unit selection submenu
func distanceUnitMenuItems() []menuet.MenuItem {
	currentUnit := store.GetDistanceUnit()

	checkmark := func(unit string) string {
		if currentUnit == unit {
			return "✓ "
		}
		return "   "
	}

	return []menuet.MenuItem{
		{
			Text: checkmark(storage.DistanceUnitFeet) + "Feet / Miles",
			Clicked: func() {
				store.SetDistanceUnit(storage.DistanceUnitFeet)
				updateMenuBarTitle()
			},
		},
		{
			Text: checkmark(storage.DistanceUnitCars) + "Cars (15ft each)",
			Clicked: func() {
				store.SetDistanceUnit(storage.DistanceUnitCars)
				updateMenuBarTitle()
			},
		},
		{
			Text: checkmark(storage.DistanceUnitFrisbee) + "Frisbee Fields (330ft)",
			Clicked: func() {
				store.SetDistanceUnit(storage.DistanceUnitFrisbee)
				updateMenuBarTitle()
			},
		},
	}
}

// leaderboardMenuItems returns the stillness leaderboard submenu
func leaderboardMenuItems() []menuet.MenuItem {
	entries, err := store.GetMouseLeaderboard(10)
	if err != nil || len(entries) == 0 {
		return []menuet.MenuItem{
			{Text: "No data yet - keep tracking!"},
		}
	}

	items := make([]menuet.MenuItem, 0, len(entries)+1)
	items = append(items, menuet.MenuItem{
		Text: "🧘 Days You Didn't Move The Mouse",
	})

	for _, entry := range entries {
		t, _ := time.Parse("2006-01-02", entry.Date)
		medal := ""
		switch entry.Rank {
		case 1:
			medal = "🥇 "
		case 2:
			medal = "🥈 "
		case 3:
			medal = "🥉 "
		}
		items = append(items, menuet.MenuItem{
			Text: fmt.Sprintf("%s#%d: %s - %s", medal, entry.Rank, t.Format("Jan 2, 2006"), formatDistance(entry.TotalDistance)),
		})
	}

	return items
}

func showLeaderboard() {
	// Open the leaderboard view in charts
	go func() {
		htmlPath, err := generateLeaderboardHTML()
		if err != nil {
			log.Printf("Failed to generate leaderboard: %v", err)
			return
		}
		cmd := exec.Command("open", htmlPath)
		if err := cmd.Run(); err != nil {
			log.Printf("Failed to open leaderboard: %v", err)
		}
	}()
}

func generateLeaderboardHTML() (string, error) {
	entries, err := store.GetMouseLeaderboard(30)
	if err != nil {
		return "", err
	}

	var rows strings.Builder
	for _, entry := range entries {
		t, _ := time.Parse("2006-01-02", entry.Date)
		medal := ""
		switch entry.Rank {
		case 1:
			medal = "🥇"
		case 2:
			medal = "🥈"
		case 3:
			medal = "🥉"
		default:
			medal = fmt.Sprintf("#%d", entry.Rank)
		}
		rows.WriteString(fmt.Sprintf(`
			<tr>
				<td class="rank">%s</td>
				<td class="date">%s</td>
				<td class="distance">%s</td>
			</tr>`, medal, t.Format("Monday, Jan 2, 2006"), formatDistance(entry.TotalDistance)))
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Typtel - Stillness Leaderboard</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%%, #16213e 100%%);
            color: #eee;
            min-height: 100vh;
            padding: 30px;
        }
        h1 {
            text-align: center;
            margin-bottom: 10px;
            font-size: 2.5em;
            background: linear-gradient(90deg, #ff6b6b, #feca57);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .subtitle {
            text-align: center;
            color: #888;
            margin-bottom: 30px;
            font-size: 1.1em;
        }
        .leaderboard-container {
            max-width: 800px;
            margin: 0 auto;
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 25px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        table {
            width: 100%%;
            border-collapse: collapse;
        }
        th {
            text-align: left;
            padding: 15px;
            border-bottom: 2px solid rgba(255,255,255,0.2);
            color: #888;
            font-size: 0.9em;
            text-transform: uppercase;
        }
        td {
            padding: 15px;
            border-bottom: 1px solid rgba(255,255,255,0.05);
        }
        tr:hover {
            background: rgba(255,255,255,0.05);
        }
        .rank {
            font-size: 1.5em;
            width: 80px;
        }
        .date {
            color: #aaa;
        }
        .distance {
            text-align: right;
            font-weight: bold;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .explanation {
            margin-top: 30px;
            padding: 20px;
            background: rgba(255,255,255,0.03);
            border-radius: 10px;
            color: #888;
            font-size: 0.9em;
            line-height: 1.6;
        }
    </style>
</head>
<body>
    <h1>🧘 Stillness Leaderboard</h1>
    <p class="subtitle">Days You Didn't Move The Mouse (Much)</p>

    <div class="leaderboard-container">
        <table>
            <thead>
                <tr>
                    <th>Rank</th>
                    <th>Date</th>
                    <th>Distance</th>
                </tr>
            </thead>
            <tbody>
                %s
            </tbody>
        </table>

        <div class="explanation">
            <strong>What is this?</strong><br>
            This leaderboard tracks the days when you moved your mouse the least. Less mouse movement
            could indicate focused keyboard work, reading, or meditation sessions. The distance is
            calculated as the total Euclidean distance your cursor traveled throughout the day,
            converted to approximate real-world measurements in feet (assuming ~100 DPI display).
        </div>
    </div>
</body>
</html>`, rows.String())

	// Write to temp file
	dataDir, err := getLogDir()
	if err != nil {
		return "", err
	}
	htmlPath := filepath.Join(dataDir, "leaderboard.html")
	if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
		return "", err
	}

	return htmlPath, nil
}

func openCharts() {
	go func() {
		htmlPath, err := generateChartsHTML()
		if err != nil {
			log.Printf("Failed to generate charts: %v", err)
			return
		}

		cmd := exec.Command("open", htmlPath)
		if err := cmd.Run(); err != nil {
			log.Printf("Failed to open charts: %v", err)
		}
	}()
}

func generateChartsHTML() (string, error) {
	// Helper to prepare chart data for a given number of days
	prepareChartData := func(days int) (labels, keystrokeData, wordData []string, mouseDataFeet []float64, totalKeystrokes, totalWords int64, totalMouseDistance float64, heatmapHTML string, err error) {
		histStats, err := store.GetHistoricalStats(days)
		if err != nil {
			return nil, nil, nil, nil, 0, 0, 0, "", err
		}

		mouseStats, err := store.GetMouseHistoricalStats(days)
		if err != nil {
			return nil, nil, nil, nil, 0, 0, 0, "", err
		}

		hourlyData, err := store.GetAllHourlyStatsForDays(days)
		if err != nil {
			return nil, nil, nil, nil, 0, 0, 0, "", err
		}

		for i, stat := range histStats {
			t, _ := time.Parse("2006-01-02", stat.Date)
			labels = append(labels, fmt.Sprintf("'%s'", t.Format("Jan 2")))
			keystrokeData = append(keystrokeData, fmt.Sprintf("%d", stat.Keystrokes))
			wordData = append(wordData, fmt.Sprintf("%d", stat.Words))
			totalKeystrokes += stat.Keystrokes
			totalWords += stat.Words

			if i < len(mouseStats) {
				feet := mousetracker.PixelsToFeet(mouseStats[i].TotalDistance)
				mouseDataFeet = append(mouseDataFeet, feet)
				totalMouseDistance += mouseStats[i].TotalDistance
			} else {
				mouseDataFeet = append(mouseDataFeet, 0)
			}
		}

		heatmapHTML = generateHeatmapHTML(hourlyData, days)
		return
	}

	// Get data for both weekly and monthly views
	weeklyLabels, weeklyKeystrokes, weeklyWords, weeklyMouseFeet, weeklyTotalKeys, weeklyTotalWords, weeklyTotalMouse, weeklyHeatmap, err := prepareChartData(7)
	if err != nil {
		return "", err
	}

	monthlyLabels, monthlyKeystrokes, monthlyWords, monthlyMouseFeet, monthlyTotalKeys, monthlyTotalWords, monthlyTotalMouse, monthlyHeatmap, err := prepareChartData(30)
	if err != nil {
		return "", err
	}

	// Convert mouse feet to JSON arrays for each unit
	formatMouseData := func(feetData []float64, divisor float64) string {
		var result []string
		for _, f := range feetData {
			result = append(result, fmt.Sprintf("%.2f", f/divisor))
		}
		return strings.Join(result, ",")
	}

	weeklyMouseFeetStr := formatMouseData(weeklyMouseFeet, 1.0)
	weeklyMouseCarsStr := formatMouseData(weeklyMouseFeet, 15.0)
	weeklyMouseFieldsStr := formatMouseData(weeklyMouseFeet, 330.0)
	monthlyMouseFeetStr := formatMouseData(monthlyMouseFeet, 1.0)
	monthlyMouseCarsStr := formatMouseData(monthlyMouseFeet, 15.0)
	monthlyMouseFieldsStr := formatMouseData(monthlyMouseFeet, 330.0)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Typtel - Typing Statistics</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%%, #16213e 100%%);
            color: #eee;
            min-height: 100vh;
            padding: 30px;
        }
        h1 {
            text-align: center;
            margin-bottom: 10px;
            font-size: 2.5em;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .controls {
            display: flex;
            justify-content: center;
            gap: 20px;
            margin-bottom: 30px;
        }
        .control-group {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .control-group label {
            color: #888;
            font-size: 0.9em;
        }
        select {
            background: rgba(255,255,255,0.1);
            border: 1px solid rgba(255,255,255,0.2);
            border-radius: 8px;
            color: #eee;
            padding: 8px 16px;
            font-size: 0.9em;
            cursor: pointer;
        }
        select:hover {
            background: rgba(255,255,255,0.15);
        }
        .charts-container {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 30px;
            max-width: 1400px;
            margin: 0 auto 40px;
        }
        .chart-box {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 25px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        .chart-box h2 {
            margin-bottom: 20px;
            font-size: 1.3em;
            color: #aaa;
        }
        .heatmap-container {
            max-width: 1400px;
            margin: 0 auto;
        }
        .heatmap-box {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 25px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        .heatmap-box h2 {
            margin-bottom: 20px;
            font-size: 1.3em;
            color: #aaa;
        }
        .heatmap {
            display: flex;
            flex-direction: column;
            gap: 3px;
        }
        .heatmap-row {
            display: flex;
            align-items: center;
            gap: 3px;
        }
        .heatmap-label {
            width: 70px;
            font-size: 11px;
            color: #888;
            text-align: right;
            padding-right: 10px;
        }
        .heatmap-cell {
            width: 20px;
            height: 20px;
            border-radius: 3px;
            transition: transform 0.2s;
        }
        .heatmap-cell:hover {
            transform: scale(1.3);
            z-index: 10;
        }
        .hour-labels {
            display: flex;
            gap: 3px;
            margin-left: 80px;
            margin-bottom: 5px;
        }
        .hour-label {
            width: 20px;
            font-size: 10px;
            color: #666;
            text-align: center;
        }
        .legend {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 8px;
            margin-top: 20px;
        }
        .legend-text { color: #666; font-size: 12px; }
        .legend-box {
            width: 15px;
            height: 15px;
            border-radius: 2px;
        }
        .stats-summary {
            display: flex;
            justify-content: center;
            gap: 40px;
            margin: 30px 0;
        }
        .stat-item {
            text-align: center;
        }
        .stat-value {
            font-size: 2.5em;
            font-weight: bold;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .stat-label {
            color: #888;
            font-size: 0.9em;
        }
    </style>
</head>
<body>
    <h1>Typtel Statistics</h1>

    <div class="controls">
        <div class="control-group">
            <label>Time Period:</label>
            <select id="periodSelect" onchange="updateCharts()">
                <option value="weekly">Weekly (7 days)</option>
                <option value="monthly">Monthly (30 days)</option>
            </select>
        </div>
        <div class="control-group">
            <label>Distance Unit:</label>
            <select id="unitSelect" onchange="updateCharts()">
                <option value="feet">Feet</option>
                <option value="cars">Car Lengths (~15ft)</option>
                <option value="fields">Frisbee Fields (~330ft)</option>
            </select>
        </div>
    </div>

    <div class="stats-summary">
        <div class="stat-item">
            <div class="stat-value" id="totalKeystrokes">-</div>
            <div class="stat-label">Total Keystrokes</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalWords">-</div>
            <div class="stat-label">Words</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="avgKeystrokes">-</div>
            <div class="stat-label">Avg Keystrokes/Day</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalMouse">-</div>
            <div class="stat-label">Mouse Distance</div>
        </div>
    </div>

    <div class="charts-container">
        <div class="chart-box">
            <h2>Keystrokes per Day</h2>
            <canvas id="keystrokesChart"></canvas>
        </div>
        <div class="chart-box">
            <h2>Words per Day</h2>
            <canvas id="wordsChart"></canvas>
        </div>
    </div>

    <div class="charts-container">
        <div class="chart-box" style="grid-column: span 2;">
            <h2 id="mouseChartTitle">Mouse Distance per Day</h2>
            <canvas id="mouseChart"></canvas>
        </div>
    </div>

    <div class="heatmap-container">
        <div class="heatmap-box">
            <h2>Activity Heatmap (Hourly)</h2>
            <div class="hour-labels">
                %s
            </div>
            <div class="heatmap" id="heatmapContainer">
            </div>
            <div class="legend">
                <span class="legend-text">Less</span>
                <div class="legend-box" style="background: #1a1a2e;"></div>
                <div class="legend-box" style="background: #2d4a3e;"></div>
                <div class="legend-box" style="background: #3d6b4f;"></div>
                <div class="legend-box" style="background: #5a9a6f;"></div>
                <div class="legend-box" style="background: #7bc96f;"></div>
                <span class="legend-text">More</span>
            </div>
        </div>
    </div>

    <script>
        // Data for both periods
        const data = {
            weekly: {
                labels: [%s],
                keystrokes: [%s],
                words: [%s],
                mouse: { feet: [%s], cars: [%s], fields: [%s] },
                totalKeystrokes: %d,
                totalWords: %d,
                totalMouseFeet: %.2f,
                days: 7,
                heatmap: `+"`%s`"+`
            },
            monthly: {
                labels: [%s],
                keystrokes: [%s],
                words: [%s],
                mouse: { feet: [%s], cars: [%s], fields: [%s] },
                totalKeystrokes: %d,
                totalWords: %d,
                totalMouseFeet: %.2f,
                days: 30,
                heatmap: `+"`%s`"+`
            }
        };

        const unitLabels = { feet: 'feet', cars: 'car lengths', fields: 'frisbee fields' };

        let keystrokesChart, wordsChart, mouseChart;

        const chartConfig = {
            responsive: true,
            plugins: { legend: { display: false } },
            scales: {
                y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.1)' }, ticks: { color: '#888' } },
                x: { grid: { display: false }, ticks: { color: '#888' } }
            }
        };

        function formatNumber(n) {
            if (n >= 1000000) return (n/1000000).toFixed(1) + 'M';
            if (n >= 1000) return (n/1000).toFixed(1) + 'K';
            return n.toString();
        }

        function formatDistance(feet) {
            if (feet >= 5280) return (feet/5280).toFixed(2) + ' mi';
            return feet.toFixed(0) + ' ft';
        }

        function updateCharts() {
            const period = document.getElementById('periodSelect').value;
            const unit = document.getElementById('unitSelect').value;
            const d = data[period];

            // Update stats
            document.getElementById('totalKeystrokes').textContent = formatNumber(d.totalKeystrokes);
            document.getElementById('totalWords').textContent = formatNumber(d.totalWords);
            document.getElementById('avgKeystrokes').textContent = formatNumber(Math.round(d.totalKeystrokes / d.days));
            document.getElementById('totalMouse').textContent = formatDistance(d.totalMouseFeet);

            // Update mouse chart title
            document.getElementById('mouseChartTitle').textContent = 'Mouse Distance per Day (' + unitLabels[unit] + ')';

            // Destroy existing charts
            if (keystrokesChart) keystrokesChart.destroy();
            if (wordsChart) wordsChart.destroy();
            if (mouseChart) mouseChart.destroy();

            // Create new charts
            keystrokesChart = new Chart(document.getElementById('keystrokesChart'), {
                type: 'bar',
                data: {
                    labels: d.labels,
                    datasets: [{ data: d.keystrokes, backgroundColor: 'rgba(0, 210, 255, 0.6)', borderColor: 'rgba(0, 210, 255, 1)', borderWidth: 1, borderRadius: 4 }]
                },
                options: chartConfig
            });

            wordsChart = new Chart(document.getElementById('wordsChart'), {
                type: 'line',
                data: {
                    labels: d.labels,
                    datasets: [{ data: d.words, borderColor: 'rgba(122, 201, 111, 1)', backgroundColor: 'rgba(122, 201, 111, 0.2)', fill: true, tension: 0.4, pointRadius: 4, pointBackgroundColor: 'rgba(122, 201, 111, 1)' }]
                },
                options: chartConfig
            });

            mouseChart = new Chart(document.getElementById('mouseChart'), {
                type: 'bar',
                data: {
                    labels: d.labels,
                    datasets: [{ data: d.mouse[unit], backgroundColor: 'rgba(255, 107, 107, 0.6)', borderColor: 'rgba(255, 107, 107, 1)', borderWidth: 1, borderRadius: 4 }]
                },
                options: chartConfig
            });

            // Update heatmap
            document.getElementById('heatmapContainer').innerHTML = d.heatmap;
        }

        // Initialize
        updateCharts();
    </script>
</body>
</html>`,
		generateHourLabels(),
		// Weekly data
		strings.Join(weeklyLabels, ","),
		strings.Join(weeklyKeystrokes, ","),
		strings.Join(weeklyWords, ","),
		weeklyMouseFeetStr,
		weeklyMouseCarsStr,
		weeklyMouseFieldsStr,
		weeklyTotalKeys,
		weeklyTotalWords,
		mousetracker.PixelsToFeet(float64(weeklyTotalMouse)),
		weeklyHeatmap,
		// Monthly data
		strings.Join(monthlyLabels, ","),
		strings.Join(monthlyKeystrokes, ","),
		strings.Join(monthlyWords, ","),
		monthlyMouseFeetStr,
		monthlyMouseCarsStr,
		monthlyMouseFieldsStr,
		monthlyTotalKeys,
		monthlyTotalWords,
		mousetracker.PixelsToFeet(float64(monthlyTotalMouse)),
		monthlyHeatmap,
	)

	// Write to temp file
	dataDir, err := getLogDir()
	if err != nil {
		return "", err
	}
	htmlPath := filepath.Join(dataDir, "charts.html")
	if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
		return "", err
	}

	return htmlPath, nil
}

func generateHourLabels() string {
	var labels []string
	for h := 0; h < 24; h++ {
		if h%3 == 0 {
			labels = append(labels, fmt.Sprintf(`<div class="hour-label">%d</div>`, h))
		} else {
			labels = append(labels, `<div class="hour-label"></div>`)
		}
	}
	return strings.Join(labels, "\n                ")
}

func generateHeatmapHTML(hourlyData map[string][]HourlyStats, days int) string {
	// Find max value for color scaling
	var maxVal int64 = 1
	for _, hours := range hourlyData {
		for _, h := range hours {
			if h.Keystrokes > maxVal {
				maxVal = h.Keystrokes
			}
		}
	}

	// Sort dates
	dates := make([]string, 0, len(hourlyData))
	for date := range hourlyData {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	var rows []string
	for _, date := range dates {
		hours := hourlyData[date]
		t, _ := time.Parse("2006-01-02", date)
		dateLabel := t.Format("Mon Jan 2")

		var cells []string
		for _, h := range hours {
			color := getHeatmapColor(h.Keystrokes, maxVal)
			title := fmt.Sprintf("%s %d:00 - %d keystrokes", dateLabel, h.Hour, h.Keystrokes)
			cells = append(cells, fmt.Sprintf(
				`<div class="heatmap-cell" style="background: %s;" title="%s"></div>`,
				color, title,
			))
		}

		rows = append(rows, fmt.Sprintf(
			`<div class="heatmap-row"><div class="heatmap-label">%s</div>%s</div>`,
			dateLabel,
			strings.Join(cells, ""),
		))
	}

	return strings.Join(rows, "\n                ")
}

func getHeatmapColor(value, max int64) string {
	if value == 0 {
		return "#1a1a2e"
	}
	ratio := float64(value) / float64(max)
	if ratio < 0.25 {
		return "#2d4a3e"
	} else if ratio < 0.5 {
		return "#3d6b4f"
	} else if ratio < 0.75 {
		return "#5a9a6f"
	}
	return "#7bc96f"
}

type HourlyStats = storage.HourlyStats

// isWordBoundary checks if the keycode represents a word boundary (end of word)
// macOS keycodes: space=49, return=36, tab=48
func isWordBoundary(keycode int) bool {
	switch keycode {
	case 49: // Space
		return true
	case 36: // Return/Enter
		return true
	case 48: // Tab
		return true
	default:
		return false
	}
}

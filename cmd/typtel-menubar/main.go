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
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/aayushbajaj/typing-telemetry/internal/keylogger"
	"github.com/aayushbajaj/typing-telemetry/internal/mousetracker"
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

	menuet.App().SetMenuState(&menuet.MenuState{
		Title: title,
	})
}

const Version = "0.8.0"

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
			Text:     "View Charts",
			Clicked:  openCharts,
			Children: chartMenuItems,
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

// formatDistance formats mouse distance in a human-readable way (feet)
// Pixels are converted to approximate real-world units assuming ~100 DPI
func formatDistance(pixels float64) string {
	// Convert pixels to feet (assuming ~100 DPI, 1 inch = 100 pixels)
	// 100 pixels = 1 inch, 12 inches = 1 foot
	feet := pixels / 100.0 / 12.0

	if feet >= 5280 { // 1 mile = 5280 feet
		return fmt.Sprintf("%.1fmi", feet/5280)
	} else if feet >= 1 {
		return fmt.Sprintf("%.0fft", feet)
	} else {
		inches := feet * 12
		return fmt.Sprintf("%.0fin", inches)
	}
}

// formatDistanceShort formats mouse distance for compact display
func formatDistanceShort(pixels float64) string {
	feet := pixels / 100.0 / 12.0
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
			Text:    "Weekly Overview",
			Clicked: func() { openChartsWithDays(7) },
		},
		{
			Text:    "Monthly Overview",
			Clicked: func() { openChartsWithDays(30) },
		},
	}
}

func settingsMenuItems() []menuet.MenuItem {
	settings := store.GetMenubarSettings()
	mouseTrackingEnabled := store.IsMouseTrackingEnabled()

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
			Type: menuet.Separator,
		},
		{
			Text: "Tracking:",
		},
		{
			Text: checkmark(mouseTrackingEnabled) + "Enable Mouse Tracking",
			Clicked: func() {
				enabled := store.IsMouseTrackingEnabled()
				store.SetMouseTrackingEnabled(!enabled)
				// Show restart message
				menuet.App().Alert(menuet.Alert{
					MessageText:     "Mouse Tracking " + map[bool]string{true: "Disabled", false: "Enabled"}[enabled],
					InformativeText: "Restart the service for changes to take effect:\nbrew services restart typing-telemetry",
					Buttons:         []string{"OK"},
				})
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
	openChartsWithDays(14) // Default to 2 weeks
}

func openChartsWithDays(days int) {
	go func() {
		htmlPath, err := generateChartsHTML(days)
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

func generateChartsHTML(days int) (string, error) {
	// Get historical data
	histStats, err := store.GetHistoricalStats(days)
	if err != nil {
		return "", err
	}

	// Get mouse historical data
	mouseStats, err := store.GetMouseHistoricalStats(days)
	if err != nil {
		return "", err
	}

	// Get hourly data for heatmap
	hourlyData, err := store.GetAllHourlyStatsForDays(days)
	if err != nil {
		return "", err
	}

	// Prepare data for charts - use actual word counts
	var labels, keystrokeData, wordData, mouseData []string
	var totalKeystrokes, totalWords int64
	var totalMouseDistance float64
	for i, stat := range histStats {
		// Parse date to get short format
		t, _ := time.Parse("2006-01-02", stat.Date)
		labels = append(labels, fmt.Sprintf("'%s'", t.Format("Jan 2")))
		keystrokeData = append(keystrokeData, fmt.Sprintf("%d", stat.Keystrokes))
		wordData = append(wordData, fmt.Sprintf("%d", stat.Words))
		totalKeystrokes += stat.Keystrokes
		totalWords += stat.Words

		// Add mouse data (convert to feet for chart)
		if i < len(mouseStats) {
			feet := mouseStats[i].TotalDistance / 100.0 / 12.0
			mouseData = append(mouseData, fmt.Sprintf("%.1f", feet))
			totalMouseDistance += mouseStats[i].TotalDistance
		} else {
			mouseData = append(mouseData, "0")
		}
	}

	// Prepare heatmap data
	heatmapHTML := generateHeatmapHTML(hourlyData, days)

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
        .subtitle {
            text-align: center;
            color: #888;
            margin-bottom: 30px;
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
    <h1>⌨️ Typtel Statistics</h1>
    <p class="subtitle">Last %d days of typing activity</p>

    <div class="stats-summary">
        <div class="stat-item">
            <div class="stat-value">%s</div>
            <div class="stat-label">Total Keystrokes</div>
        </div>
        <div class="stat-item">
            <div class="stat-value">%s</div>
            <div class="stat-label">Words</div>
        </div>
        <div class="stat-item">
            <div class="stat-value">%s</div>
            <div class="stat-label">Daily Average</div>
        </div>
        <div class="stat-item">
            <div class="stat-value">%s</div>
            <div class="stat-label">Mouse Distance</div>
        </div>
    </div>

    <div class="charts-container">
        <div class="chart-box">
            <h2>📊 Keystrokes per Day</h2>
            <canvas id="keystrokesChart"></canvas>
        </div>
        <div class="chart-box">
            <h2>📝 Words per Day</h2>
            <canvas id="wordsChart"></canvas>
        </div>
    </div>

    <div class="charts-container">
        <div class="chart-box" style="grid-column: span 2;">
            <h2>🖱️ Mouse Distance per Day (feet)</h2>
            <canvas id="mouseChart"></canvas>
        </div>
    </div>

    <div class="heatmap-container">
        <div class="heatmap-box">
            <h2>🔥 Activity Heatmap (Hourly)</h2>
            <div class="hour-labels">
                %s
            </div>
            <div class="heatmap">
                %s
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
        const chartConfig = {
            responsive: true,
            plugins: {
                legend: { display: false }
            },
            scales: {
                y: {
                    beginAtZero: true,
                    grid: { color: 'rgba(255,255,255,0.1)' },
                    ticks: { color: '#888' }
                },
                x: {
                    grid: { display: false },
                    ticks: { color: '#888' }
                }
            }
        };

        new Chart(document.getElementById('keystrokesChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [{
                    data: [%s],
                    backgroundColor: 'rgba(0, 210, 255, 0.6)',
                    borderColor: 'rgba(0, 210, 255, 1)',
                    borderWidth: 1,
                    borderRadius: 4
                }]
            },
            options: chartConfig
        });

        new Chart(document.getElementById('wordsChart'), {
            type: 'line',
            data: {
                labels: [%s],
                datasets: [{
                    data: [%s],
                    borderColor: 'rgba(122, 201, 111, 1)',
                    backgroundColor: 'rgba(122, 201, 111, 0.2)',
                    fill: true,
                    tension: 0.4,
                    pointRadius: 4,
                    pointBackgroundColor: 'rgba(122, 201, 111, 1)'
                }]
            },
            options: chartConfig
        });

        new Chart(document.getElementById('mouseChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [{
                    data: [%s],
                    backgroundColor: 'rgba(255, 107, 107, 0.6)',
                    borderColor: 'rgba(255, 107, 107, 1)',
                    borderWidth: 1,
                    borderRadius: 4
                }]
            },
            options: chartConfig
        });
    </script>
</body>
</html>`,
		days,
		formatAbsolute(totalKeystrokes),
		formatAbsolute(totalWords),
		formatAbsolute(totalKeystrokes/int64(days)),
		formatDistance(totalMouseDistance),
		generateHourLabels(),
		heatmapHTML,
		strings.Join(labels, ","),
		strings.Join(keystrokeData, ","),
		strings.Join(labels, ","),
		strings.Join(wordData, ","),
		strings.Join(labels, ","),
		strings.Join(mouseData, ","),
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

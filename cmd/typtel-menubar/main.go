//go:build darwin
// +build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications

#import <Cocoa/Cocoa.h>

// Modern alert dialog using NSAlert
static int showAlert(const char* messageText, const char* informativeText, const char** buttons, int buttonCount) {
    __block int result = 0;

    void (^showAlertBlock)(void) = ^{
        @autoreleasepool {
            NSAlert *alert = [[NSAlert alloc] init];
            [alert setMessageText:[NSString stringWithUTF8String:messageText]];
            [alert setInformativeText:[NSString stringWithUTF8String:informativeText]];
            [alert setAlertStyle:NSAlertStyleInformational];

            for (int i = 0; i < buttonCount; i++) {
                [alert addButtonWithTitle:[NSString stringWithUTF8String:buttons[i]]];
            }

            NSModalResponse response = [alert runModal];
            result = (int)(response - NSAlertFirstButtonReturn);
        }
    };

    if ([NSThread isMainThread]) {
        showAlertBlock();
    } else {
        dispatch_sync(dispatch_get_main_queue(), showAlertBlock);
    }

    return result;
}

// Open URL in default browser (synchronous, returns success)
static int openURL(const char* url) {
    __block int success = 0;

    void (^openBlock)(void) = ^{
        @autoreleasepool {
            NSURL *nsurl = [NSURL URLWithString:[NSString stringWithUTF8String:url]];
            if (nsurl != nil) {
                success = [[NSWorkspace sharedWorkspace] openURL:nsurl] ? 1 : 0;
            }
        }
    };

    if ([NSThread isMainThread]) {
        openBlock();
    } else {
        dispatch_sync(dispatch_get_main_queue(), openBlock);
    }

    return success;
}

// Open file in default application (synchronous, returns success)
static int openFile(const char* path) {
    __block int success = 0;

    void (^openBlock)(void) = ^{
        @autoreleasepool {
            NSString *pathStr = [NSString stringWithUTF8String:path];
            NSURL *fileURL = [NSURL fileURLWithPath:pathStr];
            if (fileURL != nil) {
                NSWorkspaceOpenConfiguration *config = [NSWorkspaceOpenConfiguration configuration];
                dispatch_semaphore_t sem = dispatch_semaphore_create(0);

                [[NSWorkspace sharedWorkspace] openURL:fileURL
                    configuration:config
                    completionHandler:^(NSRunningApplication *app, NSError *error) {
                        success = (error == nil) ? 1 : 0;
                        dispatch_semaphore_signal(sem);
                    }];

                dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));
            }
        }
    };

    if ([NSThread isMainThread]) {
        openBlock();
    } else {
        dispatch_sync(dispatch_get_main_queue(), openBlock);
    }

    return success;
}
*/
import "C"

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/systray"
	"github.com/aayushbajaj/typing-telemetry/internal/inertia"
	"github.com/aayushbajaj/typing-telemetry/internal/keylogger"
	"github.com/aayushbajaj/typing-telemetry/internal/mousetracker"
	"github.com/aayushbajaj/typing-telemetry/internal/storage"
)

var (
	store          *storage.Store
	lastMenuTitle  string
	menuTitleMutex sync.Mutex
)

// Version is set at build time via ldflags: -X main.Version=$(VERSION)
var Version = "dev"

// Menu item references for dynamic updates
var (
	mTodayKeystrokes    *systray.MenuItem
	mTodayMouse         *systray.MenuItem
	mWeekKeystrokes     *systray.MenuItem
	mWeekMouse          *systray.MenuItem
	mShowKeystrokes     *systray.MenuItem
	mShowWords          *systray.MenuItem
	mShowClicks         *systray.MenuItem
	mShowDistance       *systray.MenuItem
	mDistanceFeet       *systray.MenuItem
	mDistanceCars       *systray.MenuItem
	mDistanceFields     *systray.MenuItem
	mMouseTracking      *systray.MenuItem
	mInertiaEnabled     *systray.MenuItem
	mInertiaUltraFast   *systray.MenuItem
	mInertiaVeryFast    *systray.MenuItem
	mInertiaFast        *systray.MenuItem
	mInertiaMedium      *systray.MenuItem
	mInertiaSlow        *systray.MenuItem
	mThreshold100       *systray.MenuItem
	mThreshold150       *systray.MenuItem
	mThreshold200       *systray.MenuItem
	mThreshold250       *systray.MenuItem
	mThreshold350       *systray.MenuItem
	mAccelRate025       *systray.MenuItem
	mAccelRate050       *systray.MenuItem
	mAccelRate100       *systray.MenuItem
	mAccelRate150       *systray.MenuItem
	mAccelRate200       *systray.MenuItem
	leaderboardItems    []*systray.MenuItem
	mLeaderboardHeader  *systray.MenuItem
	leaderboardSubmenus *systray.MenuItem
)

func init() {
	runtime.LockOSThread()
}

func main() {
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
	go func() {
		for keycode := range keystrokeChan {
			if err := store.RecordKeystroke(keycode); err != nil {
				log.Printf("Failed to record keystroke: %v", err)
			}
			if isWordBoundary(keycode) {
				date := time.Now().Format("2006-01-02")
				if err := store.IncrementWordCount(date); err != nil {
					log.Printf("Failed to increment word count: %v", err)
				}
			}
		}
	}()

	// Start mouse tracker if enabled
	mouseTrackingEnabled := store.IsMouseTrackingEnabled()
	if mouseTrackingEnabled {
		mouseChan, clickChan, err := mousetracker.Start()
		if err != nil {
			log.Printf("Warning: Failed to start mouse tracker: %v", err)
		} else {
			defer mousetracker.Stop()

			pos := mousetracker.GetCurrentPosition()
			date := time.Now().Format("2006-01-02")
			if err := store.SetMidnightPosition(date, pos.X, pos.Y); err != nil {
				log.Printf("Failed to set midnight position: %v", err)
			}

			go func() {
				currentDate := time.Now().Format("2006-01-02")
				for movement := range mouseChan {
					newDate := time.Now().Format("2006-01-02")
					if newDate != currentDate {
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
		systray.Quit()
	}()

	log.Println("Menu bar app starting...")

	// Run the systray app
	systray.Run(onReady, onExit)
}

func onReady() {
	// Set initial title
	systray.SetTitle("⌨️")
	systray.SetTooltip("Typing Telemetry")

	// Build the menu structure
	buildMenu()

	// Start update loop
	go func() {
		// Initial update after a short delay
		time.Sleep(1 * time.Second)
		updateMenuBarTitle()
		updateStatsDisplay()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			updateMenuBarTitle()
			updateStatsDisplay()
		}
	}()
}

func onExit() {
	log.Println("Systray exiting...")
}

func buildMenu() {
	// Today's stats
	mTodayKeystrokes = systray.AddMenuItem("Today: -- keystrokes (-- words)", "")
	mTodayKeystrokes.Disable()
	mTodayMouse = systray.AddMenuItem("Today: 🖱️ -- clicks, -- distance", "")
	mTodayMouse.Disable()

	systray.AddSeparator()

	// Week stats
	mWeekKeystrokes = systray.AddMenuItem("This Week: -- keystrokes (-- words)", "")
	mWeekKeystrokes.Disable()
	mWeekMouse = systray.AddMenuItem("This Week: 🖱️ -- clicks, -- distance", "")
	mWeekMouse.Disable()

	systray.AddSeparator()

	// View Charts
	mCharts := systray.AddMenuItem("View Charts", "Open statistics charts")
	go func() {
		for range mCharts.ClickedCh {
			openCharts()
		}
	}()

	// Leaderboard submenu
	leaderboardSubmenus = systray.AddMenuItem("🏆 Stillness Leaderboard", "Days with least mouse movement")
	mLeaderboardHeader = leaderboardSubmenus.AddSubMenuItem("🧘 Days You Didn't Move The Mouse", "")
	mLeaderboardHeader.Disable()
	// Pre-allocate leaderboard slots
	for i := 0; i < 10; i++ {
		item := leaderboardSubmenus.AddSubMenuItem("", "")
		item.Hide()
		leaderboardItems = append(leaderboardItems, item)
	}
	go func() {
		for range leaderboardSubmenus.ClickedCh {
			showLeaderboard()
		}
	}()

	systray.AddSeparator()

	// Settings submenu
	mSettings := systray.AddMenuItem("⚙️ Settings", "Configure display options")

	// Menu Bar Display section
	mDisplayLabel := mSettings.AddSubMenuItem("Menu Bar Display:", "")
	mDisplayLabel.Disable()

	settings := store.GetMenubarSettings()

	mShowKeystrokes = mSettings.AddSubMenuItemCheckbox("Show Keystrokes", "", settings.ShowKeystrokes)
	mShowWords = mSettings.AddSubMenuItemCheckbox("Show Words", "", settings.ShowWords)
	mShowClicks = mSettings.AddSubMenuItemCheckbox("Show Mouse Clicks", "", settings.ShowClicks)
	mShowDistance = mSettings.AddSubMenuItemCheckbox("Show Mouse Distance", "", settings.ShowDistance)

	// Distance Unit submenu
	mDistanceUnit := mSettings.AddSubMenuItem("   Distance Unit", "")
	currentUnit := store.GetDistanceUnit()
	mDistanceFeet = mDistanceUnit.AddSubMenuItemCheckbox("Feet / Miles", "", currentUnit == storage.DistanceUnitFeet)
	mDistanceCars = mDistanceUnit.AddSubMenuItemCheckbox("Cars (15ft each)", "", currentUnit == storage.DistanceUnitCars)
	mDistanceFields = mDistanceUnit.AddSubMenuItemCheckbox("Frisbee Fields (330ft)", "", currentUnit == storage.DistanceUnitFrisbee)

	mSettings.AddSubMenuItem("", "").Disable() // Separator

	// Tracking section
	mTrackingLabel := mSettings.AddSubMenuItem("Tracking:", "")
	mTrackingLabel.Disable()

	mouseTrackingEnabled := store.IsMouseTrackingEnabled()
	mMouseTracking = mSettings.AddSubMenuItemCheckbox("Enable Mouse Distance", "", mouseTrackingEnabled)

	mSettings.AddSubMenuItem("", "").Disable() // Separator

	// Inertia section
	mInertiaLabel := mSettings.AddSubMenuItem("⚡ Inertia (Key Acceleration):", "")
	mInertiaLabel.Disable()

	inertiaSettings := store.GetInertiaSettings()
	mInertiaEnabled = mSettings.AddSubMenuItemCheckbox("Enable Inertia", "", inertiaSettings.Enabled)

	// Max Speed submenu
	mMaxSpeed := mSettings.AddSubMenuItem("   Max Speed", "")
	mInertiaUltraFast = mMaxSpeed.AddSubMenuItemCheckbox("Ultra Fast (~140 keys/sec)", "", inertiaSettings.MaxSpeed == storage.InertiaSpeedUltraFast)
	mInertiaVeryFast = mMaxSpeed.AddSubMenuItemCheckbox("Very Fast (~125 keys/sec)", "", inertiaSettings.MaxSpeed == storage.InertiaSpeedVeryFast)
	mInertiaFast = mMaxSpeed.AddSubMenuItemCheckbox("Fast (~83 keys/sec)", "", inertiaSettings.MaxSpeed == storage.InertiaSpeedFast)
	mInertiaMedium = mMaxSpeed.AddSubMenuItemCheckbox("Medium (~50 keys/sec)", "", inertiaSettings.MaxSpeed == storage.InertiaSpeedMedium)
	mInertiaSlow = mMaxSpeed.AddSubMenuItemCheckbox("Slow (~20 keys/sec)", "", inertiaSettings.MaxSpeed == storage.InertiaSpeedSlow)

	// Threshold submenu
	mThreshold := mSettings.AddSubMenuItem("   Threshold (ms)", "")
	mThreshold100 = mThreshold.AddSubMenuItemCheckbox("100ms (instant)", "", inertiaSettings.Threshold == 100)
	mThreshold150 = mThreshold.AddSubMenuItemCheckbox("150ms (fast)", "", inertiaSettings.Threshold == 150)
	mThreshold200 = mThreshold.AddSubMenuItemCheckbox("200ms (default)", "", inertiaSettings.Threshold == 200)
	mThreshold250 = mThreshold.AddSubMenuItemCheckbox("250ms (slow)", "", inertiaSettings.Threshold == 250)
	mThreshold350 = mThreshold.AddSubMenuItemCheckbox("350ms (very slow)", "", inertiaSettings.Threshold == 350)

	// Acceleration Rate submenu
	mAccelRate := mSettings.AddSubMenuItem("   Acceleration Rate", "")
	mAccelRate025 = mAccelRate.AddSubMenuItemCheckbox("0.25x (very gentle)", "", inertiaSettings.AccelRate == 0.25)
	mAccelRate050 = mAccelRate.AddSubMenuItemCheckbox("0.5x (gentle)", "", inertiaSettings.AccelRate == 0.5)
	mAccelRate100 = mAccelRate.AddSubMenuItemCheckbox("1.0x (default)", "", inertiaSettings.AccelRate == 1.0)
	mAccelRate150 = mAccelRate.AddSubMenuItemCheckbox("1.5x (faster)", "", inertiaSettings.AccelRate == 1.5)
	mAccelRate200 = mAccelRate.AddSubMenuItemCheckbox("2.0x (aggressive)", "", inertiaSettings.AccelRate == 2.0)

	mSettings.AddSubMenuItem("", "").Disable() // Separator

	// Debug section
	mDebugLabel := mSettings.AddSubMenuItem("🔧 Debug:", "")
	mDebugLabel.Disable()
	mDisplayInfo := mSettings.AddSubMenuItem("   Show Display Info", "")
	go func() {
		for range mDisplayInfo.ClickedCh {
			showDisplayDebugInfo()
		}
	}()

	// About
	mAbout := systray.AddMenuItem("About", "About Typing Telemetry")
	go func() {
		for range mAbout.ClickedCh {
			showAbout()
		}
	}()

	// Quit
	mQuit := systray.AddMenuItem("Quit", "Quit application")
	go func() {
		for range mQuit.ClickedCh {
			quit()
		}
	}()

	// Start click handlers for settings
	go handleSettingsClicks()
}

func handleSettingsClicks() {
	for {
		select {
		case <-mShowKeystrokes.ClickedCh:
			s := store.GetMenubarSettings()
			s.ShowKeystrokes = !s.ShowKeystrokes
			store.SaveMenubarSettings(s)
			if s.ShowKeystrokes {
				mShowKeystrokes.Check()
			} else {
				mShowKeystrokes.Uncheck()
			}
			updateMenuBarTitle()

		case <-mShowWords.ClickedCh:
			s := store.GetMenubarSettings()
			s.ShowWords = !s.ShowWords
			store.SaveMenubarSettings(s)
			if s.ShowWords {
				mShowWords.Check()
			} else {
				mShowWords.Uncheck()
			}
			updateMenuBarTitle()

		case <-mShowClicks.ClickedCh:
			s := store.GetMenubarSettings()
			s.ShowClicks = !s.ShowClicks
			store.SaveMenubarSettings(s)
			if s.ShowClicks {
				mShowClicks.Check()
			} else {
				mShowClicks.Uncheck()
			}
			updateMenuBarTitle()

		case <-mShowDistance.ClickedCh:
			s := store.GetMenubarSettings()
			s.ShowDistance = !s.ShowDistance
			store.SaveMenubarSettings(s)
			if s.ShowDistance {
				mShowDistance.Check()
			} else {
				mShowDistance.Uncheck()
			}
			updateMenuBarTitle()

		case <-mDistanceFeet.ClickedCh:
			store.SetDistanceUnit(storage.DistanceUnitFeet)
			mDistanceFeet.Check()
			mDistanceCars.Uncheck()
			mDistanceFields.Uncheck()
			updateMenuBarTitle()

		case <-mDistanceCars.ClickedCh:
			store.SetDistanceUnit(storage.DistanceUnitCars)
			mDistanceFeet.Uncheck()
			mDistanceCars.Check()
			mDistanceFields.Uncheck()
			updateMenuBarTitle()

		case <-mDistanceFields.ClickedCh:
			store.SetDistanceUnit(storage.DistanceUnitFrisbee)
			mDistanceFeet.Uncheck()
			mDistanceCars.Uncheck()
			mDistanceFields.Check()
			updateMenuBarTitle()

		case <-mMouseTracking.ClickedCh:
			enabled := store.IsMouseTrackingEnabled()
			store.SetMouseTrackingEnabled(!enabled)
			if !enabled {
				mMouseTracking.Check()
			} else {
				mMouseTracking.Uncheck()
			}
			showAlertDialog("Mouse Distance "+map[bool]string{true: "Disabled", false: "Enabled"}[enabled],
				"Restart the app for changes to take effect.",
				[]string{"OK"})

		case <-mInertiaEnabled.ClickedCh:
			s := store.GetInertiaSettings()
			newEnabled := !s.Enabled
			store.SetInertiaEnabled(newEnabled)
			if newEnabled {
				mInertiaEnabled.Check()
				cfg := inertia.Config{
					Enabled:   true,
					MaxSpeed:  s.MaxSpeed,
					Threshold: s.Threshold,
					AccelRate: s.AccelRate,
				}
				inertia.Start(cfg)
			} else {
				mInertiaEnabled.Uncheck()
				inertia.Stop()
			}
			showAlertDialog("Inertia "+map[bool]string{true: "Enabled", false: "Disabled"}[newEnabled],
				"Key acceleration is now "+map[bool]string{true: "active", false: "inactive"}[newEnabled]+".\n\nHold any key to accelerate repeat speed.\nPressing any other key resets acceleration.",
				[]string{"OK"})

		case <-mInertiaUltraFast.ClickedCh:
			store.SetInertiaMaxSpeed(storage.InertiaSpeedUltraFast)
			updateInertiaSpeedChecks(storage.InertiaSpeedUltraFast)
			updateInertiaConfig()

		case <-mInertiaVeryFast.ClickedCh:
			store.SetInertiaMaxSpeed(storage.InertiaSpeedVeryFast)
			updateInertiaSpeedChecks(storage.InertiaSpeedVeryFast)
			updateInertiaConfig()

		case <-mInertiaFast.ClickedCh:
			store.SetInertiaMaxSpeed(storage.InertiaSpeedFast)
			updateInertiaSpeedChecks(storage.InertiaSpeedFast)
			updateInertiaConfig()

		case <-mInertiaMedium.ClickedCh:
			store.SetInertiaMaxSpeed(storage.InertiaSpeedMedium)
			updateInertiaSpeedChecks(storage.InertiaSpeedMedium)
			updateInertiaConfig()

		case <-mInertiaSlow.ClickedCh:
			store.SetInertiaMaxSpeed(storage.InertiaSpeedSlow)
			updateInertiaSpeedChecks(storage.InertiaSpeedSlow)
			updateInertiaConfig()

		case <-mThreshold100.ClickedCh:
			store.SetInertiaThreshold(100)
			updateThresholdChecks(100)
			updateInertiaConfig()

		case <-mThreshold150.ClickedCh:
			store.SetInertiaThreshold(150)
			updateThresholdChecks(150)
			updateInertiaConfig()

		case <-mThreshold200.ClickedCh:
			store.SetInertiaThreshold(200)
			updateThresholdChecks(200)
			updateInertiaConfig()

		case <-mThreshold250.ClickedCh:
			store.SetInertiaThreshold(250)
			updateThresholdChecks(250)
			updateInertiaConfig()

		case <-mThreshold350.ClickedCh:
			store.SetInertiaThreshold(350)
			updateThresholdChecks(350)
			updateInertiaConfig()

		case <-mAccelRate025.ClickedCh:
			store.SetInertiaAccelRate(0.25)
			updateAccelRateChecks(0.25)
			updateInertiaConfig()

		case <-mAccelRate050.ClickedCh:
			store.SetInertiaAccelRate(0.5)
			updateAccelRateChecks(0.5)
			updateInertiaConfig()

		case <-mAccelRate100.ClickedCh:
			store.SetInertiaAccelRate(1.0)
			updateAccelRateChecks(1.0)
			updateInertiaConfig()

		case <-mAccelRate150.ClickedCh:
			store.SetInertiaAccelRate(1.5)
			updateAccelRateChecks(1.5)
			updateInertiaConfig()

		case <-mAccelRate200.ClickedCh:
			store.SetInertiaAccelRate(2.0)
			updateAccelRateChecks(2.0)
			updateInertiaConfig()
		}
	}
}

func updateInertiaSpeedChecks(speed string) {
	mInertiaUltraFast.Uncheck()
	mInertiaVeryFast.Uncheck()
	mInertiaFast.Uncheck()
	mInertiaMedium.Uncheck()
	mInertiaSlow.Uncheck()
	switch speed {
	case storage.InertiaSpeedUltraFast:
		mInertiaUltraFast.Check()
	case storage.InertiaSpeedVeryFast:
		mInertiaVeryFast.Check()
	case storage.InertiaSpeedFast:
		mInertiaFast.Check()
	case storage.InertiaSpeedMedium:
		mInertiaMedium.Check()
	case storage.InertiaSpeedSlow:
		mInertiaSlow.Check()
	}
}

func updateThresholdChecks(threshold int) {
	mThreshold100.Uncheck()
	mThreshold150.Uncheck()
	mThreshold200.Uncheck()
	mThreshold250.Uncheck()
	mThreshold350.Uncheck()
	switch threshold {
	case 100:
		mThreshold100.Check()
	case 150:
		mThreshold150.Check()
	case 200:
		mThreshold200.Check()
	case 250:
		mThreshold250.Check()
	case 350:
		mThreshold350.Check()
	}
}

func updateAccelRateChecks(rate float64) {
	mAccelRate025.Uncheck()
	mAccelRate050.Uncheck()
	mAccelRate100.Uncheck()
	mAccelRate150.Uncheck()
	mAccelRate200.Uncheck()
	switch rate {
	case 0.25:
		mAccelRate025.Check()
	case 0.5:
		mAccelRate050.Check()
	case 1.0:
		mAccelRate100.Check()
	case 1.5:
		mAccelRate150.Check()
	case 2.0:
		mAccelRate200.Check()
	}
}

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

func updateMenuBarTitle() {
	stats, err := store.GetTodayStats()
	if err != nil {
		setMenuTitle("⌨️ --")
		return
	}

	settings := store.GetMenubarSettings()
	mouseStats, _ := store.GetTodayMouseStats()

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

func setMenuTitle(title string) {
	menuTitleMutex.Lock()
	defer menuTitleMutex.Unlock()

	if title == lastMenuTitle {
		return
	}

	lastMenuTitle = title
	systray.SetTitle(title)
}

func updateStatsDisplay() {
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

	// Update menu items
	mTodayKeystrokes.SetTitle(fmt.Sprintf("Today: %s keystrokes (%s words)", formatAbsolute(keystrokeCount), formatAbsolute(todayWords)))
	mTodayMouse.SetTitle(fmt.Sprintf("Today: 🖱️ %s clicks, %s distance", formatAbsolute(todayClicks), formatDistance(todayMouseDistance)))
	mWeekKeystrokes.SetTitle(fmt.Sprintf("This Week: %s keystrokes (%s words)", formatAbsolute(weekKeystrokes), formatAbsolute(weekWords)))
	mWeekMouse.SetTitle(fmt.Sprintf("This Week: 🖱️ %s clicks, %s distance", formatAbsolute(weekClicks), formatDistance(weekMouseDistance)))

	// Update leaderboard
	updateLeaderboard()
}

func updateLeaderboard() {
	entries, err := store.GetMouseLeaderboard(10)
	if err != nil || len(entries) == 0 {
		for _, item := range leaderboardItems {
			item.Hide()
		}
		return
	}

	for i, item := range leaderboardItems {
		if i < len(entries) {
			entry := entries[i]
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
			item.SetTitle(fmt.Sprintf("%s#%d: %s - %s", medal, entry.Rank, t.Format("Jan 2, 2006"), formatDistance(entry.TotalDistance)))
			item.Show()
		} else {
			item.Hide()
		}
	}
}

func showAbout() {
	response := showAlertDialog("Typing Telemetry",
		fmt.Sprintf("Version %s\n\nTrack your keystrokes and typing speed.\n\nGitHub: github.com/abaj8494/typing-telemetry", Version),
		[]string{"OK", "Open GitHub"})

	if response == 1 {
		if !openURLNative("https://github.com/abaj8494/typing-telemetry") {
			log.Printf("Failed to open GitHub URL")
		}
	}
}

func quit() {
	response := showAlertDialog("Quit Typing Telemetry",
		"This will stop keystroke tracking and close the menu bar app.\n\nTo restart, run: open -a typtel-menubar",
		[]string{"Cancel", "Quit"})

	if response == 1 {
		log.Println("User requested quit")
		keylogger.Stop()
		mousetracker.Stop()
		inertia.Stop()
		if store != nil {
			store.Close()
		}
		systray.Quit()
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

func showDisplayDebugInfo() {
	ppi := mousetracker.GetAveragePPI()
	displayCount := mousetracker.GetDisplayCount()

	info := fmt.Sprintf("Displays Detected: %d\nAverage PPI: %.1f\n\n", displayCount, ppi)

	if ppi == mousetracker.DefaultPPI {
		info += "Note: Using fallback PPI (100).\nYour displays may not report physical dimensions."
	} else {
		info += fmt.Sprintf("Mouse distance is calculated using %.1f pixels per inch.", ppi)
	}

	showAlertDialog("Display Information", info, []string{"OK"})
}

// Native alert dialog using Cocoa
func showAlertDialog(messageText, informativeText string, buttons []string) int {
	cMessage := C.CString(messageText)
	cInfo := C.CString(informativeText)
	defer C.free(unsafe.Pointer(cMessage))
	defer C.free(unsafe.Pointer(cInfo))

	cButtons := make([]*C.char, len(buttons))
	for i, b := range buttons {
		cButtons[i] = C.CString(b)
		defer C.free(unsafe.Pointer(cButtons[i]))
	}

	return int(C.showAlert(cMessage, cInfo, &cButtons[0], C.int(len(buttons))))
}

// Open URL in default browser using native macOS API
func openURLNative(url string) bool {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))
	return C.openURL(cURL) == 1
}

// Open file using native macOS API
func openFileNative(path string) bool {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	return C.openFile(cPath) == 1
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

func formatAbsolute(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return s
	}

	result := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

func formatDistance(pixels float64) string {
	feet := mousetracker.PixelsToFeet(pixels)

	unit := store.GetDistanceUnit()
	switch unit {
	case storage.DistanceUnitCars:
		cars := feet / 15.0
		if cars >= 1000 {
			return fmt.Sprintf("%.1fk cars", cars/1000)
		} else if cars >= 1 {
			return fmt.Sprintf("%.0f cars", cars)
		}
		return fmt.Sprintf("%.1f cars", cars)

	case storage.DistanceUnitFrisbee:
		fields := feet / 330.0
		if fields >= 100 {
			return fmt.Sprintf("%.0f fields", fields)
		} else if fields >= 1 {
			return fmt.Sprintf("%.1f fields", fields)
		}
		return fmt.Sprintf("%.2f fields", fields)

	default:
		if feet >= 5280 {
			return fmt.Sprintf("%.1fmi", feet/5280)
		} else if feet >= 1 {
			return fmt.Sprintf("%.0fft", feet)
		}
		inches := feet * 12
		return fmt.Sprintf("%.0fin", inches)
	}
}

func showLeaderboard() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Panic in showLeaderboard: %v", r)
			}
		}()

		htmlPath, err := generateLeaderboardHTML()
		if err != nil {
			log.Printf("Failed to generate leaderboard: %v", err)
			showAlertDialog("Error", fmt.Sprintf("Failed to generate leaderboard: %v", err), []string{"OK"})
			return
		}

		if !openFileNative(htmlPath) {
			log.Printf("Failed to open leaderboard file: %s", htmlPath)
			showAlertDialog("Error", "Failed to open leaderboard in browser. The file was saved to:\n"+htmlPath, []string{"OK"})
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
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Panic in openCharts: %v", r)
			}
		}()

		htmlPath, err := generateChartsHTML()
		if err != nil {
			log.Printf("Failed to generate charts: %v", err)
			showAlertDialog("Error", fmt.Sprintf("Failed to generate charts: %v", err), []string{"OK"})
			return
		}

		if !openFileNative(htmlPath) {
			log.Printf("Failed to open charts file: %s", htmlPath)
			showAlertDialog("Error", "Failed to open charts in browser. The file was saved to:\n"+htmlPath, []string{"OK"})
		}
	}()
}

func generateChartsHTML() (string, error) {
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

	weeklyLabels, weeklyKeystrokes, weeklyWords, weeklyMouseFeet, weeklyTotalKeys, weeklyTotalWords, weeklyTotalMouse, weeklyHeatmap, err := prepareChartData(7)
	if err != nil {
		return "", err
	}

	monthlyLabels, monthlyKeystrokes, monthlyWords, monthlyMouseFeet, monthlyTotalKeys, monthlyTotalWords, monthlyTotalMouse, monthlyHeatmap, err := prepareChartData(30)
	if err != nil {
		return "", err
	}

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

            document.getElementById('totalKeystrokes').textContent = formatNumber(d.totalKeystrokes);
            document.getElementById('totalWords').textContent = formatNumber(d.totalWords);
            document.getElementById('avgKeystrokes').textContent = formatNumber(Math.round(d.totalKeystrokes / d.days));
            document.getElementById('totalMouse').textContent = formatDistance(d.totalMouseFeet);

            document.getElementById('mouseChartTitle').textContent = 'Mouse Distance per Day (' + unitLabels[unit] + ')';

            if (keystrokesChart) keystrokesChart.destroy();
            if (wordsChart) wordsChart.destroy();
            if (mouseChart) mouseChart.destroy();

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

            document.getElementById('heatmapContainer').innerHTML = d.heatmap;
        }

        updateCharts();
    </script>
</body>
</html>`,
		generateHourLabels(),
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
	var maxVal int64 = 1
	for _, hours := range hourlyData {
		for _, h := range hours {
			if h.Keystrokes > maxVal {
				maxVal = h.Keystrokes
			}
		}
	}

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

// +build darwin

package inertia

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework ApplicationServices

#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#include <stdbool.h>

// Callback declarations
extern CGEventRef goInertiaEventCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon);

// Event tap for inertia - uses kCGEventTapOptionDefault to allow modification
static CFMachPortRef inertiaEventTap = NULL;
static CFRunLoopSourceRef inertiaRunLoopSource = NULL;

static CGEventRef inertiaEventCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    return goInertiaEventCallback(proxy, type, event, refcon);
}

static int createInertiaEventTap() {
    if (inertiaEventTap != NULL) {
        return 1; // Already created
    }

    // Listen for keyDown and keyUp events
    CGEventMask eventMask = CGEventMaskBit(kCGEventKeyDown) |
                            CGEventMaskBit(kCGEventKeyUp);

    // Use kCGEventTapOptionDefault to allow event modification/suppression
    inertiaEventTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionDefault,
        eventMask,
        inertiaEventCallback,
        NULL
    );

    if (inertiaEventTap == NULL) {
        return 0;
    }

    return 1;
}

static void runInertiaEventLoop() {
    if (inertiaEventTap == NULL) {
        return;
    }

    inertiaRunLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, inertiaEventTap, 0);
    CFRunLoopAddSource(CFRunLoopGetCurrent(), inertiaRunLoopSource, kCFRunLoopCommonModes);
    CGEventTapEnable(inertiaEventTap, true);
    CFRunLoopRun();
}

static void stopInertiaEventLoop() {
    if (inertiaEventTap != NULL) {
        CGEventTapEnable(inertiaEventTap, false);
        if (inertiaRunLoopSource != NULL) {
            CFRunLoopRemoveSource(CFRunLoopGetCurrent(), inertiaRunLoopSource, kCFRunLoopCommonModes);
            CFRelease(inertiaRunLoopSource);
            inertiaRunLoopSource = NULL;
        }
        CFRelease(inertiaEventTap);
        inertiaEventTap = NULL;
    }
    CFRunLoopStop(CFRunLoopGetCurrent());
}

// Synthesize a key event
static void postKeyEvent(CGKeyCode keycode, bool keyDown) {
    CGEventRef event = CGEventCreateKeyboardEvent(NULL, keycode, keyDown);
    if (event != NULL) {
        CGEventPost(kCGHIDEventTap, event);
        CFRelease(event);
    }
}

// Check if event is an autorepeat
static bool isAutorepeatEvent(CGEventRef event) {
    return CGEventGetIntegerValueField(event, kCGKeyboardEventAutorepeat) != 0;
}

// Get keycode from event
static int getKeycode(CGEventRef event) {
    return (int)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
}

// Check accessibility
static int checkInertiaAccessibility() {
    return AXIsProcessTrusted();
}

// Return null event ref (to suppress events)
static CGEventRef nullEventRef() {
    return NULL;
}
*/
import "C"

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"
)

// Debug logging
var (
	debugMode   = true // Set to true for debugging
	debugLogger *log.Logger
)

func init() {
	if debugMode {
		// Log to file in ~/.local/share/typtel/logs/
		homeDir, _ := os.UserHomeDir()
		logDir := filepath.Join(homeDir, ".local", "share", "typtel", "logs")
		os.MkdirAll(logDir, 0755)
		logFile, err := os.OpenFile(
			filepath.Join(logDir, "inertia-debug.log"),
			os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
			0644,
		)
		if err == nil {
			debugLogger = log.New(logFile, "", log.Ltime|log.Lmicroseconds)
		}
	}
}

func debugLog(format string, args ...interface{}) {
	if debugMode && debugLogger != nil {
		debugLogger.Printf(format, args...)
	}
}

// Config holds inertia configuration
type Config struct {
	Enabled       bool
	MaxSpeed      string  // "very_fast", "fast", "medium"
	Threshold     int     // ms before acceleration starts (default 200)
	AccelRate     float64 // acceleration multiplier (default 1.0)
}

// DefaultConfig returns the default inertia configuration
func DefaultConfig() Config {
	return Config{
		Enabled:   false,
		MaxSpeed:  "fast",
		Threshold: 200,
		AccelRate: 1.0,
	}
}

// Acceleration table: key_count thresholds for each speed step
// Based on reference: { 7, 12, 17, 21, 24, 26, 28, 30 }
var accelerationTable = []int{7, 12, 17, 21, 24, 26, 28, 30}

// Base repeat interval in milliseconds (macOS default is ~35ms)
const baseRepeatInterval = 35

// Max speed caps (repeat interval in ms)
// Capped at what terminals/editors can reasonably handle
var maxSpeedCaps = map[string]int{
	"ultra_fast": 7,   // ~140 keys/sec (pushing terminal limits)
	"very_fast":  8,   // ~125 keys/sec
	"fast":       12,  // ~83 keys/sec
	"medium":     20,  // ~50 keys/sec
	"slow":       50,  // ~20 keys/sec
}

// State tracking
type keyState struct {
	isHeld        bool
	keyCount      int
	lastEventTime time.Time
	repeatTimer   *time.Timer
	stopChan      chan struct{}
	lastStopTime  time.Time // When key was released - to prevent race condition restarts
}

var (
	mu             sync.RWMutex
	config         Config
	running        bool
	keyStates      = make(map[int]*keyState)  // keycode -> state
)

// Global reference to prevent GC
var inertiaInstance *Inertia

// Inertia manages the key acceleration system
type Inertia struct {
	config Config
	stopCh chan struct{}
}

// New creates a new Inertia instance
func New(cfg Config) *Inertia {
	return &Inertia{
		config: cfg,
		stopCh: make(chan struct{}),
	}
}

// Start begins the inertia system
func Start(cfg Config) error {
	mu.Lock()
	defer mu.Unlock()

	if running {
		return nil
	}

	config = cfg

	if !cfg.Enabled {
		return nil
	}

	if C.checkInertiaAccessibility() == 0 {
		return nil // Silently fail if no accessibility
	}

	if C.createInertiaEventTap() == 0 {
		return nil
	}

	running = true
	inertiaInstance = New(cfg)

	go func() {
		C.runInertiaEventLoop()
	}()

	return nil
}

// Stop stops the inertia system
func Stop() {
	mu.Lock()
	defer mu.Unlock()

	if !running {
		return
	}

	// Stop all active key repeats
	for _, state := range keyStates {
		if state.stopChan != nil {
			close(state.stopChan)
		}
		if state.repeatTimer != nil {
			state.repeatTimer.Stop()
		}
	}
	keyStates = make(map[int]*keyState)

	C.stopInertiaEventLoop()
	running = false
	inertiaInstance = nil
}

// UpdateConfig updates the inertia configuration
func UpdateConfig(cfg Config) {
	mu.Lock()
	defer mu.Unlock()
	config = cfg

	// If disabled, stop any active repeats
	if !cfg.Enabled {
		for _, state := range keyStates {
			if state.stopChan != nil {
				close(state.stopChan)
			}
			if state.repeatTimer != nil {
				state.repeatTimer.Stop()
			}
		}
		keyStates = make(map[int]*keyState)
	}
}

// IsRunning returns whether inertia is active
func IsRunning() bool {
	mu.RLock()
	defer mu.RUnlock()
	return running && config.Enabled
}

// getAccelerationStep calculates the current speed step based on key_count
func getAccelerationStep(keyCount int) int {
	for idx, threshold := range accelerationTable {
		if threshold > keyCount {
			return idx + 1
		}
	}
	return len(accelerationTable) + 1
}

// getRepeatInterval calculates the repeat interval based on acceleration
func getRepeatInterval(keyCount int, cfg Config) time.Duration {
	step := getAccelerationStep(keyCount)

	// Base interval decreases with each step
	// Apply acceleration rate multiplier
	interval := float64(baseRepeatInterval) / (float64(step) * cfg.AccelRate)

	// Apply max speed cap
	minInterval := maxSpeedCaps[cfg.MaxSpeed]
	if minInterval == 0 {
		minInterval = maxSpeedCaps["fast"]
	}

	if interval < float64(minInterval) {
		interval = float64(minInterval)
	}

	return time.Duration(interval) * time.Millisecond
}

// startKeyRepeat starts the accelerating key repeat for a held key
func startKeyRepeat(keycode int) {
	mu.Lock()
	state, exists := keyStates[keycode]
	if !exists {
		state = &keyState{
			stopChan: make(chan struct{}),
		}
		keyStates[keycode] = state
	}

	// Reset if there was an old repeat
	if state.stopChan != nil {
		select {
		case <-state.stopChan:
			// Already closed
		default:
			close(state.stopChan)
		}
	}

	state.isHeld = true
	state.keyCount = 0
	state.lastEventTime = time.Now()
	state.stopChan = make(chan struct{})
	cfg := config
	mu.Unlock()

	// Start the repeat goroutine
	go func(kc int, stopCh chan struct{}, initialDelay time.Duration) {
		debugLog("REPEAT_START keycode=%d initialDelay=%v maxSpeed=%s accelRate=%.1f",
			kc, initialDelay, cfg.MaxSpeed, cfg.AccelRate)

		// Wait for the initial delay (threshold) before starting repeats
		select {
		case <-stopCh:
			debugLog("REPEAT_CANCELLED_EARLY keycode=%d", kc)
			return
		case <-time.After(initialDelay):
			debugLog("REPEAT_DELAY_DONE keycode=%d, starting acceleration", kc)
		}

		for {
			mu.RLock()
			s, ok := keyStates[kc]
			if !ok || !s.isHeld || !config.Enabled {
				mu.RUnlock()
				debugLog("REPEAT_STOPPED keycode=%d ok=%v isHeld=%v enabled=%v",
					kc, ok, ok && s.isHeld, config.Enabled)
				return
			}
			s.keyCount++
			keyCount := s.keyCount
			interval := getRepeatInterval(keyCount, cfg)
			mu.RUnlock()

			debugLog("REPEAT_POST keycode=%d count=%d interval=%v", kc, keyCount, interval)

			// Post only keyDown - NOT keyUp (keyUp would trigger stopKeyRepeat)
			C.postKeyEvent(C.CGKeyCode(kc), C.bool(true))

			select {
			case <-stopCh:
				debugLog("REPEAT_STOPPED_SIGNAL keycode=%d", kc)
				return
			case <-time.After(interval):
				// Continue to next repeat
			}
		}
	}(keycode, state.stopChan, time.Duration(cfg.Threshold)*time.Millisecond)
}

// stopKeyRepeat stops the key repeat for a released key
func stopKeyRepeat(keycode int) {
	mu.Lock()
	defer mu.Unlock()

	state, exists := keyStates[keycode]
	if !exists {
		return
	}

	state.isHeld = false
	state.lastStopTime = time.Now() // Record when we stopped to prevent race condition restarts
	if state.stopChan != nil {
		select {
		case <-state.stopChan:
			// Already closed
		default:
			close(state.stopChan)
		}
	}
	debugLog("STOP_KEY keycode=%d lastStopTime=%v", keycode, state.lastStopTime)
}


// stopOtherKeys stops inertia for all keys except the specified one
// This is called when a new key is pressed to prevent inertia buildup
// from continuing when the user switches to typing other keys
func stopOtherKeys(exceptKeycode int) {
	mu.Lock()
	defer mu.Unlock()

	for kc, state := range keyStates {
		if kc != exceptKeycode && state.isHeld {
			state.isHeld = false
			state.lastStopTime = time.Now()
			if state.stopChan != nil {
				select {
				case <-state.stopChan:
					// Already closed
				default:
					close(state.stopChan)
				}
			}
			debugLog("STOP_OTHER_KEY keycode=%d (new key=%d pressed)", kc, exceptKeycode)
		}
	}
}


//export goInertiaEventCallback
func goInertiaEventCallback(proxy C.CGEventTapProxy, eventType C.CGEventType, event C.CGEventRef, refcon unsafe.Pointer) C.CGEventRef {
	mu.RLock()
	enabled := config.Enabled
	mu.RUnlock()

	if !enabled {
		return event
	}

	keycode := int(C.getKeycode(event))

	switch eventType {
	case C.kCGEventKeyDown:
		isAutorepeat := C.isAutorepeatEvent(event) != false

		// Check if we're already handling this key
		mu.RLock()
		state, exists := keyStates[keycode]
		alreadyHeld := exists && state.isHeld
		// Check if key was recently released (within 50ms) - likely a synthetic event in-flight
		recentlyReleased := exists && !state.isHeld && time.Since(state.lastStopTime) < 50*time.Millisecond
		mu.RUnlock()

		debugLog("EVENT_KEYDOWN keycode=%d autorepeat=%v alreadyHeld=%v recentlyReleased=%v", keycode, isAutorepeat, alreadyHeld, recentlyReleased)

		if isAutorepeat {
			// Suppress macOS autorepeat - we generate our own
			if alreadyHeld {
				debugLog("SUPPRESS_AUTOREPEAT keycode=%d", keycode)
				return C.nullEventRef()
			}
			debugLog("PASSTHROUGH keycode=%d (autorepeat but not tracked)", keycode)
		} else if alreadyHeld {
			// This is our synthetic event - let it through but don't restart
			debugLog("PASSTHROUGH_SYNTHETIC keycode=%d", keycode)
		} else if recentlyReleased {
			// Synthetic event that arrived after keyUp - ignore to prevent restart
			debugLog("IGNORE_LATE_SYNTHETIC keycode=%d (released %v ago)", keycode, time.Since(state.lastStopTime))
			return C.nullEventRef()
		} else {
			// Initial key press - stop inertia on all other held keys first
			// This prevents the "swarm of characters" issue when switching keys
			stopOtherKeys(keycode)
			// Then start our accelerating repeat for this key
			debugLog("START_TRACKING keycode=%d", keycode)
			startKeyRepeat(keycode)
		}

	case C.kCGEventKeyUp:
		debugLog("EVENT_KEYUP keycode=%d", keycode)
		stopKeyRepeat(keycode)
	}

	return event
}

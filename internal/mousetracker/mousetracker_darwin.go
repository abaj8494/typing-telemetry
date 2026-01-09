//go:build darwin
// +build darwin

package mousetracker

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework ApplicationServices

#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>

extern void goMouseCallback(double x, double y);
extern void goMouseClickCallback();

static CGEventRef mouseEventCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    // Handle mouse moved and drag events
    if (type == kCGEventMouseMoved ||
        type == kCGEventLeftMouseDragged ||
        type == kCGEventRightMouseDragged ||
        type == kCGEventOtherMouseDragged) {
        CGPoint location = CGEventGetLocation(event);
        goMouseCallback(location.x, location.y);
    }
    // Handle mouse clicks (down events only to avoid double-counting)
    if (type == kCGEventLeftMouseDown ||
        type == kCGEventRightMouseDown ||
        type == kCGEventOtherMouseDown) {
        goMouseClickCallback();
    }
    return event;
}

static CFMachPortRef createMouseEventTap() {
    // Listen for mouse movement, drag events, and clicks
    CGEventMask eventMask = CGEventMaskBit(kCGEventMouseMoved) |
                            CGEventMaskBit(kCGEventLeftMouseDragged) |
                            CGEventMaskBit(kCGEventRightMouseDragged) |
                            CGEventMaskBit(kCGEventOtherMouseDragged) |
                            CGEventMaskBit(kCGEventLeftMouseDown) |
                            CGEventMaskBit(kCGEventRightMouseDown) |
                            CGEventMaskBit(kCGEventOtherMouseDown);

    CFMachPortRef eventTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        eventMask,
        mouseEventCallback,
        NULL
    );
    return eventTap;
}

static int isMouseEventTapValid(CFMachPortRef eventTap) {
    return eventTap != NULL;
}

static int checkMouseAccessibilityPermissions() {
    return AXIsProcessTrusted();
}

static void runMouseEventLoop(CFMachPortRef eventTap) {
    CFRunLoopSourceRef runLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, eventTap, 0);
    CFRunLoopAddSource(CFRunLoopGetCurrent(), runLoopSource, kCFRunLoopCommonModes);
    CGEventTapEnable(eventTap, true);
    CFRunLoopRun();
}

// Get initial mouse position
static void getMousePosition(double *x, double *y) {
    CGEventRef event = CGEventCreate(NULL);
    CGPoint cursor = CGEventGetLocation(event);
    *x = cursor.x;
    *y = cursor.y;
    CFRelease(event);
}

// Get average PPI across all displays
// Returns the average PPI, or 0 if unable to determine
static double getAverageDisplayPPI() {
    uint32_t displayCount;
    CGDirectDisplayID displays[16]; // Support up to 16 displays

    // Get all active displays
    if (CGGetActiveDisplayList(16, displays, &displayCount) != kCGErrorSuccess) {
        return 0;
    }

    if (displayCount == 0) {
        return 0;
    }

    double totalPPI = 0;
    int validDisplays = 0;
    double firstValidPPI = 0;

    for (uint32_t i = 0; i < displayCount; i++) {
        CGDirectDisplayID display = displays[i];

        // Get pixel dimensions
        size_t pixelWidth = CGDisplayPixelsWide(display);
        size_t pixelHeight = CGDisplayPixelsHigh(display);

        // Get physical size in millimeters
        CGSize physicalSize = CGDisplayScreenSize(display);

        // Skip if physical size is invalid (0 or negative)
        if (physicalSize.width <= 0 || physicalSize.height <= 0) {
            continue;
        }

        // Calculate PPI (using diagonal)
        double pixelDiagonal = sqrt((double)(pixelWidth * pixelWidth + pixelHeight * pixelHeight));
        double mmDiagonal = sqrt(physicalSize.width * physicalSize.width + physicalSize.height * physicalSize.height);
        double inchDiagonal = mmDiagonal / 25.4;

        if (inchDiagonal > 0) {
            double ppi = pixelDiagonal / inchDiagonal;
            totalPPI += ppi;
            validDisplays++;

            // Store first valid PPI for fallback
            if (firstValidPPI == 0) {
                firstValidPPI = ppi;
            }
        }
    }

    // If we got valid PPIs, return average
    if (validDisplays > 0) {
        return totalPPI / validDisplays;
    }

    // Fallback: if we have at least one valid PPI, multiply by display count
    if (firstValidPPI > 0) {
        return firstValidPPI; // Just use the one valid PPI we found
    }

    // No valid PPI found
    return 0;
}

// Get the number of active displays
static int getDisplayCount() {
    uint32_t displayCount;
    if (CGGetActiveDisplayList(0, NULL, &displayCount) != kCGErrorSuccess) {
        return 0;
    }
    return (int)displayCount;
}
*/
import "C"
import (
	"errors"
	"math"
	"sync"
)

// MousePosition represents a mouse position with coordinates
type MousePosition struct {
	X float64
	Y float64
}

// MouseMovement represents a mouse movement event with distance traveled
type MouseMovement struct {
	X        float64
	Y        float64
	Distance float64 // Euclidean distance from last position
}

// MouseClick represents a mouse click event
type MouseClick struct{}

var (
	mouseChan    chan MouseMovement
	clickChan    chan MouseClick
	mu           sync.Mutex
	running      bool
	lastX, lastY float64
	initialized  bool
)

//export goMouseCallback
func goMouseCallback(x, y C.double) {
	mu.Lock()
	defer mu.Unlock()

	newX := float64(x)
	newY := float64(y)

	if mouseChan != nil {
		var distance float64
		if initialized {
			// Calculate Euclidean distance
			dx := newX - lastX
			dy := newY - lastY
			distance = math.Sqrt(dx*dx + dy*dy)
		} else {
			distance = 0
			initialized = true
		}

		// Update last position
		lastX = newX
		lastY = newY

		// Only send if there was actual movement
		if distance > 0 {
			select {
			case mouseChan <- MouseMovement{X: newX, Y: newY, Distance: distance}:
			default:
				// Channel full, drop event
			}
		}
	}
}

//export goMouseClickCallback
func goMouseClickCallback() {
	mu.Lock()
	defer mu.Unlock()

	if clickChan != nil {
		select {
		case clickChan <- MouseClick{}:
		default:
			// Channel full, drop event
		}
	}
}

// CheckAccessibilityPermissions returns true if the app has accessibility permissions
func CheckAccessibilityPermissions() bool {
	return C.checkMouseAccessibilityPermissions() != 0
}

// GetCurrentPosition returns the current mouse cursor position
func GetCurrentPosition() MousePosition {
	var x, y C.double
	C.getMousePosition(&x, &y)
	return MousePosition{X: float64(x), Y: float64(y)}
}

// Start begins capturing mouse movements and clicks, returns channels for both
func Start() (<-chan MouseMovement, <-chan MouseClick, error) {
	mu.Lock()
	defer mu.Unlock()

	if running {
		return nil, nil, errors.New("mouse tracker already running")
	}

	if !CheckAccessibilityPermissions() {
		return nil, nil, errors.New("accessibility permissions not granted - please enable in System Preferences > Privacy & Security > Accessibility")
	}

	mouseChan = make(chan MouseMovement, 1000)
	clickChan = make(chan MouseClick, 1000)

	// Get initial position
	pos := GetCurrentPosition()
	lastX = pos.X
	lastY = pos.Y
	initialized = true

	go func() {
		eventTap := C.createMouseEventTap()
		if C.isMouseEventTapValid(eventTap) == 0 {
			return
		}
		mu.Lock()
		running = true
		mu.Unlock()
		C.runMouseEventLoop(eventTap)
	}()

	return mouseChan, clickChan, nil
}

// Stop stops the mouse tracker
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if mouseChan != nil {
		close(mouseChan)
		mouseChan = nil
	}
	if clickChan != nil {
		close(clickChan)
		clickChan = nil
	}
	running = false
	initialized = false
}

// GetMidnightPosition returns the stored midnight position (to be used with storage)
// This is a helper function - actual storage is handled by the storage package
func ResetForNewDay() {
	mu.Lock()
	defer mu.Unlock()
	// Reset the initialized flag so next movement calculates from current position
	// This allows proper tracking from midnight
	initialized = false
}

// DefaultPPI is the fallback PPI when display info cannot be queried
const DefaultPPI = 100.0

// GetAveragePPI returns the average PPI across all connected displays.
// It uses a cascading fallback approach:
// 1. Query all displays and average their PPIs
// 2. If some displays report 0, use the average of valid ones
// 3. If all fail, fall back to DefaultPPI (100)
func GetAveragePPI() float64 {
	ppi := float64(C.getAverageDisplayPPI())

	if ppi > 0 {
		return ppi
	}

	// Fallback to default
	return DefaultPPI
}

// GetDisplayCount returns the number of active displays
func GetDisplayCount() int {
	return int(C.getDisplayCount())
}

// PixelsToInches converts pixel distance to inches using the average display PPI
func PixelsToInches(pixels float64) float64 {
	ppi := GetAveragePPI()
	return pixels / ppi
}

// PixelsToFeet converts pixel distance to feet using the average display PPI
func PixelsToFeet(pixels float64) float64 {
	return PixelsToInches(pixels) / 12.0
}

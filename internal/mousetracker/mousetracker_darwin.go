// +build darwin

package mousetracker

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework ApplicationServices

#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>

extern void goMouseCallback(double x, double y);

static CGEventRef mouseEventCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    // Handle mouse moved and drag events
    if (type == kCGEventMouseMoved ||
        type == kCGEventLeftMouseDragged ||
        type == kCGEventRightMouseDragged ||
        type == kCGEventOtherMouseDragged) {
        CGPoint location = CGEventGetLocation(event);
        goMouseCallback(location.x, location.y);
    }
    return event;
}

static CFMachPortRef createMouseEventTap() {
    // Listen for mouse movement and drag events
    CGEventMask eventMask = CGEventMaskBit(kCGEventMouseMoved) |
                            CGEventMaskBit(kCGEventLeftMouseDragged) |
                            CGEventMaskBit(kCGEventRightMouseDragged) |
                            CGEventMaskBit(kCGEventOtherMouseDragged);

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

var (
	mouseChan     chan MouseMovement
	mu            sync.Mutex
	running       bool
	lastX, lastY  float64
	initialized   bool
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

// Start begins capturing mouse movements and returns a channel that receives movement events
func Start() (<-chan MouseMovement, error) {
	mu.Lock()
	defer mu.Unlock()

	if running {
		return nil, errors.New("mouse tracker already running")
	}

	if !CheckAccessibilityPermissions() {
		return nil, errors.New("accessibility permissions not granted - please enable in System Preferences > Privacy & Security > Accessibility")
	}

	mouseChan = make(chan MouseMovement, 1000)

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

	return mouseChan, nil
}

// Stop stops the mouse tracker
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if mouseChan != nil {
		close(mouseChan)
		mouseChan = nil
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

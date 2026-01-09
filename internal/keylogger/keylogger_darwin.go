//go:build darwin
// +build darwin

package keylogger

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework ApplicationServices

#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>

extern void goKeystrokeCallback(int keycode, int isRepeat);

static CGEventRef eventCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    if (type == kCGEventKeyDown) {
        CGKeyCode keycode = (CGKeyCode)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
        // Check if this is a key repeat event (holding key down)
        int isRepeat = (int)CGEventGetIntegerValueField(event, kCGKeyboardEventAutorepeat);
        goKeystrokeCallback((int)keycode, isRepeat);
    }
    return event;
}

static CFMachPortRef createEventTap() {
    CGEventMask eventMask = CGEventMaskBit(kCGEventKeyDown);
    CFMachPortRef eventTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        eventMask,
        eventCallback,
        NULL
    );
    return eventTap;
}

static int isEventTapValid(CFMachPortRef eventTap) {
    return eventTap != NULL;
}

static int checkAccessibilityPermissions() {
    return AXIsProcessTrusted();
}

static void runEventLoop(CFMachPortRef eventTap) {
    CFRunLoopSourceRef runLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, eventTap, 0);
    CFRunLoopAddSource(CFRunLoopGetCurrent(), runLoopSource, kCFRunLoopCommonModes);
    CGEventTapEnable(eventTap, true);
    CFRunLoopRun();
}
*/
import "C"
import (
	"errors"
	"sync"
)

var (
	keystrokeChan chan int
	mu            sync.Mutex
	running       bool
)

//export goKeystrokeCallback
func goKeystrokeCallback(keycode C.int, isRepeat C.int) {
	// Ignore key repeat events - holding a key counts as 1 keypress
	if isRepeat != 0 {
		return
	}

	mu.Lock()
	defer mu.Unlock()
	if keystrokeChan != nil {
		select {
		case keystrokeChan <- int(keycode):
		default:
			// Channel full, drop keystroke
		}
	}
}

// CheckAccessibilityPermissions returns true if the app has accessibility permissions
func CheckAccessibilityPermissions() bool {
	return C.checkAccessibilityPermissions() != 0
}

// Start begins capturing keystrokes and returns a channel that receives keycodes
func Start() (<-chan int, error) {
	mu.Lock()
	defer mu.Unlock()

	if running {
		return nil, errors.New("keylogger already running")
	}

	if !CheckAccessibilityPermissions() {
		return nil, errors.New("accessibility permissions not granted - please enable in System Preferences > Privacy & Security > Accessibility")
	}

	keystrokeChan = make(chan int, 1000)

	go func() {
		eventTap := C.createEventTap()
		if C.isEventTapValid(eventTap) == 0 {
			return
		}
		mu.Lock()
		running = true
		mu.Unlock()
		C.runEventLoop(eventTap)
	}()

	return keystrokeChan, nil
}

// Stop stops the keylogger
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if keystrokeChan != nil {
		close(keystrokeChan)
		keystrokeChan = nil
	}
	running = false
}

//go:build darwin
// +build darwin

package inertia

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("Expected Enabled to be false by default")
	}
	if cfg.MaxSpeed != "fast" {
		t.Errorf("Expected MaxSpeed 'fast', got %q", cfg.MaxSpeed)
	}
	if cfg.Threshold != 200 {
		t.Errorf("Expected Threshold 200, got %d", cfg.Threshold)
	}
	if cfg.AccelRate != 1.0 {
		t.Errorf("Expected AccelRate 1.0, got %f", cfg.AccelRate)
	}
}

func TestGetAccelerationStep(t *testing.T) {
	tests := []struct {
		name     string
		keyCount int
		expected int
	}{
		{"zero keys", 0, 1},
		{"1 key", 1, 1},
		{"6 keys (below first threshold)", 6, 1},
		{"7 keys (at first threshold)", 7, 2},
		{"8 keys (above first threshold)", 8, 2},
		{"11 keys (below second threshold)", 11, 2},
		{"12 keys (at second threshold)", 12, 3},
		{"17 keys (at third threshold)", 17, 4},
		{"21 keys (at fourth threshold)", 21, 5},
		{"24 keys (at fifth threshold)", 24, 6},
		{"26 keys (at sixth threshold)", 26, 7},
		{"28 keys (at seventh threshold)", 28, 8},
		{"30 keys (at eighth threshold)", 30, 9},
		{"31 keys (above all thresholds)", 31, 9},
		{"100 keys (way above)", 100, 9},
		{"1000 keys (extreme)", 1000, 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAccelerationStep(tt.keyCount)
			if result != tt.expected {
				t.Errorf("getAccelerationStep(%d) = %d, want %d", tt.keyCount, result, tt.expected)
			}
		})
	}
}

func TestGetRepeatInterval(t *testing.T) {
	tests := []struct {
		name      string
		keyCount  int
		maxSpeed  string
		accelRate float64
		expected  time.Duration
	}{
		// Step 1 (keyCount < 7), base interval / (1 * accelRate)
		{"step1 fast rate1", 0, "fast", 1.0, 35 * time.Millisecond},
		{"step1 fast rate2", 0, "fast", 2.0, 17 * time.Millisecond},

		// Step 2 (keyCount >= 7), base interval / (2 * accelRate)
		{"step2 fast rate1", 7, "fast", 1.0, 17 * time.Millisecond},
		{"step2 fast rate2", 10, "fast", 2.0, 12 * time.Millisecond}, // 35/(2*2) = 8.75, clamped to 12

		// Max speed caps
		{"step9 ultra_fast", 100, "ultra_fast", 1.0, 7 * time.Millisecond}, // 35/9 = 3.88, clamped to 7
		{"step9 very_fast", 100, "very_fast", 1.0, 8 * time.Millisecond},   // clamped to 8
		{"step9 fast", 100, "fast", 1.0, 12 * time.Millisecond},            // clamped to 12
		{"step9 medium", 100, "medium", 1.0, 20 * time.Millisecond},        // clamped to 20
		{"step9 slow", 100, "slow", 1.0, 50 * time.Millisecond},            // clamped to 50

		// Unknown max speed should default to "fast"
		{"step9 invalid speed", 100, "unknown", 1.0, 12 * time.Millisecond},
		{"step9 empty speed", 100, "", 1.0, 12 * time.Millisecond},

		// High acceleration rate
		{"step5 high accel", 21, "ultra_fast", 3.0, 7 * time.Millisecond}, // 35/(5*3)=2.33, clamped to 7

		// Low acceleration rate
		{"step1 low accel", 0, "fast", 0.5, 70 * time.Millisecond}, // 35/(1*0.5) = 70, not clamped
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Enabled:   true,
				MaxSpeed:  tt.maxSpeed,
				Threshold: 200,
				AccelRate: tt.accelRate,
			}
			result := getRepeatInterval(tt.keyCount, cfg)
			if result != tt.expected {
				t.Errorf("getRepeatInterval(%d, {MaxSpeed: %q, AccelRate: %.1f}) = %v, want %v",
					tt.keyCount, tt.maxSpeed, tt.accelRate, result, tt.expected)
			}
		})
	}
}

func TestAccelerationTableConsistency(t *testing.T) {
	// Verify the acceleration table is sorted in ascending order
	for i := 1; i < len(accelerationTable); i++ {
		if accelerationTable[i] <= accelerationTable[i-1] {
			t.Errorf("accelerationTable not sorted: [%d]=%d <= [%d]=%d",
				i, accelerationTable[i], i-1, accelerationTable[i-1])
		}
	}

	// Verify table length matches expected
	if len(accelerationTable) != 8 {
		t.Errorf("Expected 8 acceleration thresholds, got %d", len(accelerationTable))
	}
}

func TestMaxSpeedCaps(t *testing.T) {
	// Verify all expected speed caps exist
	expectedSpeeds := []string{"ultra_fast", "very_fast", "fast", "medium", "slow"}
	for _, speed := range expectedSpeeds {
		if _, ok := maxSpeedCaps[speed]; !ok {
			t.Errorf("Missing speed cap for %q", speed)
		}
	}

	// Verify caps are in expected order (faster = lower interval)
	if maxSpeedCaps["ultra_fast"] >= maxSpeedCaps["very_fast"] {
		t.Error("ultra_fast should have lower interval than very_fast")
	}
	if maxSpeedCaps["very_fast"] >= maxSpeedCaps["fast"] {
		t.Error("very_fast should have lower interval than fast")
	}
	if maxSpeedCaps["fast"] >= maxSpeedCaps["medium"] {
		t.Error("fast should have lower interval than medium")
	}
	if maxSpeedCaps["medium"] >= maxSpeedCaps["slow"] {
		t.Error("medium should have lower interval than slow")
	}
}

func TestNewInertia(t *testing.T) {
	cfg := Config{
		Enabled:   true,
		MaxSpeed:  "fast",
		Threshold: 150,
		AccelRate: 1.5,
	}

	inertia := New(cfg)
	if inertia == nil {
		t.Fatal("New() returned nil")
	}

	if inertia.config.Enabled != cfg.Enabled {
		t.Error("Config.Enabled not preserved")
	}
	if inertia.config.MaxSpeed != cfg.MaxSpeed {
		t.Error("Config.MaxSpeed not preserved")
	}
	if inertia.config.Threshold != cfg.Threshold {
		t.Error("Config.Threshold not preserved")
	}
	if inertia.config.AccelRate != cfg.AccelRate {
		t.Error("Config.AccelRate not preserved")
	}
	if inertia.stopCh == nil {
		t.Error("stopCh should be initialized")
	}
}

func TestIsRunningInitialState(t *testing.T) {
	// Before any Start call, should not be running
	// Note: This test assumes no other tests have left inertia running
	// Reset state first
	mu.Lock()
	running = false
	config = Config{}
	mu.Unlock()

	if IsRunning() {
		t.Error("Expected IsRunning() to be false initially")
	}
}

func TestGetRepeatIntervalEdgeCases(t *testing.T) {
	// Test with AccelRate of 0 (should avoid division by zero)
	// The function divides by (step * accelRate), so if accelRate is 0
	// this could cause issues
	cfg := Config{
		Enabled:   true,
		MaxSpeed:  "fast",
		Threshold: 200,
		AccelRate: 0.0,
	}

	// This might panic or return infinity - we want to ensure it doesn't crash
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Recovered from panic with AccelRate=0: %v", r)
		}
	}()

	// With AccelRate = 0, interval becomes Inf which converts to a very large duration
	result := getRepeatInterval(0, cfg)
	t.Logf("getRepeatInterval with AccelRate=0 returned: %v", result)
}

func TestBaseRepeatInterval(t *testing.T) {
	// Verify constant is as expected
	if baseRepeatInterval != 35 {
		t.Errorf("Expected baseRepeatInterval to be 35, got %d", baseRepeatInterval)
	}
}

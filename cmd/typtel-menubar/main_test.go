//go:build darwin
// +build darwin

package main

import (
	"strings"
	"testing"
)

func TestFormatAbsolute(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{"zero", 0, "0"},
		{"single digit", 5, "5"},
		{"double digit", 42, "42"},
		{"triple digit", 100, "100"},
		{"999", 999, "999"},
		{"1000", 1000, "1,000"},
		{"1234", 1234, "1,234"},
		{"12345", 12345, "12,345"},
		{"123456", 123456, "123,456"},
		{"1234567", 1234567, "1,234,567"},
		{"one million", 1000000, "1,000,000"},
		{"one billion", 1000000000, "1,000,000,000"},
		{"negative", -100, "-100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAbsolute(tt.input)
			if result != tt.expected {
				t.Errorf("formatAbsolute(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetHeatmapColor(t *testing.T) {
	tests := []struct {
		name     string
		value    int64
		max      int64
		expected string
	}{
		{"zero value", 0, 100, "#1a1a2e"},
		{"ratio 0.1 (< 0.25)", 10, 100, "#2d4a3e"},
		{"ratio 0.24 (< 0.25)", 24, 100, "#2d4a3e"},
		{"ratio 0.25 (boundary)", 25, 100, "#3d6b4f"},
		{"ratio 0.4 (< 0.5)", 40, 100, "#3d6b4f"},
		{"ratio 0.5 (boundary)", 50, 100, "#5a9a6f"},
		{"ratio 0.6 (< 0.75)", 60, 100, "#5a9a6f"},
		{"ratio 0.75 (boundary)", 75, 100, "#7bc96f"},
		{"ratio 1.0 (max)", 100, 100, "#7bc96f"},
		{"ratio > 1", 150, 100, "#7bc96f"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getHeatmapColor(tt.value, tt.max)
			if result != tt.expected {
				t.Errorf("getHeatmapColor(%d, %d) = %q, want %q",
					tt.value, tt.max, result, tt.expected)
			}
		})
	}
}

func TestIsWordBoundary(t *testing.T) {
	tests := []struct {
		name     string
		keycode  int
		expected bool
	}{
		{"space (49)", 49, true},
		{"return (36)", 36, true},
		{"tab (48)", 48, true},
		{"letter a (0)", 0, false},
		{"letter s (1)", 1, false},
		{"number 1 (18)", 18, false},
		{"backspace (51)", 51, false},
		{"escape (53)", 53, false},
		{"random keycode", 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWordBoundary(tt.keycode)
			if result != tt.expected {
				t.Errorf("isWordBoundary(%d) = %v, want %v",
					tt.keycode, result, tt.expected)
			}
		})
	}
}

func TestGenerateHourLabels(t *testing.T) {
	labels := generateHourLabels()

	// Should have labels for hours (0, 3, 6, 9, 12, 15, 18, 21)
	if !strings.Contains(labels, ">0<") {
		t.Error("Expected '0' hour label")
	}
	if !strings.Contains(labels, ">6<") {
		t.Error("Expected '6' hour label")
	}
	if !strings.Contains(labels, ">12<") {
		t.Error("Expected '12' hour label")
	}
	if !strings.Contains(labels, ">18<") {
		t.Error("Expected '18' hour label")
	}

	// Should be properly formatted HTML
	if !strings.Contains(labels, "<div") {
		t.Error("Expected HTML div elements in labels")
	}
	if !strings.Contains(labels, "hour-label") {
		t.Error("Expected 'hour-label' class in labels")
	}
}

func TestFormatDistanceShort(t *testing.T) {
	tests := []struct {
		name     string
		pixels   float64
		contains string
	}{
		{"small distance", 100, "in"},       // inches
		{"medium distance", 10000, "ft"},    // feet
		{"large distance", 100000000, "mi"}, // miles (need more pixels at ~100 PPI)
		{"zero distance", 0, "0"},           // zero
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDistanceShort(tt.pixels)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("formatDistanceShort(%f) = %q, expected to contain %q",
					tt.pixels, result, tt.contains)
			}
		})
	}
}

func TestFormatDistanceShortUnitConversion(t *testing.T) {
	// Test that very large distances use miles
	result := formatDistanceShort(10000000) // Very large
	if !strings.Contains(result, "mi") {
		t.Errorf("Expected miles for large distance, got %q", result)
	}

	// Test that small distances use inches
	result = formatDistanceShort(10) // Very small
	if !strings.Contains(result, "in") {
		t.Errorf("Expected inches for small distance, got %q", result)
	}
}

func TestHeatmapColorConsistency(t *testing.T) {
	// Verify colors are in increasing intensity order
	colors := []string{
		getHeatmapColor(0, 100),   // darkest
		getHeatmapColor(10, 100),  // light
		getHeatmapColor(30, 100),  // medium-light
		getHeatmapColor(60, 100),  // medium
		getHeatmapColor(100, 100), // brightest
	}

	// All colors should be valid hex
	for i, c := range colors {
		if !strings.HasPrefix(c, "#") || len(c) != 7 {
			t.Errorf("Color %d (%q) is not a valid hex color", i, c)
		}
	}

	// First should be the zero/darkest color
	if colors[0] != "#1a1a2e" {
		t.Errorf("Expected zero value to be darkest color, got %q", colors[0])
	}

	// Last should be the max/brightest color
	if colors[4] != "#7bc96f" {
		t.Errorf("Expected max value to be brightest color, got %q", colors[4])
	}
}

func TestGetHeatmapColorEdgeCases(t *testing.T) {
	// Test with max = 0 (would cause division by zero without guard)
	// The function should handle this gracefully
	result := getHeatmapColor(0, 0)
	if result != "#1a1a2e" {
		t.Logf("getHeatmapColor(0, 0) = %q", result)
	}

	// Test with very large numbers
	result = getHeatmapColor(1000000000, 1000000000)
	if result != "#7bc96f" {
		t.Errorf("Expected max color for equal large values, got %q", result)
	}
}

func TestFormatAbsoluteEdgeCases(t *testing.T) {
	// Test very large numbers
	result := formatAbsolute(123456789012)
	if !strings.Contains(result, ",") {
		t.Error("Expected commas in large number")
	}

	// Count commas: 123,456,789,012 has 3 commas
	commaCount := strings.Count(result, ",")
	if commaCount != 3 {
		t.Errorf("Expected 3 commas in %q, got %d", result, commaCount)
	}

	// Verify exact format
	if result != "123,456,789,012" {
		t.Errorf("formatAbsolute(123456789012) = %q, want '123,456,789,012'", result)
	}
}

func TestShowPermissionAlert(t *testing.T) {
	// This just prints to stdout - verify it doesn't panic
	showPermissionAlert()
}

func TestGenerateHeatmapHTML(t *testing.T) {
	// Test with empty data
	result := generateHeatmapHTML(map[string][]HourlyStats{}, 7)
	if result != "" {
		t.Logf("Empty heatmap result: %q", result)
	}

	// Test with some data
	hourlyData := map[string][]HourlyStats{
		"2024-01-01": {
			{Hour: 9, Keystrokes: 100},
			{Hour: 10, Keystrokes: 200},
		},
		"2024-01-02": {
			{Hour: 9, Keystrokes: 50},
			{Hour: 10, Keystrokes: 300},
		},
	}
	result = generateHeatmapHTML(hourlyData, 2)

	// Should contain HTML elements
	if !strings.Contains(result, "heatmap-row") {
		t.Error("Expected heatmap-row class in result")
	}
	if !strings.Contains(result, "heatmap-cell") {
		t.Error("Expected heatmap-cell class in result")
	}
	if !strings.Contains(result, "heatmap-label") {
		t.Error("Expected heatmap-label class in result")
	}
}

func TestGenerateHeatmapHTMLMaxValue(t *testing.T) {
	// Test that max value detection works correctly
	hourlyData := map[string][]HourlyStats{
		"2024-01-01": {
			{Hour: 9, Keystrokes: 1000}, // This is max
			{Hour: 10, Keystrokes: 100},
		},
	}
	result := generateHeatmapHTML(hourlyData, 1)

	// Max value cell should get brightest color
	if !strings.Contains(result, "#7bc96f") {
		t.Error("Expected brightest color for max value")
	}
}

func TestGenerateHeatmapHTMLDatesSorted(t *testing.T) {
	// Test that dates appear in sorted order
	hourlyData := map[string][]HourlyStats{
		"2024-01-03": {{Hour: 9, Keystrokes: 100}},
		"2024-01-01": {{Hour: 9, Keystrokes: 100}},
		"2024-01-02": {{Hour: 9, Keystrokes: 100}},
	}
	result := generateHeatmapHTML(hourlyData, 3)

	// Check order by finding positions
	jan1Pos := strings.Index(result, "Jan 1")
	jan2Pos := strings.Index(result, "Jan 2")
	jan3Pos := strings.Index(result, "Jan 3")

	if jan1Pos > jan2Pos || jan2Pos > jan3Pos {
		t.Error("Expected dates to be in sorted order")
	}
}

func TestGetLogDir(t *testing.T) {
	dir, err := getLogDir()
	if err != nil {
		t.Errorf("getLogDir() error = %v", err)
	}

	// Should return a path containing typtel
	if !strings.Contains(dir, "typtel") {
		t.Errorf("getLogDir() = %q, expected to contain 'typtel'", dir)
	}

	// Should return a path containing logs
	if !strings.Contains(dir, "logs") {
		t.Errorf("getLogDir() = %q, expected to contain 'logs'", dir)
	}
}

func TestVersionVariable(t *testing.T) {
	// Version should be set (either "dev" or a real version)
	if Version == "" {
		t.Error("Version should not be empty")
	}
}

func TestAppStartedChannelExists(t *testing.T) {
	// appStarted channel should be initialized
	if appStarted == nil {
		t.Error("appStarted channel should not be nil")
	}
}

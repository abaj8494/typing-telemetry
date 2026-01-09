package stats

import (
	"testing"
	"time"
)

func TestCalculateWeeklyAverage(t *testing.T) {
	tests := []struct {
		name     string
		days     []DayData
		expected float64
	}{
		{
			name:     "empty data",
			days:     []DayData{},
			expected: 0,
		},
		{
			name: "single day",
			days: []DayData{
				{Date: time.Now(), Keystrokes: 1000, Words: 200},
			},
			expected: 1000,
		},
		{
			name: "multiple days",
			days: []DayData{
				{Date: time.Now(), Keystrokes: 1000, Words: 200},
				{Date: time.Now().AddDate(0, 0, -1), Keystrokes: 2000, Words: 400},
				{Date: time.Now().AddDate(0, 0, -2), Keystrokes: 3000, Words: 600},
			},
			expected: 2000, // (1000 + 2000 + 3000) / 3
		},
		{
			name: "week of data",
			days: []DayData{
				{Keystrokes: 1000}, {Keystrokes: 1500}, {Keystrokes: 2000},
				{Keystrokes: 1800}, {Keystrokes: 2200}, {Keystrokes: 1700}, {Keystrokes: 800},
			},
			expected: 1571.4285714285713, // 11000 / 7
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateWeeklyAverage(tt.days)
			if result != tt.expected {
				t.Errorf("CalculateWeeklyAverage() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFindPeakHour(t *testing.T) {
	tests := []struct {
		name          string
		hourlyData    []int64
		expectedHour  int
		expectedCount int64
	}{
		{
			name:          "empty data",
			hourlyData:    []int64{},
			expectedHour:  0,
			expectedCount: 0,
		},
		{
			name:          "single hour",
			hourlyData:    []int64{100},
			expectedHour:  0,
			expectedCount: 100,
		},
		{
			name:          "peak at midnight",
			hourlyData:    []int64{500, 100, 200, 300},
			expectedHour:  0,
			expectedCount: 500,
		},
		{
			name:          "peak in afternoon",
			hourlyData:    make24HoursWithPeakAt(14, 1500),
			expectedHour:  14,
			expectedCount: 1500,
		},
		{
			name:          "all zeros",
			hourlyData:    make([]int64, 24),
			expectedHour:  0,
			expectedCount: 0,
		},
		{
			name:          "equal values",
			hourlyData:    []int64{100, 100, 100, 100},
			expectedHour:  0, // First occurrence wins
			expectedCount: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hour, count := FindPeakHour(tt.hourlyData)
			if hour != tt.expectedHour || count != tt.expectedCount {
				t.Errorf("FindPeakHour() = (%v, %v), want (%v, %v)", hour, count, tt.expectedHour, tt.expectedCount)
			}
		})
	}
}

func TestFormatKeystrokeCount(t *testing.T) {
	tests := []struct {
		name     string
		count    int64
		expected string
	}{
		{"zero", 0, "0"},
		{"single digit", 5, "5"},
		{"double digit", 42, "42"},
		{"triple digit", 999, "999"},
		{"just under 1K", 999, "999"},
		{"exactly 1K", 1000, "1K"},
		{"1.5K", 1500, "1.5K"},
		{"10K", 10000, "10K"},
		{"100K", 100000, "100K"},
		{"just under 1M", 999999, "999.9K"},
		{"exactly 1M", 1000000, "1M"},
		{"1.5M", 1500000, "1.5M"},
		{"10M", 10000000, "10M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatKeystrokeCount(tt.count)
			if result != tt.expected {
				t.Errorf("FormatKeystrokeCount(%d) = %q, want %q", tt.count, result, tt.expected)
			}
		})
	}
}

func TestFormatInt(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{100, "100"},
		{1234, "1234"},
		{1000000, "1000000"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatInt(tt.input)
			if result != tt.expected {
				t.Errorf("formatInt(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Helper function to create 24-hour data with a peak at specific hour
func make24HoursWithPeakAt(peakHour int, peakValue int64) []int64 {
	data := make([]int64, 24)
	for i := range data {
		data[i] = 100 // baseline
	}
	data[peakHour] = peakValue
	return data
}

package main

import (
	"testing"
)

func TestRootCmdExists(t *testing.T) {
	if rootCmd == nil {
		t.Fatal("rootCmd should not be nil")
	}

	if rootCmd.Use != "typtel" {
		t.Errorf("rootCmd.Use = %q, want 'typtel'", rootCmd.Use)
	}

	if rootCmd.Short == "" {
		t.Error("rootCmd.Short should not be empty")
	}
}

func TestStatsCmdExists(t *testing.T) {
	if statsCmd == nil {
		t.Fatal("statsCmd should not be nil")
	}

	if statsCmd.Use != "stats" {
		t.Errorf("statsCmd.Use = %q, want 'stats'", statsCmd.Use)
	}
}

func TestTodayCmdExists(t *testing.T) {
	if todayCmd == nil {
		t.Fatal("todayCmd should not be nil")
	}

	if todayCmd.Use != "today" {
		t.Errorf("todayCmd.Use = %q, want 'today'", todayCmd.Use)
	}
}

func TestTestCmdExists(t *testing.T) {
	if testCmd == nil {
		t.Fatal("testCmd should not be nil")
	}

	if testCmd.Use != "test" {
		t.Errorf("testCmd.Use = %q, want 'test'", testCmd.Use)
	}
}

func TestTestCmdFlags(t *testing.T) {
	// Check that the flags exist
	fileFlag := testCmd.Flags().Lookup("file")
	if fileFlag == nil {
		t.Error("testCmd should have a 'file' flag")
	}
	if fileFlag != nil && fileFlag.Shorthand != "f" {
		t.Errorf("file flag shorthand = %q, want 'f'", fileFlag.Shorthand)
	}

	wordsFlag := testCmd.Flags().Lookup("words")
	if wordsFlag == nil {
		t.Error("testCmd should have a 'words' flag")
	}
	if wordsFlag != nil && wordsFlag.Shorthand != "w" {
		t.Errorf("words flag shorthand = %q, want 'w'", wordsFlag.Shorthand)
	}
	if wordsFlag != nil && wordsFlag.DefValue != "25" {
		t.Errorf("words flag default = %q, want '25'", wordsFlag.DefValue)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	commands := rootCmd.Commands()

	// Check that we have the expected subcommands
	cmdNames := make(map[string]bool)
	for _, cmd := range commands {
		cmdNames[cmd.Use] = true
	}

	expectedCmds := []string{"stats", "today", "test"}
	for _, name := range expectedCmds {
		if !cmdNames[name] {
			t.Errorf("rootCmd should have subcommand %q", name)
		}
	}
}

func TestFormatNum(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{"zero", 0, "0"},
		{"single digit", 5, "5"},
		{"double digit", 42, "42"},
		{"triple digit", 100, "100"},
		{"just under 1K", 999, "999"},
		{"exactly 1K", 1000, "1.0K"},
		{"1.5K", 1500, "1.5K"},
		{"5K", 5000, "5.0K"},
		{"10K", 10000, "10.0K"},
		{"99.9K", 99900, "99.9K"},
		{"100K", 100000, "100.0K"},
		{"just under 1M", 999999, "1000.0K"},
		{"exactly 1M", 1000000, "1.0M"},
		{"1.5M", 1500000, "1.5M"},
		{"5M", 5000000, "5.0M"},
		{"10M", 10000000, "10.0M"},
		{"100M", 100000000, "100.0M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatNum(tt.input)
			if result != tt.expected {
				t.Errorf("formatNum(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatNumRounding(t *testing.T) {
	// Test rounding behavior for K suffix
	// Note: Go's float formatting uses banker's rounding
	tests := []struct {
		input    int64
		expected string
	}{
		{1049, "1.0K"}, // 1.049 rounds down to 1.0
		{1050, "1.1K"}, // 1.050 rounds to 1.1
		{1450, "1.4K"}, // 1.450 rounds down to 1.4 (banker's rounding)
		{1550, "1.6K"}, // 1.550 rounds up to 1.6
		{1555, "1.6K"}, // 1.555 rounds to 1.6
	}

	for _, tt := range tests {
		result := formatNum(tt.input)
		if result != tt.expected {
			t.Errorf("formatNum(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFormatNumEdgeCases(t *testing.T) {
	// Test boundary conditions
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{"negative (displayed as is)", -100, "-100"},
		{"K boundary minus 1", 999, "999"},
		{"K boundary", 1000, "1.0K"},
		{"M boundary minus 1", 999999, "1000.0K"},
		{"M boundary", 1000000, "1.0M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatNum(tt.input)
			if result != tt.expected {
				t.Errorf("formatNum(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

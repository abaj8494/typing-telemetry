package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// newTestStore creates a test store with a temporary database
func newTestStore(t *testing.T) (*Store, func()) {
	t.Helper()

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "typtel-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open database: %v", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to init schema: %v", err)
	}

	store := &Store{db: db}
	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

func TestRecordKeystroke(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record a keystroke
	err := store.RecordKeystroke(42)
	if err != nil {
		t.Fatalf("RecordKeystroke failed: %v", err)
	}

	// Verify it was recorded
	stats, err := store.GetTodayStats()
	if err != nil {
		t.Fatalf("GetTodayStats failed: %v", err)
	}

	if stats.Keystrokes != 1 {
		t.Errorf("Expected 1 keystroke, got %d", stats.Keystrokes)
	}

	// Record more keystrokes
	for i := 0; i < 99; i++ {
		if err := store.RecordKeystroke(i % 50); err != nil {
			t.Fatalf("RecordKeystroke failed: %v", err)
		}
	}

	stats, err = store.GetTodayStats()
	if err != nil {
		t.Fatalf("GetTodayStats failed: %v", err)
	}

	if stats.Keystrokes != 100 {
		t.Errorf("Expected 100 keystrokes, got %d", stats.Keystrokes)
	}
}

func TestIncrementWordCount(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	date := time.Now().Format("2006-01-02")

	// Increment word count
	err := store.IncrementWordCount(date)
	if err != nil {
		t.Fatalf("IncrementWordCount failed: %v", err)
	}

	stats, err := store.GetTodayStats()
	if err != nil {
		t.Fatalf("GetTodayStats failed: %v", err)
	}

	if stats.Words != 1 {
		t.Errorf("Expected 1 word, got %d", stats.Words)
	}

	// Increment more
	for i := 0; i < 49; i++ {
		store.IncrementWordCount(date)
	}

	stats, _ = store.GetTodayStats()
	if stats.Words != 50 {
		t.Errorf("Expected 50 words, got %d", stats.Words)
	}
}

func TestGetDayStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Get stats for non-existent day
	stats, err := store.GetDayStats("2020-01-01")
	if err != nil {
		t.Fatalf("GetDayStats failed: %v", err)
	}

	if stats.Keystrokes != 0 || stats.Words != 0 {
		t.Errorf("Expected 0 keystrokes and 0 words for empty day, got %d and %d", stats.Keystrokes, stats.Words)
	}

	if stats.Date != "2020-01-01" {
		t.Errorf("Expected date 2020-01-01, got %s", stats.Date)
	}
}

func TestGetWeekStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record some keystrokes
	for i := 0; i < 10; i++ {
		store.RecordKeystroke(i)
	}

	stats, err := store.GetWeekStats()
	if err != nil {
		t.Fatalf("GetWeekStats failed: %v", err)
	}

	if len(stats) != 7 {
		t.Errorf("Expected 7 days of stats, got %d", len(stats))
	}

	// Today should have 10 keystrokes
	todayStats := stats[6]
	if todayStats.Keystrokes != 10 {
		t.Errorf("Expected 10 keystrokes for today, got %d", todayStats.Keystrokes)
	}
}

func TestGetHourlyStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record keystrokes
	for i := 0; i < 5; i++ {
		store.RecordKeystroke(42)
	}

	date := time.Now().Format("2006-01-02")
	stats, err := store.GetHourlyStats(date)
	if err != nil {
		t.Fatalf("GetHourlyStats failed: %v", err)
	}

	if len(stats) != 24 {
		t.Errorf("Expected 24 hours of stats, got %d", len(stats))
	}

	// Current hour should have 5 keystrokes
	currentHour := time.Now().Hour()
	if stats[currentHour].Keystrokes != 5 {
		t.Errorf("Expected 5 keystrokes for current hour, got %d", stats[currentHour].Keystrokes)
	}

	// Verify hour is set correctly
	for i, stat := range stats {
		if stat.Hour != i {
			t.Errorf("Expected hour %d, got %d", i, stat.Hour)
		}
	}
}

func TestSettings(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test SetSetting and GetSetting
	err := store.SetSetting("test_key", "test_value")
	if err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}

	value, err := store.GetSetting("test_key")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}

	if value != "test_value" {
		t.Errorf("Expected 'test_value', got %q", value)
	}

	// Test updating setting
	err = store.SetSetting("test_key", "new_value")
	if err != nil {
		t.Fatalf("SetSetting (update) failed: %v", err)
	}

	value, _ = store.GetSetting("test_key")
	if value != "new_value" {
		t.Errorf("Expected 'new_value', got %q", value)
	}

	// Test non-existent key
	value, err = store.GetSetting("nonexistent")
	if err != nil {
		t.Fatalf("GetSetting for nonexistent key failed: %v", err)
	}
	if value != "" {
		t.Errorf("Expected empty string for nonexistent key, got %q", value)
	}
}

func TestMenubarSettings(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default settings
	settings := store.GetMenubarSettings()
	if !settings.ShowKeystrokes {
		t.Error("Expected ShowKeystrokes to default to true")
	}
	if !settings.ShowWords {
		t.Error("Expected ShowWords to default to true")
	}
	if settings.ShowClicks {
		t.Error("Expected ShowClicks to default to false")
	}
	if settings.ShowDistance {
		t.Error("Expected ShowDistance to default to false")
	}

	// Modify and save settings
	settings.ShowKeystrokes = false
	settings.ShowClicks = true
	err := store.SaveMenubarSettings(settings)
	if err != nil {
		t.Fatalf("SaveMenubarSettings failed: %v", err)
	}

	// Retrieve and verify
	settings = store.GetMenubarSettings()
	if settings.ShowKeystrokes {
		t.Error("Expected ShowKeystrokes to be false after save")
	}
	if settings.ShowClicks != true {
		t.Error("Expected ShowClicks to be true after save")
	}
}

func TestInertiaSettings(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default settings
	settings := store.GetInertiaSettings()
	if settings.Enabled {
		t.Error("Expected Enabled to default to false")
	}
	if settings.MaxSpeed != InertiaSpeedFast {
		t.Errorf("Expected MaxSpeed to default to %q, got %q", InertiaSpeedFast, settings.MaxSpeed)
	}
	if settings.Threshold != 200 {
		t.Errorf("Expected Threshold to default to 200, got %d", settings.Threshold)
	}
	if settings.AccelRate != 1.0 {
		t.Errorf("Expected AccelRate to default to 1.0, got %f", settings.AccelRate)
	}

	// Modify settings
	store.SetInertiaEnabled(true)
	store.SetInertiaMaxSpeed(InertiaSpeedVeryFast)
	store.SetInertiaThreshold(150)
	store.SetInertiaAccelRate(1.5)

	// Verify
	settings = store.GetInertiaSettings()
	if !settings.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if settings.MaxSpeed != InertiaSpeedVeryFast {
		t.Errorf("Expected MaxSpeed to be %q, got %q", InertiaSpeedVeryFast, settings.MaxSpeed)
	}
	if settings.Threshold != 150 {
		t.Errorf("Expected Threshold to be 150, got %d", settings.Threshold)
	}
	if settings.AccelRate != 1.5 {
		t.Errorf("Expected AccelRate to be 1.5, got %f", settings.AccelRate)
	}
}

func TestMouseTracking(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default
	if !store.IsMouseTrackingEnabled() {
		t.Error("Expected mouse tracking to be enabled by default")
	}

	// Disable
	store.SetMouseTrackingEnabled(false)
	if store.IsMouseTrackingEnabled() {
		t.Error("Expected mouse tracking to be disabled")
	}

	// Re-enable
	store.SetMouseTrackingEnabled(true)
	if !store.IsMouseTrackingEnabled() {
		t.Error("Expected mouse tracking to be enabled")
	}
}

func TestDistanceUnit(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default
	unit := store.GetDistanceUnit()
	if unit != DistanceUnitFeet {
		t.Errorf("Expected default unit to be %q, got %q", DistanceUnitFeet, unit)
	}

	// Change unit
	store.SetDistanceUnit(DistanceUnitCars)
	unit = store.GetDistanceUnit()
	if unit != DistanceUnitCars {
		t.Errorf("Expected unit to be %q, got %q", DistanceUnitCars, unit)
	}

	store.SetDistanceUnit(DistanceUnitFrisbee)
	unit = store.GetDistanceUnit()
	if unit != DistanceUnitFrisbee {
		t.Errorf("Expected unit to be %q, got %q", DistanceUnitFrisbee, unit)
	}
}

func TestRecordMouseMovement(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record movement
	err := store.RecordMouseMovement(100, 100, 50.5)
	if err != nil {
		t.Fatalf("RecordMouseMovement failed: %v", err)
	}

	stats, err := store.GetTodayMouseStats()
	if err != nil {
		t.Fatalf("GetTodayMouseStats failed: %v", err)
	}

	if stats.TotalDistance != 50.5 {
		t.Errorf("Expected total distance 50.5, got %f", stats.TotalDistance)
	}
	if stats.MovementCount != 1 {
		t.Errorf("Expected movement count 1, got %d", stats.MovementCount)
	}

	// Record more movements
	store.RecordMouseMovement(150, 150, 70.7)
	store.RecordMouseMovement(200, 200, 70.7)

	stats, _ = store.GetTodayMouseStats()
	expectedDistance := 50.5 + 70.7 + 70.7
	if stats.TotalDistance != expectedDistance {
		t.Errorf("Expected total distance %f, got %f", expectedDistance, stats.TotalDistance)
	}
	if stats.MovementCount != 3 {
		t.Errorf("Expected movement count 3, got %d", stats.MovementCount)
	}
}

func TestRecordMouseClick(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record clicks
	for i := 0; i < 10; i++ {
		err := store.RecordMouseClick()
		if err != nil {
			t.Fatalf("RecordMouseClick failed: %v", err)
		}
	}

	stats, err := store.GetTodayMouseStats()
	if err != nil {
		t.Fatalf("GetTodayMouseStats failed: %v", err)
	}

	if stats.ClickCount != 10 {
		t.Errorf("Expected click count 10, got %d", stats.ClickCount)
	}
}

func TestMouseLeaderboard(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record some mouse movements to create leaderboard data
	store.RecordMouseMovement(100, 100, 500)

	entries, err := store.GetMouseLeaderboard(10)
	if err != nil {
		t.Fatalf("GetMouseLeaderboard failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(entries))
	}

	if len(entries) > 0 && entries[0].Rank != 1 {
		t.Errorf("Expected rank 1, got %d", entries[0].Rank)
	}
}

func TestTypingTestStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default stats
	stats := store.GetTypingTestStats()
	if stats.PersonalBest != 0 {
		t.Errorf("Expected default PB to be 0, got %f", stats.PersonalBest)
	}
	if stats.TestCount != 0 {
		t.Errorf("Expected default test count to be 0, got %d", stats.TestCount)
	}

	// Save a result
	err := store.SaveTypingTestResult(75.5)
	if err != nil {
		t.Fatalf("SaveTypingTestResult failed: %v", err)
	}

	stats = store.GetTypingTestStats()
	if stats.PersonalBest != 75.5 {
		t.Errorf("Expected PB to be 75.5, got %f", stats.PersonalBest)
	}
	if stats.TestCount != 1 {
		t.Errorf("Expected test count to be 1, got %d", stats.TestCount)
	}

	// Save a lower result (should not update PB)
	store.SaveTypingTestResult(60.0)
	stats = store.GetTypingTestStats()
	if stats.PersonalBest != 75.5 {
		t.Errorf("Expected PB to remain 75.5, got %f", stats.PersonalBest)
	}
	if stats.TestCount != 2 {
		t.Errorf("Expected test count to be 2, got %d", stats.TestCount)
	}

	// Save a higher result (should update PB)
	store.SaveTypingTestResult(100.0)
	stats = store.GetTypingTestStats()
	if stats.PersonalBest != 100.0 {
		t.Errorf("Expected PB to be 100.0, got %f", stats.PersonalBest)
	}
}

func TestTypingTestModeStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	mode := TypingTestMode{WordCount: 25, Punctuation: true}

	// Save result for mode
	err := store.SaveTypingTestResultForMode(80.0, mode)
	if err != nil {
		t.Fatalf("SaveTypingTestResultForMode failed: %v", err)
	}

	stats := store.GetTypingTestStatsForMode(mode)
	if stats.PersonalBest != 80.0 {
		t.Errorf("Expected mode PB to be 80.0, got %f", stats.PersonalBest)
	}

	// Different mode should have separate stats
	mode2 := TypingTestMode{WordCount: 50, Punctuation: false}
	stats2 := store.GetTypingTestStatsForMode(mode2)
	if stats2.PersonalBest != 0 {
		t.Errorf("Expected different mode PB to be 0, got %f", stats2.PersonalBest)
	}
}

func TestTypingTestModeKey(t *testing.T) {
	tests := []struct {
		mode     TypingTestMode
		expected string
	}{
		{TypingTestMode{WordCount: 10, Punctuation: true}, "mode_10_punct"},
		{TypingTestMode{WordCount: 25, Punctuation: false}, "mode_25_no_punct"},
		{TypingTestMode{WordCount: 100, Punctuation: true}, "mode_100_punct"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.mode.ModeKey()
			if result != tt.expected {
				t.Errorf("ModeKey() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTypingTestTheme(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default
	theme := store.GetTypingTestTheme()
	if theme != "default" {
		t.Errorf("Expected default theme, got %q", theme)
	}

	// Set theme
	store.SetTypingTestTheme("dracula")
	theme = store.GetTypingTestTheme()
	if theme != "dracula" {
		t.Errorf("Expected 'dracula' theme, got %q", theme)
	}
}

func TestTypingTestCustomTexts(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default (empty)
	texts := store.GetTypingTestCustomTexts()
	if texts != "" {
		t.Errorf("Expected empty custom texts, got %q", texts)
	}

	// Set custom texts
	customTexts := "First custom text\n---\nSecond custom text"
	store.SetTypingTestCustomTexts(customTexts)

	texts = store.GetTypingTestCustomTexts()
	if texts != customTexts {
		t.Errorf("Expected %q, got %q", customTexts, texts)
	}
}

func TestHelperFunctions(t *testing.T) {
	// Test abs
	if abs(-5.0) != 5.0 {
		t.Error("abs(-5.0) should be 5.0")
	}
	if abs(5.0) != 5.0 {
		t.Error("abs(5.0) should be 5.0")
	}
	if abs(0) != 0 {
		t.Error("abs(0) should be 0")
	}

	// Test boolToString
	if boolToString(true) != "true" {
		t.Error("boolToString(true) should be 'true'")
	}
	if boolToString(false) != "false" {
		t.Error("boolToString(false) should be 'false'")
	}

	// Test parseInt
	v, err := parseInt("123")
	if err != nil || v != 123 {
		t.Errorf("parseInt('123') = %d, %v; want 123, nil", v, err)
	}

	// Test parseFloat
	f, err := parseFloat("3.14")
	if err != nil || f != 3.14 {
		t.Errorf("parseFloat('3.14') = %f, %v; want 3.14, nil", f, err)
	}

	// Test intToString
	if intToString(42) != "42" {
		t.Error("intToString(42) should be '42'")
	}

	// Test floatToString
	if floatToString(3.14159) != "3.14" {
		t.Errorf("floatToString(3.14159) = %q, want '3.14'", floatToString(3.14159))
	}
}

package storage

import (
	"database/sql"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

type DailyStats struct {
	Date       string
	Keystrokes int64
	Words      int64
}

type HourlyStats struct {
	Hour       int
	Keystrokes int64
}

// MouseDailyStats represents mouse movement statistics for a day
type MouseDailyStats struct {
	Date           string
	TotalDistance  float64 // Total Euclidean distance traveled in pixels
	MidnightX      float64 // Mouse X position at midnight (or start of tracking)
	MidnightY      float64 // Mouse Y position at midnight (or start of tracking)
	CurrentX       float64 // Current mouse X position
	CurrentY       float64 // Current mouse Y position
	MAEFromOrigin  float64 // Mean Absolute Error from midnight position
	MovementCount  int64   // Number of movement events recorded
	ClickCount     int64   // Number of mouse clicks
}

// MouseLeaderboardEntry represents a day in the "least mouse movement" leaderboard
type MouseLeaderboardEntry struct {
	Date          string
	TotalDistance float64
	Rank          int
}

func getDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback: get home dir from user.Current()
		if u, userErr := user.Current(); userErr == nil {
			home = u.HomeDir
		} else {
			return "", err
		}
	}
	dataDir := filepath.Join(home, ".local", "share", "typtel")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", err
	}
	return dataDir, nil
}

func New() (*Store, error) {
	dataDir, err := getDataDir()
	if err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dataDir, "typtel.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err := initSchema(db); err != nil {
		return nil, err
	}

	return &Store{db: db}, nil
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS keystrokes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		keycode INTEGER,
		date TEXT,
		hour INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_keystrokes_date ON keystrokes(date);
	CREATE INDEX IF NOT EXISTS idx_keystrokes_hour ON keystrokes(date, hour);

	CREATE TABLE IF NOT EXISTS daily_summary (
		date TEXT PRIMARY KEY,
		keystrokes INTEGER DEFAULT 0,
		words INTEGER DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS mouse_daily (
		date TEXT PRIMARY KEY,
		total_distance REAL DEFAULT 0,
		midnight_x REAL DEFAULT 0,
		midnight_y REAL DEFAULT 0,
		current_x REAL DEFAULT 0,
		current_y REAL DEFAULT 0,
		sum_abs_error REAL DEFAULT 0,
		movement_count INTEGER DEFAULT 0,
		click_count INTEGER DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_mouse_daily_distance ON mouse_daily(total_distance);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT
	);
	`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	// Add click_count column if it doesn't exist (migration for existing DBs)
	_, _ = db.Exec("ALTER TABLE mouse_daily ADD COLUMN click_count INTEGER DEFAULT 0")

	return nil
}

func (s *Store) RecordKeystroke(keycode int) error {
	now := time.Now()
	date := now.Format("2006-01-02")
	hour := now.Hour()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"INSERT INTO keystrokes (keycode, date, hour) VALUES (?, ?, ?)",
		keycode, date, hour,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		INSERT INTO daily_summary (date, keystrokes) VALUES (?, 1)
		ON CONFLICT(date) DO UPDATE SET
			keystrokes = keystrokes + 1,
			updated_at = CURRENT_TIMESTAMP
	`, date)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) IncrementWordCount(date string) error {
	_, err := s.db.Exec(`
		INSERT INTO daily_summary (date, words) VALUES (?, 1)
		ON CONFLICT(date) DO UPDATE SET
			words = words + 1,
			updated_at = CURRENT_TIMESTAMP
	`, date)
	return err
}

func (s *Store) GetTodayStats() (*DailyStats, error) {
	date := time.Now().Format("2006-01-02")
	return s.GetDayStats(date)
}

func (s *Store) GetDayStats(date string) (*DailyStats, error) {
	var stats DailyStats
	stats.Date = date

	err := s.db.QueryRow(
		"SELECT COALESCE(keystrokes, 0), COALESCE(words, 0) FROM daily_summary WHERE date = ?",
		date,
	).Scan(&stats.Keystrokes, &stats.Words)

	if err == sql.ErrNoRows {
		return &DailyStats{Date: date, Keystrokes: 0, Words: 0}, nil
	}
	if err != nil {
		return nil, err
	}

	return &stats, nil
}

func (s *Store) GetWeekStats() ([]DailyStats, error) {
	now := time.Now()
	stats := make([]DailyStats, 7)

	for i := 6; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		dayStat, err := s.GetDayStats(date)
		if err != nil {
			return nil, err
		}
		stats[6-i] = *dayStat
	}

	return stats, nil
}

func (s *Store) GetHourlyStats(date string) ([]HourlyStats, error) {
	stats := make([]HourlyStats, 24)
	for i := range stats {
		stats[i].Hour = i
	}

	rows, err := s.db.Query(
		"SELECT hour, COUNT(*) FROM keystrokes WHERE date = ? GROUP BY hour",
		date,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var hour int
		var count int64
		if err := rows.Scan(&hour, &count); err != nil {
			return nil, err
		}
		if hour >= 0 && hour < 24 {
			stats[hour].Keystrokes = count
		}
	}

	return stats, nil
}

// GetHistoricalStats returns stats for the last N days
func (s *Store) GetHistoricalStats(days int) ([]DailyStats, error) {
	now := time.Now()
	stats := make([]DailyStats, days)

	for i := days - 1; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		dayStat, err := s.GetDayStats(date)
		if err != nil {
			return nil, err
		}
		stats[days-1-i] = *dayStat
	}

	return stats, nil
}

// GetAllHourlyStatsForDays returns hourly stats for multiple days (for heatmap)
func (s *Store) GetAllHourlyStatsForDays(days int) (map[string][]HourlyStats, error) {
	result := make(map[string][]HourlyStats)
	now := time.Now()

	for i := days - 1; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		hourlyStats, err := s.GetHourlyStats(date)
		if err != nil {
			return nil, err
		}
		result[date] = hourlyStats
	}

	return result, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// RecordMouseMovement records a mouse movement event with distance traveled
func (s *Store) RecordMouseMovement(x, y, distance float64) error {
	now := time.Now()
	date := now.Format("2006-01-02")

	// Calculate absolute error from midnight position
	// We'll need to get the midnight position first
	var midnightX, midnightY float64
	var exists bool

	err := s.db.QueryRow(
		"SELECT midnight_x, midnight_y FROM mouse_daily WHERE date = ?",
		date,
	).Scan(&midnightX, &midnightY)

	if err == sql.ErrNoRows {
		// First movement of the day - set midnight position to current
		midnightX = x
		midnightY = y
		exists = false
	} else if err != nil {
		return err
	} else {
		exists = true
	}

	// Calculate absolute error from midnight position
	absError := abs(x-midnightX) + abs(y-midnightY)

	if !exists {
		// Insert new record for the day
		_, err = s.db.Exec(`
			INSERT INTO mouse_daily (date, total_distance, midnight_x, midnight_y, current_x, current_y, sum_abs_error, movement_count)
			VALUES (?, ?, ?, ?, ?, ?, ?, 1)
		`, date, distance, midnightX, midnightY, x, y, absError)
	} else {
		// Update existing record
		_, err = s.db.Exec(`
			UPDATE mouse_daily SET
				total_distance = total_distance + ?,
				current_x = ?,
				current_y = ?,
				sum_abs_error = sum_abs_error + ?,
				movement_count = movement_count + 1,
				updated_at = CURRENT_TIMESTAMP
			WHERE date = ?
		`, distance, x, y, absError, date)
	}

	return err
}

// SetMidnightPosition sets the starting mouse position for a new day
func (s *Store) SetMidnightPosition(date string, x, y float64) error {
	_, err := s.db.Exec(`
		INSERT INTO mouse_daily (date, midnight_x, midnight_y, current_x, current_y)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			midnight_x = ?,
			midnight_y = ?,
			updated_at = CURRENT_TIMESTAMP
	`, date, x, y, x, y, x, y)
	return err
}

// GetMouseDailyStats returns mouse movement stats for a specific day
func (s *Store) GetMouseDailyStats(date string) (*MouseDailyStats, error) {
	var stats MouseDailyStats
	stats.Date = date

	err := s.db.QueryRow(`
		SELECT COALESCE(total_distance, 0), COALESCE(midnight_x, 0), COALESCE(midnight_y, 0),
		       COALESCE(current_x, 0), COALESCE(current_y, 0), COALESCE(sum_abs_error, 0),
		       COALESCE(movement_count, 0), COALESCE(click_count, 0)
		FROM mouse_daily WHERE date = ?
	`, date).Scan(&stats.TotalDistance, &stats.MidnightX, &stats.MidnightY,
		&stats.CurrentX, &stats.CurrentY, &stats.MAEFromOrigin, &stats.MovementCount, &stats.ClickCount)

	if err == sql.ErrNoRows {
		return &MouseDailyStats{Date: date}, nil
	}
	if err != nil {
		return nil, err
	}

	// Calculate actual MAE (Mean Absolute Error)
	if stats.MovementCount > 0 {
		stats.MAEFromOrigin = stats.MAEFromOrigin / float64(stats.MovementCount)
	}

	return &stats, nil
}

// RecordMouseClick records a mouse click event
func (s *Store) RecordMouseClick() error {
	now := time.Now()
	date := now.Format("2006-01-02")

	_, err := s.db.Exec(`
		INSERT INTO mouse_daily (date, click_count) VALUES (?, 1)
		ON CONFLICT(date) DO UPDATE SET
			click_count = click_count + 1,
			updated_at = CURRENT_TIMESTAMP
	`, date)
	return err
}

// GetTodayMouseStats returns today's mouse movement stats
func (s *Store) GetTodayMouseStats() (*MouseDailyStats, error) {
	date := time.Now().Format("2006-01-02")
	return s.GetMouseDailyStats(date)
}

// GetMouseLeaderboard returns days with the least mouse movement (stillness leaderboard)
func (s *Store) GetMouseLeaderboard(limit int) ([]MouseLeaderboardEntry, error) {
	rows, err := s.db.Query(`
		SELECT date, total_distance
		FROM mouse_daily
		WHERE movement_count > 0
		ORDER BY total_distance ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []MouseLeaderboardEntry
	rank := 1
	for rows.Next() {
		var entry MouseLeaderboardEntry
		if err := rows.Scan(&entry.Date, &entry.TotalDistance); err != nil {
			return nil, err
		}
		entry.Rank = rank
		entries = append(entries, entry)
		rank++
	}

	return entries, nil
}

// GetMouseHistoricalStats returns mouse stats for the last N days
func (s *Store) GetMouseHistoricalStats(days int) ([]MouseDailyStats, error) {
	now := time.Now()
	stats := make([]MouseDailyStats, days)

	for i := days - 1; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		dayStat, err := s.GetMouseDailyStats(date)
		if err != nil {
			return nil, err
		}
		stats[days-1-i] = *dayStat
	}

	return stats, nil
}

// GetWeekMouseStats returns mouse stats for the last 7 days
func (s *Store) GetWeekMouseStats() ([]MouseDailyStats, error) {
	return s.GetMouseHistoricalStats(7)
}

// Helper function for absolute value
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Settings keys
const (
	SettingShowKeystrokes       = "menubar_show_keystrokes"
	SettingShowWords            = "menubar_show_words"
	SettingShowClicks           = "menubar_show_clicks"
	SettingShowDistance         = "menubar_show_distance"
	SettingMouseTrackingEnabled = "mouse_tracking_enabled"
	SettingDistanceUnit         = "distance_unit"
	// Inertia settings
	SettingInertiaEnabled   = "inertia_enabled"
	SettingInertiaMaxSpeed  = "inertia_max_speed"
	SettingInertiaThreshold = "inertia_threshold"
	SettingInertiaAccelRate = "inertia_accel_rate"
	// Typing test settings
	SettingTypingTestPB      = "typing_test_pb"
	SettingTypingTestAvgWPM  = "typing_test_avg_wpm"
	SettingTypingTestCount   = "typing_test_count"
	SettingTypingTestTheme   = "typing_test_theme"
)

// Distance unit options
const (
	DistanceUnitFeet    = "feet"    // feet/miles (default)
	DistanceUnitCars    = "cars"    // average car length ~15ft
	DistanceUnitFrisbee = "frisbee" // ultimate frisbee field ~330ft
)

// Inertia max speed options (capped at what terminals/editors can handle)
const (
	InertiaSpeedUltraFast = "ultra_fast" // Cap at ~140 keys/sec (pushing limits)
	InertiaSpeedVeryFast  = "very_fast"  // Cap at ~125 keys/sec
	InertiaSpeedFast      = "fast"       // Cap at ~83 keys/sec
	InertiaSpeedMedium    = "medium"     // Cap at ~50 keys/sec
	InertiaSpeedSlow      = "slow"       // Cap at ~20 keys/sec
)

// MenubarSettings represents what to show in the menubar
type MenubarSettings struct {
	ShowKeystrokes bool
	ShowWords      bool
	ShowClicks     bool
	ShowDistance   bool
}

// GetSetting retrieves a setting value
func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetSetting saves a setting value
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?
	`, key, value, value)
	return err
}

// GetMenubarSettings returns the current menubar display settings
func (s *Store) GetMenubarSettings() MenubarSettings {
	settings := MenubarSettings{
		ShowKeystrokes: true, // default on
		ShowWords:      true, // default on
		ShowClicks:     false, // default off
		ShowDistance:   false, // default off (per user request)
	}

	if val, _ := s.GetSetting(SettingShowKeystrokes); val == "false" {
		settings.ShowKeystrokes = false
	} else if val == "true" {
		settings.ShowKeystrokes = true
	}

	if val, _ := s.GetSetting(SettingShowWords); val == "false" {
		settings.ShowWords = false
	} else if val == "true" {
		settings.ShowWords = true
	}

	if val, _ := s.GetSetting(SettingShowClicks); val == "true" {
		settings.ShowClicks = true
	}

	if val, _ := s.GetSetting(SettingShowDistance); val == "true" {
		settings.ShowDistance = true
	}

	return settings
}

// SaveMenubarSettings saves the menubar display settings
func (s *Store) SaveMenubarSettings(settings MenubarSettings) error {
	if err := s.SetSetting(SettingShowKeystrokes, boolToString(settings.ShowKeystrokes)); err != nil {
		return err
	}
	if err := s.SetSetting(SettingShowWords, boolToString(settings.ShowWords)); err != nil {
		return err
	}
	if err := s.SetSetting(SettingShowClicks, boolToString(settings.ShowClicks)); err != nil {
		return err
	}
	return s.SetSetting(SettingShowDistance, boolToString(settings.ShowDistance))
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// IsMouseTrackingEnabled returns whether mouse tracking is enabled (default: true)
func (s *Store) IsMouseTrackingEnabled() bool {
	val, _ := s.GetSetting(SettingMouseTrackingEnabled)
	// Default to enabled if not set
	return val != "false"
}

// SetMouseTrackingEnabled sets whether mouse tracking is enabled
func (s *Store) SetMouseTrackingEnabled(enabled bool) error {
	return s.SetSetting(SettingMouseTrackingEnabled, boolToString(enabled))
}

// GetDistanceUnit returns the current distance unit (default: feet)
func (s *Store) GetDistanceUnit() string {
	val, _ := s.GetSetting(SettingDistanceUnit)
	if val == "" {
		return DistanceUnitFeet
	}
	return val
}

// SetDistanceUnit sets the distance unit
func (s *Store) SetDistanceUnit(unit string) error {
	return s.SetSetting(SettingDistanceUnit, unit)
}

// InertiaSettings represents inertia configuration
type InertiaSettings struct {
	Enabled   bool
	MaxSpeed  string  // "very_fast", "fast", "medium"
	Threshold int     // ms before acceleration starts (default 200)
	AccelRate float64 // acceleration multiplier (default 1.0)
}

// GetInertiaSettings returns the current inertia settings
func (s *Store) GetInertiaSettings() InertiaSettings {
	settings := InertiaSettings{
		Enabled:   false,
		MaxSpeed:  InertiaSpeedFast,
		Threshold: 200,
		AccelRate: 1.0,
	}

	if val, _ := s.GetSetting(SettingInertiaEnabled); val == "true" {
		settings.Enabled = true
	}

	if val, _ := s.GetSetting(SettingInertiaMaxSpeed); val != "" {
		settings.MaxSpeed = val
	}

	if val, _ := s.GetSetting(SettingInertiaThreshold); val != "" {
		if v, err := parseInt(val); err == nil && v > 0 {
			settings.Threshold = v
		}
	}

	if val, _ := s.GetSetting(SettingInertiaAccelRate); val != "" {
		if v, err := parseFloat(val); err == nil && v > 0 {
			settings.AccelRate = v
		}
	}

	return settings
}

// SetInertiaEnabled sets whether inertia is enabled
func (s *Store) SetInertiaEnabled(enabled bool) error {
	return s.SetSetting(SettingInertiaEnabled, boolToString(enabled))
}

// SetInertiaMaxSpeed sets the inertia max speed
func (s *Store) SetInertiaMaxSpeed(speed string) error {
	return s.SetSetting(SettingInertiaMaxSpeed, speed)
}

// SetInertiaThreshold sets the inertia threshold in ms
func (s *Store) SetInertiaThreshold(threshold int) error {
	return s.SetSetting(SettingInertiaThreshold, intToString(threshold))
}

// SetInertiaAccelRate sets the inertia acceleration rate
func (s *Store) SetInertiaAccelRate(rate float64) error {
	return s.SetSetting(SettingInertiaAccelRate, floatToString(rate))
}

// Helper functions for parsing
func parseInt(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

func parseFloat(s string) (float64, error) {
	var v float64
	_, err := fmt.Sscanf(s, "%f", &v)
	return v, err
}

func intToString(i int) string {
	return fmt.Sprintf("%d", i)
}

func floatToString(f float64) string {
	return fmt.Sprintf("%.2f", f)
}

// TypingTestStats holds typing test performance data
type TypingTestStats struct {
	PersonalBest float64 // Best WPM
	AverageWPM   float64 // Running average WPM
	TestCount    int     // Number of tests completed
}

// TypingTestMode represents a specific configuration for typing tests
type TypingTestMode struct {
	WordCount   int
	Punctuation bool
}

// ModeKey generates a unique key for a typing test mode
func (m TypingTestMode) ModeKey() string {
	punct := "no_punct"
	if m.Punctuation {
		punct = "punct"
	}
	return fmt.Sprintf("mode_%d_%s", m.WordCount, punct)
}

// GetTypingTestStats retrieves typing test statistics (global)
func (s *Store) GetTypingTestStats() TypingTestStats {
	stats := TypingTestStats{
		PersonalBest: 0,
		AverageWPM:   50.0, // Default average
		TestCount:    0,
	}

	if val, _ := s.GetSetting(SettingTypingTestPB); val != "" {
		if v, err := parseFloat(val); err == nil {
			stats.PersonalBest = v
		}
	}

	if val, _ := s.GetSetting(SettingTypingTestAvgWPM); val != "" {
		if v, err := parseFloat(val); err == nil {
			stats.AverageWPM = v
		}
	}

	if val, _ := s.GetSetting(SettingTypingTestCount); val != "" {
		if v, err := parseInt(val); err == nil {
			stats.TestCount = v
		}
	}

	return stats
}

// GetTypingTestStatsForMode retrieves typing test statistics for a specific mode
func (s *Store) GetTypingTestStatsForMode(mode TypingTestMode) TypingTestStats {
	modeKey := mode.ModeKey()
	stats := TypingTestStats{
		PersonalBest: 0,
		AverageWPM:   50.0,
		TestCount:    0,
	}

	if val, _ := s.GetSetting(SettingTypingTestPB + "_" + modeKey); val != "" {
		if v, err := parseFloat(val); err == nil {
			stats.PersonalBest = v
		}
	}

	if val, _ := s.GetSetting(SettingTypingTestAvgWPM + "_" + modeKey); val != "" {
		if v, err := parseFloat(val); err == nil {
			stats.AverageWPM = v
		}
	}

	if val, _ := s.GetSetting(SettingTypingTestCount + "_" + modeKey); val != "" {
		if v, err := parseInt(val); err == nil {
			stats.TestCount = v
		}
	}

	return stats
}

// SaveTypingTestResult saves a new typing test result and updates stats (global)
func (s *Store) SaveTypingTestResult(wpm float64) error {
	stats := s.GetTypingTestStats()

	// Update personal best
	if wpm > stats.PersonalBest {
		if err := s.SetSetting(SettingTypingTestPB, floatToString(wpm)); err != nil {
			return err
		}
	}

	// Update running average (weighted average)
	newCount := stats.TestCount + 1
	newAvg := ((stats.AverageWPM * float64(stats.TestCount)) + wpm) / float64(newCount)
	if err := s.SetSetting(SettingTypingTestAvgWPM, floatToString(newAvg)); err != nil {
		return err
	}

	// Update test count
	return s.SetSetting(SettingTypingTestCount, intToString(newCount))
}

// SaveTypingTestResultForMode saves a typing test result for a specific mode
func (s *Store) SaveTypingTestResultForMode(wpm float64, mode TypingTestMode) error {
	modeKey := mode.ModeKey()
	stats := s.GetTypingTestStatsForMode(mode)

	// Update personal best for mode
	if wpm > stats.PersonalBest {
		if err := s.SetSetting(SettingTypingTestPB+"_"+modeKey, floatToString(wpm)); err != nil {
			return err
		}
	}

	// Update running average for mode
	newCount := stats.TestCount + 1
	newAvg := ((stats.AverageWPM * float64(stats.TestCount)) + wpm) / float64(newCount)
	if err := s.SetSetting(SettingTypingTestAvgWPM+"_"+modeKey, floatToString(newAvg)); err != nil {
		return err
	}

	// Update test count for mode
	if err := s.SetSetting(SettingTypingTestCount+"_"+modeKey, intToString(newCount)); err != nil {
		return err
	}

	// Also update global stats
	return s.SaveTypingTestResult(wpm)
}

// GetTypingTestTheme retrieves the saved theme preference
func (s *Store) GetTypingTestTheme() string {
	val, _ := s.GetSetting(SettingTypingTestTheme)
	if val == "" {
		return "default"
	}
	return val
}

// SetTypingTestTheme saves the theme preference
func (s *Store) SetTypingTestTheme(theme string) error {
	return s.SetSetting(SettingTypingTestTheme, theme)
}

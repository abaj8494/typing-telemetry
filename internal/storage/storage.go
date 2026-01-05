package storage

import (
	"database/sql"
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
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_mouse_daily_distance ON mouse_daily(total_distance);
	`
	_, err := db.Exec(schema)
	return err
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
		       COALESCE(movement_count, 0)
		FROM mouse_daily WHERE date = ?
	`, date).Scan(&stats.TotalDistance, &stats.MidnightX, &stats.MidnightY,
		&stats.CurrentX, &stats.CurrentY, &stats.MAEFromOrigin, &stats.MovementCount)

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

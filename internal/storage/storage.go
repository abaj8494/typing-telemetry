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

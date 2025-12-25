package main

import (
	"fmt"
	"os"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/aayushbajaj/typing-telemetry/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "typtel",
	Short: "Typing telemetry - track your keystrokes",
	Long:  `A keystroke tracking tool for developers. Shows daily keystroke counts and statistics.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show typing statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showStats()
	},
}

var todayCmd = &cobra.Command{
	Use:   "today",
	Short: "Show today's keystroke count (for menu bar)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showToday()
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(todayCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runTUI() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	p := tea.NewProgram(tui.New(store), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func showStats() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	today, err := store.GetTodayStats()
	if err != nil {
		return fmt.Errorf("failed to get today's stats: %w", err)
	}

	week, err := store.GetWeekStats()
	if err != nil {
		return fmt.Errorf("failed to get week stats: %w", err)
	}

	var weekTotal int64
	for _, day := range week {
		weekTotal += day.Keystrokes
	}

	fmt.Printf("Today: %d keystrokes\n", today.Keystrokes)
	fmt.Printf("This week: %d keystrokes (avg: %d/day)\n", weekTotal, weekTotal/7)

	return nil
}

func showToday() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	today, err := store.GetTodayStats()
	if err != nil {
		return fmt.Errorf("failed to get today's stats: %w", err)
	}

	// Output format suitable for menu bar scripts
	fmt.Printf("%d", today.Keystrokes)
	return nil
}

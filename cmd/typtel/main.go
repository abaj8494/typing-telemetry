package main

import (
	"fmt"
	"os"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/aayushbajaj/typing-telemetry/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var (
	// Flags for test command
	testFile      string
	testWordCount int
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

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Start a typing test",
	Long: `Start an interactive typing test to measure your WPM and accuracy.

Examples:
  typtel test                    # Default 25-word test
  typtel test -w 50              # 50-word test
  typtel test -f words.txt       # Use custom word list
  typtel test -f passage.txt -w 100  # 100 words from custom file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTypingTest()
	},
}

func init() {
	testCmd.Flags().StringVarP(&testFile, "file", "f", "", "Path to text file with words/passages")
	testCmd.Flags().IntVarP(&testWordCount, "words", "w", 25, "Number of words in the test")

	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(todayCmd)
	rootCmd.AddCommand(testCmd)
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
	model, err := p.Run()
	if err != nil {
		return err
	}

	// Check if user wants to switch to typing test
	if m, ok := model.(tui.Model); ok && m.SwitchToTypingTest {
		return runTypingTest()
	}

	return nil
}

func runTypingTest() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	p := tea.NewProgram(
		tui.NewTypingTestWithStore(testFile, testWordCount, store),
		tea.WithAltScreen(),
	)
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

	// Calculate week words from daily stats
	var weekWords int64
	for _, day := range week {
		weekWords += day.Words
	}

	fmt.Println("📊 Typing Statistics")
	fmt.Println("────────────────────")
	fmt.Printf("Today:     %s keystrokes (%s words)\n", formatNum(today.Keystrokes), formatNum(today.Words))
	fmt.Printf("This week: %s keystrokes (%s words)\n", formatNum(weekTotal), formatNum(weekWords))
	fmt.Printf("Daily avg: %s keystrokes (%s words)\n", formatNum(weekTotal/7), formatNum(weekWords/7))

	return nil
}

func formatNum(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
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
	fmt.Printf("%d\n", today.Keystrokes)
	return nil
}

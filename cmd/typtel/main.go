package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

var viewCmd = &cobra.Command{
	Use:     "v",
	Aliases: []string{"view", "charts"},
	Short:   "View typing statistics charts in browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		return viewCharts()
	},
}

func init() {
	testCmd.Flags().StringVarP(&testFile, "file", "f", "", "Path to text file with words/passages")
	testCmd.Flags().IntVarP(&testWordCount, "words", "w", 25, "Number of words in the test")

	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(todayCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(viewCmd)
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

func viewCharts() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	htmlPath, err := generateChartsHTML(store)
	if err != nil {
		return fmt.Errorf("failed to generate charts: %w", err)
	}

	fmt.Printf("Opening charts: %s\n", htmlPath)
	return exec.Command("open", htmlPath).Start()
}

// DefaultPPI is the default pixels per inch if display info is unavailable
const DefaultPPI = 100.0

func pixelsToFeet(pixels float64) float64 {
	inches := pixels / DefaultPPI
	return inches / 12.0
}

func formatAbsolute(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return s
	}

	result := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

func formatDistance(pixels float64) string {
	feet := pixelsToFeet(pixels)
	if feet >= 5280 {
		return fmt.Sprintf("%.1fmi", feet/5280)
	} else if feet >= 1 {
		return fmt.Sprintf("%.0fft", feet)
	}
	inches := feet * 12
	return fmt.Sprintf("%.0fin", inches)
}

func generateChartsHTML(store *storage.Store) (string, error) {
	prepareChartData := func(days int) (labels, keystrokeData, wordData []string, mouseDataFeet []float64, totalKeystrokes, totalWords int64, totalMouseDistance float64, heatmapHTML string, err error) {
		histStats, err := store.GetHistoricalStats(days)
		if err != nil {
			return nil, nil, nil, nil, 0, 0, 0, "", err
		}

		mouseStats, err := store.GetMouseHistoricalStats(days)
		if err != nil {
			return nil, nil, nil, nil, 0, 0, 0, "", err
		}

		hourlyData, err := store.GetAllHourlyStatsForDays(days)
		if err != nil {
			return nil, nil, nil, nil, 0, 0, 0, "", err
		}

		for i, stat := range histStats {
			t, _ := time.Parse("2006-01-02", stat.Date)
			labels = append(labels, fmt.Sprintf("'%s'", t.Format("Jan 2")))
			keystrokeData = append(keystrokeData, fmt.Sprintf("%d", stat.Keystrokes))
			wordData = append(wordData, fmt.Sprintf("%d", stat.Words))
			totalKeystrokes += stat.Keystrokes
			totalWords += stat.Words

			if i < len(mouseStats) {
				feet := pixelsToFeet(mouseStats[i].TotalDistance)
				mouseDataFeet = append(mouseDataFeet, feet)
				totalMouseDistance += mouseStats[i].TotalDistance
			} else {
				mouseDataFeet = append(mouseDataFeet, 0)
			}
		}

		heatmapHTML = generateHeatmapHTML(hourlyData)
		return
	}

	weeklyLabels, weeklyKeystrokes, weeklyWords, weeklyMouseFeet, weeklyTotalKeys, weeklyTotalWords, weeklyTotalMouse, weeklyHeatmap, err := prepareChartData(7)
	if err != nil {
		return "", err
	}

	monthlyLabels, monthlyKeystrokes, monthlyWords, monthlyMouseFeet, monthlyTotalKeys, monthlyTotalWords, monthlyTotalMouse, monthlyHeatmap, err := prepareChartData(30)
	if err != nil {
		return "", err
	}

	formatMouseData := func(feetData []float64, divisor float64) string {
		var result []string
		for _, f := range feetData {
			result = append(result, fmt.Sprintf("%.2f", f/divisor))
		}
		return strings.Join(result, ",")
	}

	weeklyMouseFeetStr := formatMouseData(weeklyMouseFeet, 1.0)
	weeklyMouseCarsStr := formatMouseData(weeklyMouseFeet, 15.0)
	weeklyMouseFieldsStr := formatMouseData(weeklyMouseFeet, 330.0)
	monthlyMouseFeetStr := formatMouseData(monthlyMouseFeet, 1.0)
	monthlyMouseCarsStr := formatMouseData(monthlyMouseFeet, 15.0)
	monthlyMouseFieldsStr := formatMouseData(monthlyMouseFeet, 330.0)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Typtel - Typing Statistics</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%%, #16213e 100%%);
            color: #eee;
            min-height: 100vh;
            padding: 30px;
        }
        h1 {
            text-align: center;
            margin-bottom: 10px;
            font-size: 2.5em;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .controls {
            display: flex;
            justify-content: center;
            gap: 20px;
            margin-bottom: 30px;
        }
        .control-group {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .control-group label {
            color: #888;
            font-size: 0.9em;
        }
        select {
            background: rgba(255,255,255,0.1);
            border: 1px solid rgba(255,255,255,0.2);
            border-radius: 8px;
            color: #eee;
            padding: 8px 16px;
            font-size: 0.9em;
            cursor: pointer;
        }
        select:hover {
            background: rgba(255,255,255,0.15);
        }
        .charts-container {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 30px;
            max-width: 1400px;
            margin: 0 auto 40px;
        }
        .chart-box {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 25px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        .chart-box h2 {
            margin-bottom: 20px;
            font-size: 1.3em;
            color: #aaa;
        }
        .heatmap-container {
            max-width: 1400px;
            margin: 0 auto;
        }
        .heatmap-box {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 25px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        .heatmap-box h2 {
            margin-bottom: 20px;
            font-size: 1.3em;
            color: #aaa;
        }
        .heatmap {
            display: flex;
            flex-direction: column;
            gap: 3px;
        }
        .heatmap-row {
            display: flex;
            align-items: center;
            gap: 3px;
        }
        .heatmap-label {
            width: 70px;
            font-size: 11px;
            color: #888;
            text-align: right;
            padding-right: 10px;
        }
        .heatmap-cell {
            width: 20px;
            height: 20px;
            border-radius: 3px;
            transition: transform 0.2s;
        }
        .heatmap-cell:hover {
            transform: scale(1.3);
            z-index: 10;
        }
        .hour-labels {
            display: flex;
            gap: 3px;
            margin-left: 80px;
            margin-bottom: 5px;
        }
        .hour-label {
            width: 20px;
            font-size: 10px;
            color: #666;
            text-align: center;
        }
        .legend {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 8px;
            margin-top: 20px;
        }
        .legend-text { color: #666; font-size: 12px; }
        .legend-box {
            width: 15px;
            height: 15px;
            border-radius: 2px;
        }
        .stats-summary {
            display: flex;
            justify-content: center;
            gap: 40px;
            margin: 30px 0;
        }
        .stat-item {
            text-align: center;
        }
        .stat-value {
            font-size: 2.5em;
            font-weight: bold;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .stat-label {
            color: #888;
            font-size: 0.9em;
        }
    </style>
</head>
<body>
    <h1>Typtel Statistics</h1>

    <div class="controls">
        <div class="control-group">
            <label>Time Period:</label>
            <select id="periodSelect" onchange="updateCharts()">
                <option value="weekly">Weekly (7 days)</option>
                <option value="monthly">Monthly (30 days)</option>
            </select>
        </div>
        <div class="control-group">
            <label>Distance Unit:</label>
            <select id="unitSelect" onchange="updateCharts()">
                <option value="feet">Feet</option>
                <option value="cars">Car Lengths (~15ft)</option>
                <option value="fields">Frisbee Fields (~330ft)</option>
            </select>
        </div>
    </div>

    <div class="stats-summary">
        <div class="stat-item">
            <div class="stat-value" id="totalKeystrokes">-</div>
            <div class="stat-label">Total Keystrokes</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalWords">-</div>
            <div class="stat-label">Words</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="avgKeystrokes">-</div>
            <div class="stat-label">Avg Keystrokes/Day</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalMouse">-</div>
            <div class="stat-label">Mouse Distance</div>
        </div>
    </div>

    <div class="charts-container">
        <div class="chart-box">
            <h2>Keystrokes per Day</h2>
            <canvas id="keystrokesChart"></canvas>
        </div>
        <div class="chart-box">
            <h2>Words per Day</h2>
            <canvas id="wordsChart"></canvas>
        </div>
    </div>

    <div class="charts-container">
        <div class="chart-box" style="grid-column: span 2;">
            <h2 id="mouseChartTitle">Mouse Distance per Day</h2>
            <canvas id="mouseChart"></canvas>
        </div>
    </div>

    <div class="heatmap-container">
        <div class="heatmap-box">
            <h2>Activity Heatmap (Hourly)</h2>
            <div class="hour-labels">
                %s
            </div>
            <div class="heatmap" id="heatmapContainer">
            </div>
            <div class="legend">
                <span class="legend-text">Less</span>
                <div class="legend-box" style="background: #1a1a2e;"></div>
                <div class="legend-box" style="background: #2d4a3e;"></div>
                <div class="legend-box" style="background: #3d6b4f;"></div>
                <div class="legend-box" style="background: #5a9a6f;"></div>
                <div class="legend-box" style="background: #7bc96f;"></div>
                <span class="legend-text">More</span>
            </div>
        </div>
    </div>

    <script>
        const data = {
            weekly: {
                labels: [%s],
                keystrokes: [%s],
                words: [%s],
                mouse: { feet: [%s], cars: [%s], fields: [%s] },
                totalKeystrokes: %d,
                totalWords: %d,
                totalMouseFeet: %.2f,
                days: 7,
                heatmap: `+"`%s`"+`
            },
            monthly: {
                labels: [%s],
                keystrokes: [%s],
                words: [%s],
                mouse: { feet: [%s], cars: [%s], fields: [%s] },
                totalKeystrokes: %d,
                totalWords: %d,
                totalMouseFeet: %.2f,
                days: 30,
                heatmap: `+"`%s`"+`
            }
        };

        const unitLabels = { feet: 'feet', cars: 'car lengths', fields: 'frisbee fields' };

        let keystrokesChart, wordsChart, mouseChart;

        const chartConfig = {
            responsive: true,
            plugins: { legend: { display: false } },
            scales: {
                y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.1)' }, ticks: { color: '#888' } },
                x: { grid: { display: false }, ticks: { color: '#888' } }
            }
        };

        function formatNumber(n) {
            if (n >= 1000000) return (n/1000000).toFixed(1) + 'M';
            if (n >= 1000) return (n/1000).toFixed(1) + 'K';
            return n.toString();
        }

        function formatDistanceJS(feet) {
            if (feet >= 5280) return (feet/5280).toFixed(2) + ' mi';
            return feet.toFixed(0) + ' ft';
        }

        function updateCharts() {
            const period = document.getElementById('periodSelect').value;
            const unit = document.getElementById('unitSelect').value;
            const d = data[period];

            document.getElementById('totalKeystrokes').textContent = formatNumber(d.totalKeystrokes);
            document.getElementById('totalWords').textContent = formatNumber(d.totalWords);
            document.getElementById('avgKeystrokes').textContent = formatNumber(Math.round(d.totalKeystrokes / d.days));
            document.getElementById('totalMouse').textContent = formatDistanceJS(d.totalMouseFeet);

            document.getElementById('mouseChartTitle').textContent = 'Mouse Distance per Day (' + unitLabels[unit] + ')';

            if (keystrokesChart) keystrokesChart.destroy();
            if (wordsChart) wordsChart.destroy();
            if (mouseChart) mouseChart.destroy();

            keystrokesChart = new Chart(document.getElementById('keystrokesChart'), {
                type: 'bar',
                data: {
                    labels: d.labels,
                    datasets: [{ data: d.keystrokes, backgroundColor: 'rgba(0, 210, 255, 0.6)', borderColor: 'rgba(0, 210, 255, 1)', borderWidth: 1, borderRadius: 4 }]
                },
                options: chartConfig
            });

            wordsChart = new Chart(document.getElementById('wordsChart'), {
                type: 'line',
                data: {
                    labels: d.labels,
                    datasets: [{ data: d.words, borderColor: 'rgba(122, 201, 111, 1)', backgroundColor: 'rgba(122, 201, 111, 0.2)', fill: true, tension: 0.4, pointRadius: 4, pointBackgroundColor: 'rgba(122, 201, 111, 1)' }]
                },
                options: chartConfig
            });

            mouseChart = new Chart(document.getElementById('mouseChart'), {
                type: 'bar',
                data: {
                    labels: d.labels,
                    datasets: [{ data: d.mouse[unit], backgroundColor: 'rgba(255, 107, 107, 0.6)', borderColor: 'rgba(255, 107, 107, 1)', borderWidth: 1, borderRadius: 4 }]
                },
                options: chartConfig
            });

            document.getElementById('heatmapContainer').innerHTML = d.heatmap;
        }

        updateCharts();
    </script>
</body>
</html>`,
		generateHourLabels(),
		strings.Join(weeklyLabels, ","),
		strings.Join(weeklyKeystrokes, ","),
		strings.Join(weeklyWords, ","),
		weeklyMouseFeetStr,
		weeklyMouseCarsStr,
		weeklyMouseFieldsStr,
		weeklyTotalKeys,
		weeklyTotalWords,
		pixelsToFeet(weeklyTotalMouse),
		weeklyHeatmap,
		strings.Join(monthlyLabels, ","),
		strings.Join(monthlyKeystrokes, ","),
		strings.Join(monthlyWords, ","),
		monthlyMouseFeetStr,
		monthlyMouseCarsStr,
		monthlyMouseFieldsStr,
		monthlyTotalKeys,
		monthlyTotalWords,
		pixelsToFeet(monthlyTotalMouse),
		monthlyHeatmap,
	)

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dataDir := filepath.Join(home, ".local", "share", "typtel", "logs")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", err
	}
	htmlPath := filepath.Join(dataDir, "charts.html")
	if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
		return "", err
	}

	return htmlPath, nil
}

func generateHourLabels() string {
	var labels []string
	for h := 0; h < 24; h++ {
		if h%3 == 0 {
			labels = append(labels, fmt.Sprintf(`<div class="hour-label">%d</div>`, h))
		} else {
			labels = append(labels, `<div class="hour-label"></div>`)
		}
	}
	return strings.Join(labels, "\n                ")
}

func generateHeatmapHTML(hourlyData map[string][]storage.HourlyStats) string {
	var maxVal int64 = 1
	for _, hours := range hourlyData {
		for _, h := range hours {
			if h.Keystrokes > maxVal {
				maxVal = h.Keystrokes
			}
		}
	}

	dates := make([]string, 0, len(hourlyData))
	for date := range hourlyData {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	var rows []string
	for _, date := range dates {
		hours := hourlyData[date]
		t, _ := time.Parse("2006-01-02", date)
		dateLabel := t.Format("Mon Jan 2")

		var cells []string
		for _, h := range hours {
			color := getHeatmapColor(h.Keystrokes, maxVal)
			title := fmt.Sprintf("%s %d:00 - %d keystrokes", dateLabel, h.Hour, h.Keystrokes)
			cells = append(cells, fmt.Sprintf(
				`<div class="heatmap-cell" style="background: %s;" title="%s"></div>`,
				color, title,
			))
		}

		rows = append(rows, fmt.Sprintf(
			`<div class="heatmap-row"><div class="heatmap-label">%s</div>%s</div>`,
			dateLabel,
			strings.Join(cells, ""),
		))
	}

	return strings.Join(rows, "\n                ")
}

func getHeatmapColor(value, max int64) string {
	if value == 0 {
		return "#1a1a2e"
	}
	ratio := float64(value) / float64(max)
	if ratio < 0.25 {
		return "#2d4a3e"
	} else if ratio < 0.5 {
		return "#3d6b4f"
	} else if ratio < 0.75 {
		return "#5a9a6f"
	}
	return "#7bc96f"
}

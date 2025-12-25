package tui

import (
	"fmt"
	"strings"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	statLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	statValueStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2)

	graphStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)
)

type Model struct {
	store       *storage.Store
	todayStats  *storage.DailyStats
	weekStats   []storage.DailyStats
	hourlyStats []storage.HourlyStats
	width       int
	height      int
	err         error
}

type statsMsg struct {
	today  *storage.DailyStats
	week   []storage.DailyStats
	hourly []storage.HourlyStats
	err    error
}

func New(store *storage.Store) Model {
	return Model{store: store}
}

func (m Model) Init() tea.Cmd {
	return m.fetchStats
}

func (m Model) fetchStats() tea.Msg {
	today, err := m.store.GetTodayStats()
	if err != nil {
		return statsMsg{err: err}
	}

	week, err := m.store.GetWeekStats()
	if err != nil {
		return statsMsg{err: err}
	}

	hourly, err := m.store.GetHourlyStats(today.Date)
	if err != nil {
		return statsMsg{err: err}
	}

	return statsMsg{today: today, week: week, hourly: hourly}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			return m, m.fetchStats
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case statsMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.todayStats = msg.today
			m.weekStats = msg.week
			m.hourlyStats = msg.hourly
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err)
	}

	if m.todayStats == nil {
		return "Loading..."
	}

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("⌨️  Typing Telemetry"))
	b.WriteString("\n\n")

	// Today's stats box
	todayContent := fmt.Sprintf(
		"%s %s\n%s %s",
		statLabelStyle.Render("Keystrokes:"),
		statValueStyle.Render(formatNumber(m.todayStats.Keystrokes)),
		statLabelStyle.Render("Words:"),
		statValueStyle.Render(formatNumber(m.todayStats.Words)),
	)
	b.WriteString(boxStyle.Render("Today\n" + todayContent))
	b.WriteString("\n\n")

	// Weekly summary
	var weekTotal int64
	for _, day := range m.weekStats {
		weekTotal += day.Keystrokes
	}
	weekContent := fmt.Sprintf(
		"%s %s\n%s %s",
		statLabelStyle.Render("Total:"),
		statValueStyle.Render(formatNumber(weekTotal)),
		statLabelStyle.Render("Daily Avg:"),
		statValueStyle.Render(formatNumber(weekTotal/7)),
	)
	b.WriteString(boxStyle.Render("This Week\n" + weekContent))
	b.WriteString("\n\n")

	// Hourly graph
	b.WriteString(statLabelStyle.Render("Today's Activity:"))
	b.WriteString("\n")
	b.WriteString(m.renderHourlyGraph())
	b.WriteString("\n")

	// Weekly graph
	b.WriteString(statLabelStyle.Render("Weekly Activity:"))
	b.WriteString("\n")
	b.WriteString(m.renderWeeklyGraph())
	b.WriteString("\n")

	// Help
	b.WriteString(helpStyle.Render("r: refresh • q: quit"))

	return b.String()
}

func (m Model) renderHourlyGraph() string {
	if len(m.hourlyStats) == 0 {
		return "No data"
	}

	var maxCount int64
	for _, h := range m.hourlyStats {
		if h.Keystrokes > maxCount {
			maxCount = h.Keystrokes
		}
	}

	if maxCount == 0 {
		return "No activity today"
	}

	bars := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	var graph strings.Builder

	for _, h := range m.hourlyStats {
		if maxCount > 0 {
			idx := int(float64(h.Keystrokes) / float64(maxCount) * float64(len(bars)-1))
			if h.Keystrokes > 0 && idx == 0 {
				idx = 1
			}
			graph.WriteString(graphStyle.Render(bars[idx]))
		} else {
			graph.WriteString(bars[0])
		}
	}

	// Hour labels
	graph.WriteString("\n")
	graph.WriteString(statLabelStyle.Render("0     6     12    18  23"))

	return graph.String()
}

func (m Model) renderWeeklyGraph() string {
	if len(m.weekStats) == 0 {
		return "No data"
	}

	var maxCount int64
	for _, d := range m.weekStats {
		if d.Keystrokes > maxCount {
			maxCount = d.Keystrokes
		}
	}

	if maxCount == 0 {
		return "No activity this week"
	}

	bars := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	var graph strings.Builder

	for _, d := range m.weekStats {
		if maxCount > 0 {
			idx := int(float64(d.Keystrokes) / float64(maxCount) * float64(len(bars)-1))
			if d.Keystrokes > 0 && idx == 0 {
				idx = 1
			}
			graph.WriteString(graphStyle.Render(bars[idx]))
			graph.WriteString(" ")
		}
	}

	return graph.String()
}

func formatNumber(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

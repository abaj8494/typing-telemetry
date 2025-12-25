// +build darwin

package menubar

import (
	"fmt"
	"os"
	"time"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/caseymrm/menuet"
)

type App struct {
	store *storage.Store
}

func New(store *storage.Store) *App {
	return &App{store: store}
}

func (a *App) Run() {
	go a.updateLoop()

	menuet.App().Label = "com.typtel.menubar"
	menuet.App().Children = a.menuItems

	menuet.App().RunApplication()
}

func (a *App) updateLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		a.updateTitle()
		<-ticker.C
	}
}

func (a *App) updateTitle() {
	stats, err := a.store.GetTodayStats()
	if err != nil {
		menuet.App().SetMenuState(&menuet.MenuState{
			Title: "⌨️ --",
		})
		return
	}

	title := fmt.Sprintf("⌨️ %s", formatCompact(stats.Keystrokes))
	menuet.App().SetMenuState(&menuet.MenuState{
		Title: title,
	})
}

func (a *App) menuItems() []menuet.MenuItem {
	stats, _ := a.store.GetTodayStats()
	weekStats, _ := a.store.GetWeekStats()

	var weekTotal int64
	for _, day := range weekStats {
		weekTotal += day.Keystrokes
	}

	items := []menuet.MenuItem{
		{
			Text: fmt.Sprintf("Today: %s keystrokes", formatNumber(stats.Keystrokes)),
		},
		{
			Text: fmt.Sprintf("This Week: %s keystrokes", formatNumber(weekTotal)),
		},
		{
			Type: menuet.Separator,
		},
		{
			Text:     "Open Dashboard",
			Clicked:  a.openDashboard,
		},
		{
			Type: menuet.Separator,
		},
		{
			Text:    "Quit",
			Clicked: a.quit,
		},
	}

	return items
}

func (a *App) openDashboard() {
	// Launch the TUI in a new terminal window
	// This is handled by the main app
}

func (a *App) quit() {
	os.Exit(0)
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

func formatCompact(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.0fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

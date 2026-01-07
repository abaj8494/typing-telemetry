package tui

import "github.com/charmbracelet/lipgloss"

// Theme defines a color scheme for the typing test
type Theme struct {
	Name             string
	PrimaryAccent    string // Titles, cursor background
	SecondaryAccent  string // Options box border, pace caret
	CorrectText      string // Correctly typed text, selected options, values
	ErrorText        string // Incorrectly typed text
	LabelText        string // Labels, prompts, help text
	RemainingText    string // Untyped text
	Border           string // Stats box border
	SelectedBg       string // Selected option background
}

// Available themes
var Themes = map[string]Theme{
	"default": {
		Name:             "Default",
		PrimaryAccent:    "#C73B3C", // New burgundy red accent
		SecondaryAccent:  "#C73B3C",
		CorrectText:      "#5fafaf", // Teal/cyan
		ErrorText:        "#ff5f5f", // Bright red
		LabelText:        "#6c6c6c", // Gray
		RemainingText:    "#8a8a8a", // Light gray
		Border:           "#5f87d7", // Blue
		SelectedBg:       "#303030", // Dark gray
	},
	"gruvbox": {
		Name:             "Gruvbox",
		PrimaryAccent:    "#d65d0e", // Gruvbox orange
		SecondaryAccent:  "#b16286", // Gruvbox purple
		CorrectText:      "#98971a", // Gruvbox green
		ErrorText:        "#cc241d", // Gruvbox red
		LabelText:        "#928374", // Gruvbox gray
		RemainingText:    "#a89984", // Gruvbox light gray
		Border:           "#458588", // Gruvbox aqua
		SelectedBg:       "#3c3836", // Gruvbox bg1
	},
	"tokyonight": {
		Name:             "Tokyo Night",
		PrimaryAccent:    "#7aa2f7", // Tokyo Night blue
		SecondaryAccent:  "#bb9af7", // Tokyo Night purple
		CorrectText:      "#9ece6a", // Tokyo Night green
		ErrorText:        "#f7768e", // Tokyo Night red
		LabelText:        "#565f89", // Tokyo Night comment
		RemainingText:    "#9aa5ce", // Tokyo Night foreground dim
		Border:           "#7dcfff", // Tokyo Night cyan
		SelectedBg:       "#292e42", // Tokyo Night bg highlight
	},
	"catppuccin": {
		Name:             "Catppuccin",
		PrimaryAccent:    "#cba6f7", // Catppuccin Mauve
		SecondaryAccent:  "#f5c2e7", // Catppuccin Pink
		CorrectText:      "#a6e3a1", // Catppuccin Green
		ErrorText:        "#f38ba8", // Catppuccin Red
		LabelText:        "#6c7086", // Catppuccin Overlay0
		RemainingText:    "#9399b2", // Catppuccin Overlay2
		Border:           "#89b4fa", // Catppuccin Blue
		SelectedBg:       "#313244", // Catppuccin Surface0
	},
}

// ThemeNames returns the list of available theme names
var ThemeNames = []string{"default", "gruvbox", "tokyonight", "catppuccin"}

// CurrentTheme holds the active theme
var CurrentTheme = Themes["default"]

// SetTheme updates the current theme and regenerates all styles
func SetTheme(name string) {
	if theme, ok := Themes[name]; ok {
		CurrentTheme = theme
		regenerateStyles()
	}
}

// regenerateStyles updates all lipgloss styles with current theme colors
func regenerateStyles() {
	// Update typing test styles
	promptStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(CurrentTheme.LabelText))

	correctStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(CurrentTheme.CorrectText))

	incorrectStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(CurrentTheme.ErrorText)).
		Underline(true)

	cursorStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(CurrentTheme.PrimaryAccent)).
		Foreground(lipgloss.Color("#000000"))

	remainingStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(CurrentTheme.RemainingText))

	statsBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(CurrentTheme.CorrectText)).
		Padding(1, 3).
		MarginTop(2)

	resultTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(CurrentTheme.PrimaryAccent)).
		MarginBottom(1)

	resultValueStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(CurrentTheme.CorrectText))

	resultLabelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(CurrentTheme.LabelText))

	optionsTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(CurrentTheme.PrimaryAccent)).
		MarginBottom(1)

	optionsBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(CurrentTheme.SecondaryAccent)).
		Padding(1, 2)

	selectedOptionStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(CurrentTheme.CorrectText)).
		Background(lipgloss.Color(CurrentTheme.SelectedBg))

	unselectedOptionStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(CurrentTheme.RemainingText))

	searchBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(CurrentTheme.PrimaryAccent)).
		Padding(0, 1).
		MarginBottom(1)

	valueStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(CurrentTheme.SecondaryAccent))

	paceCaretStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(CurrentTheme.SecondaryAccent)).
		Foreground(lipgloss.Color("#000000"))

	// Update main TUI styles
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(CurrentTheme.PrimaryAccent)).
		MarginBottom(1)

	statLabelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(CurrentTheme.LabelText))

	statValueStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(CurrentTheme.CorrectText))

	boxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(CurrentTheme.Border)).
		Padding(1, 2)

	graphStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(CurrentTheme.PrimaryAccent))

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(CurrentTheme.LabelText)).
		MarginTop(1)
}

// Initialize styles with default theme
func init() {
	regenerateStyles()
}

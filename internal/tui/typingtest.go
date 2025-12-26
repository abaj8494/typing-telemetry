package tui

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Typing test styles
	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	correctStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86"))

	incorrectStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Underline(true)

	cursorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("205")).
			Foreground(lipgloss.Color("0"))

	remainingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	statsBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("86")).
			Padding(1, 3).
			MarginTop(2)

	resultTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				MarginBottom(1)

	resultValueStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("86"))

	resultLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))
)

// Default word lists for typing tests
var defaultWords = []string{
	"the", "be", "to", "of", "and", "a", "in", "that", "have", "I",
	"it", "for", "not", "on", "with", "he", "as", "you", "do", "at",
	"this", "but", "his", "by", "from", "they", "we", "say", "her", "she",
	"or", "an", "will", "my", "one", "all", "would", "there", "their", "what",
	"so", "up", "out", "if", "about", "who", "get", "which", "go", "me",
	"when", "make", "can", "like", "time", "no", "just", "him", "know", "take",
	"people", "into", "year", "your", "good", "some", "could", "them", "see", "other",
	"than", "then", "now", "look", "only", "come", "its", "over", "think", "also",
	"back", "after", "use", "two", "how", "our", "work", "first", "well", "way",
	"even", "new", "want", "because", "any", "these", "give", "day", "most", "us",
	"code", "program", "function", "variable", "string", "number", "array", "object", "class", "method",
	"return", "import", "export", "const", "let", "var", "async", "await", "promise", "callback",
}

type TestState int

const (
	StateReady TestState = iota
	StateRunning
	StateFinished
)

type TypingTestModel struct {
	targetText  string
	typed       string
	startTime   time.Time
	endTime     time.Time
	state       TestState
	width       int
	height      int
	sourceFile  string
	wordCount   int
	errors      int
}

type tickMsg time.Time

func NewTypingTest(sourceFile string, wordCount int) TypingTestModel {
	text := generateText(sourceFile, wordCount)
	return TypingTestModel{
		targetText: text,
		state:      StateReady,
		sourceFile: sourceFile,
		wordCount:  wordCount,
	}
}

func generateText(sourceFile string, wordCount int) string {
	var words []string

	if sourceFile != "" {
		// Load from file
		file, err := os.Open(sourceFile)
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line != "" {
					lineWords := strings.Fields(line)
					words = append(words, lineWords...)
				}
			}
		}
	}

	// Fall back to default words if file is empty or not found
	if len(words) == 0 {
		words = defaultWords
	}

	// Shuffle and select words
	rand.Shuffle(len(words), func(i, j int) {
		words[i], words[j] = words[j], words[i]
	})

	if wordCount <= 0 {
		wordCount = 25
	}

	// Build the text
	var result []string
	for i := 0; i < wordCount; i++ {
		result = append(result, words[i%len(words)])
	}

	return strings.Join(result, " ")
}

func (m TypingTestModel) Init() tea.Cmd {
	return nil
}

func (m TypingTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyBackspace:
			if len(m.typed) > 0 && m.state == StateRunning {
				m.typed = m.typed[:len(m.typed)-1]
			}
			return m, nil

		case tea.KeyEnter:
			if m.state == StateFinished {
				// Restart with new text
				m.targetText = generateText(m.sourceFile, m.wordCount)
				m.typed = ""
				m.state = StateReady
				m.errors = 0
			}
			return m, nil

		case tea.KeyRunes, tea.KeySpace:
			char := string(msg.Runes)
			if msg.Type == tea.KeySpace {
				char = " "
			}

			if m.state == StateReady {
				m.state = StateRunning
				m.startTime = time.Now()
			}

			if m.state == StateRunning {
				m.typed += char

				// Check if character is wrong
				if len(m.typed) <= len(m.targetText) {
					if m.typed[len(m.typed)-1] != m.targetText[len(m.typed)-1] {
						m.errors++
					}
				}

				// Check if finished
				if len(m.typed) >= len(m.targetText) {
					m.state = StateFinished
					m.endTime = time.Now()
				}
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m TypingTestModel) View() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("⌨️  Typing Test"))
	b.WriteString("\n\n")

	if m.state == StateReady {
		b.WriteString(promptStyle.Render("Start typing to begin..."))
		b.WriteString("\n\n")
	}

	// Render the text with highlighting
	b.WriteString(m.renderText())
	b.WriteString("\n")

	// Stats during typing
	if m.state == StateRunning {
		elapsed := time.Since(m.startTime).Seconds()
		wordsTyped := float64(len(m.typed)) / 5.0 // Standard: 5 chars = 1 word
		wpm := 0.0
		if elapsed > 0 {
			wpm = (wordsTyped / elapsed) * 60
		}

		accuracy := 100.0
		if len(m.typed) > 0 {
			accuracy = float64(len(m.typed)-m.errors) / float64(len(m.typed)) * 100
		}

		progress := float64(len(m.typed)) / float64(len(m.targetText)) * 100

		stats := fmt.Sprintf(
			"%s %.0f  %s %.0f%%  %s %.0f%%",
			resultLabelStyle.Render("WPM:"),
			wpm,
			resultLabelStyle.Render("Accuracy:"),
			accuracy,
			resultLabelStyle.Render("Progress:"),
			progress,
		)
		b.WriteString("\n")
		b.WriteString(stats)
	}

	// Final results
	if m.state == StateFinished {
		b.WriteString(m.renderResults())
	}

	// Help
	b.WriteString("\n\n")
	if m.state == StateFinished {
		b.WriteString(helpStyle.Render("enter: new test • esc: quit"))
	} else {
		b.WriteString(helpStyle.Render("esc: quit"))
	}

	return b.String()
}

func (m TypingTestModel) renderText() string {
	var b strings.Builder

	maxWidth := m.width - 4
	if maxWidth <= 0 {
		maxWidth = 80
	}

	// Wrap text to fit width
	target := m.targetText
	typed := m.typed

	lineLen := 0
	for i, char := range target {
		// Add newline if needed
		if lineLen >= maxWidth && char == ' ' {
			b.WriteString("\n")
			lineLen = 0
			continue
		}

		if i < len(typed) {
			if typed[i] == byte(char) {
				b.WriteString(correctStyle.Render(string(char)))
			} else {
				b.WriteString(incorrectStyle.Render(string(char)))
			}
		} else if i == len(typed) {
			b.WriteString(cursorStyle.Render(string(char)))
		} else {
			b.WriteString(remainingStyle.Render(string(char)))
		}
		lineLen++
	}

	return b.String()
}

func (m TypingTestModel) renderResults() string {
	duration := m.endTime.Sub(m.startTime).Seconds()
	wordsTyped := float64(len(m.targetText)) / 5.0
	wpm := (wordsTyped / duration) * 60

	correctChars := len(m.targetText) - m.errors
	accuracy := float64(correctChars) / float64(len(m.targetText)) * 100

	results := fmt.Sprintf(
		"%s\n\n%s %s\n%s %s\n%s %s\n%s %s",
		resultTitleStyle.Render("Test Complete!"),
		resultLabelStyle.Render("WPM:"),
		resultValueStyle.Render(fmt.Sprintf("%.1f", wpm)),
		resultLabelStyle.Render("Accuracy:"),
		resultValueStyle.Render(fmt.Sprintf("%.1f%%", accuracy)),
		resultLabelStyle.Render("Time:"),
		resultValueStyle.Render(fmt.Sprintf("%.1fs", duration)),
		resultLabelStyle.Render("Characters:"),
		resultValueStyle.Render(fmt.Sprintf("%d", len(m.targetText))),
	)

	return statsBoxStyle.Render(results)
}

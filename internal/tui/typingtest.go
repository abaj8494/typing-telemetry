package tui

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
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

	// Options menu styles
	optionsTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				MarginBottom(1)

	optionsBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(1, 2)

	selectedOptionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("86")).
				Background(lipgloss.Color("236"))

	unselectedOptionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	searchBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205")).
			Padding(0, 1).
			MarginBottom(1)

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99"))

	paceCaretStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("99")).
			Foreground(lipgloss.Color("0"))
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

// Punctuation characters to add
var punctuationMarks = []string{".", ",", "!", "?", ";", ":", "'", "\"", "-", "(", ")"}

// Layout mappings (QWERTY to other layouts)
var layoutMappings = map[string]map[rune]rune{
	"qwerty": {}, // Identity mapping
	"dvorak": {
		'q': '\'', 'w': ',', 'e': '.', 'r': 'p', 't': 'y', 'y': 'f', 'u': 'g', 'i': 'c', 'o': 'r', 'p': 'l',
		'a': 'a', 's': 'o', 'd': 'e', 'f': 'u', 'g': 'i', 'h': 'd', 'j': 'h', 'k': 't', 'l': 'n', ';': 's',
		'z': ';', 'x': 'q', 'c': 'j', 'v': 'k', 'b': 'x', 'n': 'b', 'm': 'm', ',': 'w', '.': 'v', '/': 'z',
	},
	"colemak": {
		'e': 'f', 'r': 'p', 't': 'g', 'y': 'j', 'u': 'l', 'i': 'u', 'o': 'y', 'p': ';',
		's': 'r', 'd': 's', 'f': 't', 'g': 'd', 'j': 'n', 'k': 'e', 'l': 'i', ';': 'o',
		'n': 'k',
	},
}

type TestState int

const (
	StateReady TestState = iota
	StateRunning
	StateFinished
	StateOptions
)

// PaceCaretMode determines pace caret behavior
type PaceCaretMode int

const (
	PaceOff PaceCaretMode = iota
	PacePB
	PaceAverage
	PaceCustom
)

// TestOptions holds all configurable options
type TestOptions struct {
	Layout        string        // "qwerty", "dvorak", "colemak"
	LiveWPM       bool          // Show live WPM while typing
	WordCount     int           // Number of words in test
	Punctuation   bool          // Include sentence-style punctuation and capitalization
	PaceCaret     PaceCaretMode // Pace caret mode
	CustomPaceWPM float64       // Custom pace WPM target
}

// Option represents a single option in the menu
type Option struct {
	ID          string
	Name        string
	Description string
	Type        string // "choice", "toggle", "number", "submenu"
	Choices     []string
	Value       interface{}
}

type TypingTestModel struct {
	targetText    string
	typed         string
	startTime     time.Time
	endTime       time.Time
	state         TestState
	width         int
	height        int
	sourceFile    string
	wordCount     int
	errors        int
	options       TestOptions
	allOptions    []Option
	filteredOpts  []Option
	selectedIdx   int
	searchQuery   string
	inSubMenu     bool
	subMenuIdx    int
	personalBest  float64 // Personal best WPM
	avgWPM        float64 // Average WPM from past tests
	inCustomWPMInput bool   // Whether we're inputting custom WPM
	customWPMInput   string // Buffer for custom WPM input
}

type tickMsg time.Time

func NewTypingTest(sourceFile string, wordCount int) TypingTestModel {
	if wordCount <= 0 {
		wordCount = 25
	}

	options := TestOptions{
		Layout:        "qwerty",
		LiveWPM:       true,
		WordCount:     wordCount,
		Punctuation:   false,
		PaceCaret:     PaceOff,
		CustomPaceWPM: 60.0,
	}

	allOptions := []Option{
		{
			ID:          "layout",
			Name:        "Layout",
			Description: "Keyboard layout to emulate",
			Type:        "choice",
			Choices:     []string{"qwerty", "dvorak", "colemak"},
			Value:       "qwerty",
		},
		{
			ID:          "live_wpm",
			Name:        "Live WPM",
			Description: "Show WPM while typing",
			Type:        "toggle",
			Value:       true,
		},
		{
			ID:          "test_length",
			Name:        "Test Length",
			Description: "Number of words",
			Type:        "choice",
			Choices:     []string{"10", "25", "50", "100", "200"},
			Value:       "25",
		},
		{
			ID:          "punctuation",
			Name:        "Punctuation",
			Description: "Sentence-style capitalization and punctuation",
			Type:        "toggle",
			Value:       false,
		},
		{
			ID:          "pace_caret",
			Name:        "Pace Caret",
			Description: "Ghost cursor to pace against",
			Type:        "submenu",
			Choices:     []string{"off", "pb", "average", "custom"},
			Value:       "off",
		},
	}

	m := TypingTestModel{
		state:        StateReady,
		sourceFile:   sourceFile,
		wordCount:    wordCount,
		options:      options,
		allOptions:   allOptions,
		filteredOpts: allOptions,
		personalBest: 0,
		avgWPM:       50.0, // Default average
	}

	m.targetText = m.generateText()
	return m
}

func (m *TypingTestModel) generateText() string {
	var words []string

	if m.sourceFile != "" {
		// Load from file
		file, err := os.Open(m.sourceFile)
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

	wordCount := m.options.WordCount
	if wordCount <= 0 {
		wordCount = 25
	}

	// Build the text
	var result []string
	startOfSentence := true
	wordsInSentence := 0

	for i := 0; i < wordCount; i++ {
		word := words[i%len(words)]

		if m.options.Punctuation {
			// Capitalize first letter at start of sentence
			if startOfSentence && len(word) > 0 {
				word = strings.ToUpper(string(word[0])) + word[1:]
				startOfSentence = false
			}

			wordsInSentence++

			// Add punctuation with grammatically sensible patterns
			// Sentences should be 4-12 words, with occasional commas
			if wordsInSentence >= 3 && i < wordCount-1 {
				// Add comma occasionally mid-sentence (after 3+ words)
				if wordsInSentence < 8 && rand.Float32() < 0.15 {
					word = word + ","
				} else if wordsInSentence >= 4 && rand.Float32() < 0.25 {
					// End sentence with period, question mark, or exclamation
					r := rand.Float32()
					if r < 0.7 {
						word = word + "."
					} else if r < 0.85 {
						word = word + "?"
					} else {
						word = word + "!"
					}
					startOfSentence = true
					wordsInSentence = 0
				}
			}

			// Force sentence end if too long
			if wordsInSentence >= 10 && i < wordCount-1 {
				if !strings.HasSuffix(word, ".") && !strings.HasSuffix(word, "?") && !strings.HasSuffix(word, "!") && !strings.HasSuffix(word, ",") {
					word = word + "."
					startOfSentence = true
					wordsInSentence = 0
				}
			}
		}

		result = append(result, word)
	}

	// End with a period if punctuation is enabled and doesn't already end with one
	if m.options.Punctuation && len(result) > 0 {
		lastWord := result[len(result)-1]
		if !strings.HasSuffix(lastWord, ".") && !strings.HasSuffix(lastWord, "?") && !strings.HasSuffix(lastWord, "!") {
			result[len(result)-1] = lastWord + "."
		}
	}

	text := strings.Join(result, " ")

	// Apply layout transformation if not qwerty
	if m.options.Layout != "qwerty" {
		text = m.transformLayout(text)
	}

	return text
}

func (m *TypingTestModel) transformLayout(text string) string {
	mapping := layoutMappings[m.options.Layout]
	if len(mapping) == 0 {
		return text
	}

	var result strings.Builder
	for _, char := range text {
		if mapped, ok := mapping[char]; ok {
			result.WriteRune(mapped)
		} else {
			result.WriteRune(char)
		}
	}
	return result.String()
}

func (m *TypingTestModel) resetTest() {
	m.targetText = m.generateText()
	m.typed = ""
	m.state = StateReady
	m.errors = 0
}

func (m *TypingTestModel) filterOptions() {
	if m.searchQuery == "" {
		m.filteredOpts = m.allOptions
		return
	}

	// Use fuzzy matching
	var optionNames []string
	for _, opt := range m.allOptions {
		optionNames = append(optionNames, opt.Name+" "+opt.Description)
	}

	matches := fuzzy.Find(m.searchQuery, optionNames)
	m.filteredOpts = make([]Option, 0)
	for _, match := range matches {
		m.filteredOpts = append(m.filteredOpts, m.allOptions[match.Index])
	}

	if m.selectedIdx >= len(m.filteredOpts) {
		m.selectedIdx = 0
	}
}

func (m *TypingTestModel) applyOption(opt Option, choiceIdx int) {
	switch opt.ID {
	case "layout":
		m.options.Layout = opt.Choices[choiceIdx]
		m.allOptions[0].Value = opt.Choices[choiceIdx]
	case "live_wpm":
		m.options.LiveWPM = !m.options.LiveWPM
		m.allOptions[1].Value = m.options.LiveWPM
	case "test_length":
		count, _ := strconv.Atoi(opt.Choices[choiceIdx])
		m.options.WordCount = count
		m.wordCount = count
		m.allOptions[2].Value = opt.Choices[choiceIdx]
	case "punctuation":
		m.options.Punctuation = !m.options.Punctuation
		m.allOptions[3].Value = m.options.Punctuation
	case "pace_caret":
		switch opt.Choices[choiceIdx] {
		case "off":
			m.options.PaceCaret = PaceOff
		case "pb":
			m.options.PaceCaret = PacePB
		case "average":
			m.options.PaceCaret = PaceAverage
		case "custom":
			m.options.PaceCaret = PaceCustom
			// Enter custom WPM input mode
			m.inCustomWPMInput = true
			m.customWPMInput = fmt.Sprintf("%.0f", m.options.CustomPaceWPM)
		}
		m.allOptions[4].Value = opt.Choices[choiceIdx]
	}
}

func (m TypingTestModel) Init() tea.Cmd {
	return nil
}

func (m TypingTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle options menu state
		if m.state == StateOptions {
			return m.updateOptions(msg)
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyEsc:
			// Open options menu
			m.state = StateOptions
			m.searchQuery = ""
			m.selectedIdx = 0
			m.inSubMenu = false
			m.filterOptions()
			return m, nil

		case tea.KeyTab:
			// Reset test
			m.resetTest()
			return m, nil

		case tea.KeyBackspace:
			if len(m.typed) > 0 && m.state == StateRunning {
				m.typed = m.typed[:len(m.typed)-1]
			}
			return m, nil

		case tea.KeyEnter:
			if m.state == StateFinished {
				// Update personal best
				duration := m.endTime.Sub(m.startTime).Seconds()
				wordsTyped := float64(len(m.targetText)) / 5.0
				wpm := (wordsTyped / duration) * 60
				if wpm > m.personalBest {
					m.personalBest = wpm
				}
				// Update average (simple moving average)
				m.avgWPM = (m.avgWPM + wpm) / 2

				// Restart with new text
				m.resetTest()
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

func (m TypingTestModel) updateOptions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle custom WPM input mode
	if m.inCustomWPMInput {
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc, tea.KeyCtrlG:
			m.inCustomWPMInput = false
			return m, nil
		case tea.KeyEnter:
			if wpm, err := strconv.ParseFloat(m.customWPMInput, 64); err == nil && wpm > 0 {
				m.options.CustomPaceWPM = wpm
			}
			m.inCustomWPMInput = false
			m.inSubMenu = false
			return m, nil
		case tea.KeyBackspace:
			if len(m.customWPMInput) > 0 {
				m.customWPMInput = m.customWPMInput[:len(m.customWPMInput)-1]
			}
			return m, nil
		case tea.KeyRunes:
			// Only allow digits and decimal point
			for _, r := range msg.Runes {
				if (r >= '0' && r <= '9') || r == '.' {
					m.customWPMInput += string(r)
				}
			}
			return m, nil
		}
		return m, nil
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit

	case tea.KeyEsc, tea.KeyCtrlG:
		if m.inSubMenu {
			m.inSubMenu = false
			return m, nil
		}
		// Close options and regenerate text with new options
		m.state = StateReady
		m.resetTest()
		return m, nil

	case tea.KeyTab:
		// Also close options
		m.state = StateReady
		m.resetTest()
		return m, nil

	case tea.KeyUp, tea.KeyCtrlP:
		if m.inSubMenu {
			if m.subMenuIdx > 0 {
				m.subMenuIdx--
			}
		} else {
			if m.selectedIdx > 0 {
				m.selectedIdx--
			}
		}
		return m, nil

	case tea.KeyDown, tea.KeyCtrlN:
		if m.inSubMenu {
			opt := m.filteredOpts[m.selectedIdx]
			if m.subMenuIdx < len(opt.Choices)-1 {
				m.subMenuIdx++
			}
		} else {
			if m.selectedIdx < len(m.filteredOpts)-1 {
				m.selectedIdx++
			}
		}
		return m, nil

	case tea.KeyEnter:
		if len(m.filteredOpts) == 0 {
			return m, nil
		}
		opt := m.filteredOpts[m.selectedIdx]
		if opt.Type == "toggle" {
			m.applyOption(opt, 0)
		} else if opt.Type == "choice" || opt.Type == "submenu" {
			if m.inSubMenu {
				m.applyOption(opt, m.subMenuIdx)
				if !m.inCustomWPMInput {
					m.inSubMenu = false
				}
			} else {
				m.inSubMenu = true
				m.subMenuIdx = 0
				// Find current selection index
				for i, choice := range opt.Choices {
					if choice == opt.Value.(string) {
						m.subMenuIdx = i
						break
					}
				}
			}
		}
		return m, nil

	case tea.KeyBackspace:
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.filterOptions()
		}
		return m, nil

	case tea.KeyRunes:
		m.searchQuery += string(msg.Runes)
		m.filterOptions()
		return m, nil
	}

	return m, nil
}

func (m TypingTestModel) View() string {
	if m.state == StateOptions {
		return m.renderOptions()
	}

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
	if m.state == StateRunning && m.options.LiveWPM {
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
		b.WriteString(helpStyle.Render("enter: new test • tab: restart • esc: options • ctrl+c: quit"))
	} else {
		b.WriteString(helpStyle.Render("tab: restart • esc: options • ctrl+c: quit"))
	}

	// Show current options summary
	b.WriteString("\n")
	opts := fmt.Sprintf("layout: %s • words: %d", m.options.Layout, m.options.WordCount)
	if m.options.Punctuation {
		opts += " • punct"
	}
	if m.options.PaceCaret != PaceOff {
		switch m.options.PaceCaret {
		case PacePB:
			opts += " • pace:pb"
		case PaceAverage:
			opts += " • pace:avg"
		case PaceCustom:
			opts += fmt.Sprintf(" • pace:%.0f", m.options.CustomPaceWPM)
		}
	}
	b.WriteString(promptStyle.Render(opts))

	return b.String()
}

func (m TypingTestModel) renderOptions() string {
	var b strings.Builder

	b.WriteString(optionsTitleStyle.Render("⚙️  Options"))
	b.WriteString("\n\n")

	// Show custom WPM input if in that mode
	if m.inCustomWPMInput {
		b.WriteString(promptStyle.Render("Enter custom pace WPM:"))
		b.WriteString("\n\n")
		inputDisplay := m.customWPMInput
		if inputDisplay == "" {
			inputDisplay = "_"
		}
		b.WriteString(searchBoxStyle.Render(inputDisplay + " WPM"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("enter: confirm • esc/C-g: cancel"))
		return optionsBoxStyle.Render(b.String())
	}

	// Search box
	searchContent := m.searchQuery
	if searchContent == "" {
		searchContent = promptStyle.Render("Type to search...")
	}
	b.WriteString(searchBoxStyle.Render("🔍 " + searchContent))
	b.WriteString("\n\n")

	// Options list
	if len(m.filteredOpts) == 0 {
		b.WriteString(promptStyle.Render("No matching options"))
	} else {
		for i, opt := range m.filteredOpts {
			var line string
			prefix := "  "
			if i == m.selectedIdx {
				prefix = "▸ "
			}

			// Format value display
			var valueStr string
			switch v := opt.Value.(type) {
			case bool:
				if v {
					valueStr = valueStyle.Render("ON")
				} else {
					valueStr = promptStyle.Render("off")
				}
			case string:
				// Show custom WPM value for pace_caret when set to custom
				if opt.ID == "pace_caret" && v == "custom" {
					valueStr = valueStyle.Render(fmt.Sprintf("%s (%.0f WPM)", v, m.options.CustomPaceWPM))
				} else {
					valueStr = valueStyle.Render(v)
				}
			}

			if i == m.selectedIdx {
				line = selectedOptionStyle.Render(fmt.Sprintf("%s%-15s %s", prefix, opt.Name, valueStr))
			} else {
				line = unselectedOptionStyle.Render(fmt.Sprintf("%s%-15s ", prefix, opt.Name)) + valueStr
			}

			b.WriteString(line)
			b.WriteString("\n")

			// Show submenu if selected and in submenu mode
			if i == m.selectedIdx && m.inSubMenu && (opt.Type == "choice" || opt.Type == "submenu") {
				for j, choice := range opt.Choices {
					subPrefix := "    "
					if j == m.subMenuIdx {
						subPrefix = "  ▸ "
						b.WriteString(selectedOptionStyle.Render(subPrefix + choice))
					} else {
						b.WriteString(unselectedOptionStyle.Render(subPrefix + choice))
					}
					b.WriteString("\n")
				}
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓/C-p/C-n: navigate • enter: select • esc/tab/C-g: close • type to search"))

	return optionsBoxStyle.Render(b.String())
}

func (m TypingTestModel) renderText() string {
	var b strings.Builder

	maxWidth := m.width - 4
	if maxWidth <= 0 {
		maxWidth = 80
	}

	// Calculate pace caret position
	pacePos := -1
	if m.state == StateRunning && m.options.PaceCaret != PaceOff {
		elapsed := time.Since(m.startTime).Seconds()
		var targetWPM float64
		switch m.options.PaceCaret {
		case PacePB:
			targetWPM = m.personalBest
		case PaceAverage:
			targetWPM = m.avgWPM
		case PaceCustom:
			targetWPM = m.options.CustomPaceWPM
		}
		if targetWPM > 0 {
			// Characters per second at target WPM (5 chars per word)
			charsPerSecond := (targetWPM * 5) / 60
			pacePos = int(charsPerSecond * elapsed)
			if pacePos > len(m.targetText)-1 {
				pacePos = len(m.targetText) - 1
			}
		}
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
		} else if i == pacePos {
			b.WriteString(paceCaretStyle.Render(string(char)))
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

	pbIndicator := ""
	if wpm > m.personalBest && m.personalBest > 0 {
		pbIndicator = " 🎉 NEW PB!"
	}

	results := fmt.Sprintf(
		"%s%s\n\n%s %s\n%s %s\n%s %s\n%s %s",
		resultTitleStyle.Render("Test Complete!"),
		pbIndicator,
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

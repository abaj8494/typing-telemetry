package tui

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

var (
	// Typing test styles - initialized by themes.go
	promptStyle           lipgloss.Style
	correctStyle          lipgloss.Style
	incorrectStyle        lipgloss.Style
	cursorStyle           lipgloss.Style
	remainingStyle        lipgloss.Style
	statsBoxStyle         lipgloss.Style
	resultTitleStyle      lipgloss.Style
	resultValueStyle      lipgloss.Style
	resultLabelStyle      lipgloss.Style
	optionsTitleStyle     lipgloss.Style
	optionsBoxStyle       lipgloss.Style
	selectedOptionStyle   lipgloss.Style
	unselectedOptionStyle lipgloss.Style
	searchBoxStyle        lipgloss.Style
	valueStyle            lipgloss.Style
	paceCaretStyle        lipgloss.Style
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
	Theme         string        // Color theme
	TestType      string        // "normal" or "custom"
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

// MenuFocus tracks which part of the UI has focus
type MenuFocus int

const (
	FocusTyping MenuFocus = iota
	FocusMenuBar
)

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
	testCount     int     // Number of tests completed
	inCustomWPMInput bool   // Whether we're inputting custom WPM
	customWPMInput   string // Buffer for custom WPM input
	menuFocus     MenuFocus // Current UI focus
	menuSelection int       // Selected menu item (0=stats, 1=custom)
	showStats     bool      // Show stats panel
	lastWPM       float64   // Last test WPM (for tab restart counting)
	resultRecorded bool     // Whether current result has been recorded
	store         *storage.Store // Database storage for persistence
	customTexts   []string  // Custom text snippets
	showCustomPanel bool    // Show custom text panel
	customTextInput string  // Buffer for custom text input
	inCustomTextInput bool  // Whether we're inputting custom text
}

type tickMsg time.Time

func NewTypingTest(sourceFile string, wordCount int) TypingTestModel {
	return NewTypingTestWithStore(sourceFile, wordCount, nil)
}

func NewTypingTestWithStore(sourceFile string, wordCount int, store *storage.Store) TypingTestModel {
	if wordCount <= 0 {
		wordCount = 25
	}

	options := TestOptions{
		Layout:        "qwerty",
		LiveWPM:       true,
		WordCount:     wordCount,
		Punctuation:   true, // Enabled by default
		PaceCaret:     PaceOff,
		CustomPaceWPM: 60.0,
		Theme:         "default",
		TestType:      "normal",
	}

	allOptions := []Option{
		{
			ID:          "theme",
			Name:        "Theme",
			Description: "Color scheme",
			Type:        "choice",
			Choices:     ThemeNames,
			Value:       "default",
		},
		{
			ID:          "test_type",
			Name:        "Test Type",
			Description: "Word source for test",
			Type:        "choice",
			Choices:     []string{"normal", "custom"},
			Value:       "normal",
		},
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
			Value:       true, // Enabled by default
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
		state:         StateReady,
		sourceFile:    sourceFile,
		wordCount:     wordCount,
		options:       options,
		allOptions:    allOptions,
		filteredOpts:  allOptions,
		personalBest:  0,
		avgWPM:        50.0, // Default average
		testCount:     0,
		menuFocus:     FocusTyping,
		menuSelection: 0,
		showStats:     false,
		store:         store,
	}

	// Load stats from storage if available
	if store != nil {
		stats := store.GetTypingTestStats()
		m.personalBest = stats.PersonalBest
		m.avgWPM = stats.AverageWPM
		m.testCount = stats.TestCount
		if m.avgWPM == 0 {
			m.avgWPM = 50.0 // Default if no tests yet
		}

		// Load custom texts
		customTextsStr := store.GetTypingTestCustomTexts()
		if customTextsStr != "" {
			m.customTexts = strings.Split(customTextsStr, "\n---\n")
		}
	}

	m.targetText = m.generateText()
	return m
}

func (m *TypingTestModel) generateText() string {
	// If using custom test type and custom texts are available, use one directly
	if m.options.TestType == "custom" && len(m.customTexts) > 0 {
		// Pick a random custom text
		idx := rand.Intn(len(m.customTexts))
		text := strings.TrimSpace(m.customTexts[idx])
		if text != "" {
			return text
		}
	}

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

// deleteLastWord removes the last word from the typed string
// It deletes back to the previous space or the beginning of the string
func deleteLastWord(s string) string {
	if len(s) == 0 {
		return s
	}

	// Trim trailing spaces first
	end := len(s)
	for end > 0 && s[end-1] == ' ' {
		end--
	}

	// Find the start of the last word
	start := end
	for start > 0 && s[start-1] != ' ' {
		start--
	}

	return s[:start]
}

func (m *TypingTestModel) resetTest() {
	m.targetText = m.generateText()
	m.typed = ""
	m.state = StateReady
	m.errors = 0
	m.resultRecorded = false
	m.lastWPM = 0
}

// recordTestResult records the current test result to statistics
func (m *TypingTestModel) recordTestResult() {
	if m.state != StateFinished || m.resultRecorded {
		return
	}

	duration := m.endTime.Sub(m.startTime).Seconds()
	wordsTyped := float64(len(m.targetText)) / 5.0
	wpm := (wordsTyped / duration) * 60

	// Update local stats
	if wpm > m.personalBest {
		m.personalBest = wpm
	}
	m.testCount++
	if m.testCount == 1 {
		m.avgWPM = wpm
	} else {
		m.avgWPM = ((m.avgWPM * float64(m.testCount-1)) + wpm) / float64(m.testCount)
	}

	// Persist to database if store is available
	if m.store != nil {
		mode := storage.TypingTestMode{
			WordCount:   m.options.WordCount,
			Punctuation: m.options.Punctuation,
		}
		m.store.SaveTypingTestResultForMode(wpm, mode)
	}

	m.lastWPM = wpm
	m.resultRecorded = true
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
	// Find option index by ID for updating value
	findOptIdx := func(id string) int {
		for i, o := range m.allOptions {
			if o.ID == id {
				return i
			}
		}
		return -1
	}

	switch opt.ID {
	case "theme":
		themeName := opt.Choices[choiceIdx]
		m.options.Theme = themeName
		if idx := findOptIdx("theme"); idx >= 0 {
			m.allOptions[idx].Value = themeName
		}
		SetTheme(themeName)
	case "test_type":
		m.options.TestType = opt.Choices[choiceIdx]
		if idx := findOptIdx("test_type"); idx >= 0 {
			m.allOptions[idx].Value = opt.Choices[choiceIdx]
		}
	case "layout":
		m.options.Layout = opt.Choices[choiceIdx]
		if idx := findOptIdx("layout"); idx >= 0 {
			m.allOptions[idx].Value = opt.Choices[choiceIdx]
		}
	case "live_wpm":
		m.options.LiveWPM = !m.options.LiveWPM
		if idx := findOptIdx("live_wpm"); idx >= 0 {
			m.allOptions[idx].Value = m.options.LiveWPM
		}
	case "test_length":
		count, _ := strconv.Atoi(opt.Choices[choiceIdx])
		m.options.WordCount = count
		m.wordCount = count
		if idx := findOptIdx("test_length"); idx >= 0 {
			m.allOptions[idx].Value = opt.Choices[choiceIdx]
		}
	case "punctuation":
		m.options.Punctuation = !m.options.Punctuation
		if idx := findOptIdx("punctuation"); idx >= 0 {
			m.allOptions[idx].Value = m.options.Punctuation
		}
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
		if idx := findOptIdx("pace_caret"); idx >= 0 {
			m.allOptions[idx].Value = opt.Choices[choiceIdx]
		}
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

		// Handle stats panel
		if m.showStats {
			switch msg.Type {
			case tea.KeyCtrlC:
				return m, tea.Quit
			case tea.KeyEsc, tea.KeyEnter:
				m.showStats = false
				return m, nil
			}
			return m, nil
		}

		// Handle custom text panel
		if m.showCustomPanel {
			return m.updateCustomPanel(msg)
		}

		// Handle menubar focus
		if m.menuFocus == FocusMenuBar {
			switch msg.Type {
			case tea.KeyCtrlC:
				return m, tea.Quit
			case tea.KeyDown, tea.KeyEsc:
				m.menuFocus = FocusTyping
				return m, nil
			case tea.KeyLeft:
				if m.menuSelection > 0 {
					m.menuSelection--
				}
				return m, nil
			case tea.KeyRight:
				if m.menuSelection < 1 { // 0=Stats, 1=Custom
					m.menuSelection++
				}
				return m, nil
			case tea.KeyEnter:
				// Activate selected menu item
				if m.menuSelection == 0 {
					m.showStats = true
				} else if m.menuSelection == 1 {
					m.showCustomPanel = true
					m.inCustomTextInput = false
					m.customTextInput = ""
				}
				m.menuFocus = FocusTyping
				return m, nil
			}
			return m, nil
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyUp:
			// Move focus to menubar (only when not typing)
			if m.state != StateRunning {
				m.menuFocus = FocusMenuBar
			}
			return m, nil

		case tea.KeyEsc:
			// Open options menu
			m.state = StateOptions
			m.searchQuery = ""
			m.selectedIdx = 0
			m.inSubMenu = false
			m.filterOptions()
			return m, nil

		case tea.KeyTab:
			// If test was completed but not recorded via Enter, record it now
			if m.state == StateFinished && !m.resultRecorded {
				m.recordTestResult()
			}
			// Reset test
			m.resetTest()
			return m, nil

		case tea.KeyBackspace:
			if len(m.typed) > 0 && m.state == StateRunning {
				if msg.Alt {
					// Alt+Backspace: delete the previous word
					m.typed = deleteLastWord(m.typed)
				} else {
					// Regular backspace: delete one character
					m.typed = m.typed[:len(m.typed)-1]
				}
			}
			return m, nil

		case tea.KeyEnter:
			if m.state == StateFinished {
				// Record result if not already recorded
				if !m.resultRecorded {
					m.recordTestResult()
				}
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

				// Check if character is wrong (only count errors for target length)
				if len(m.typed) <= len(m.targetText) {
					if m.typed[len(m.typed)-1] != m.targetText[len(m.typed)-1] {
						m.errors++
					}
				} else {
					// Extra characters are always errors
					m.errors++
				}

				// Check if finished: test completes when we've typed the exact target length
				// AND the last character is correct
				if len(m.typed) == len(m.targetText) {
					// Check if last character matches
					if m.typed[len(m.typed)-1] == m.targetText[len(m.typed)-1] {
						m.state = StateFinished
						m.endTime = time.Now()
						// Auto-save result immediately on completion
						m.recordTestResult()
					}
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

func (m TypingTestModel) updateCustomPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inCustomTextInput {
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			m.inCustomTextInput = false
			m.customTextInput = ""
			return m, nil
		case tea.KeyEnter:
			// Save the custom text
			if strings.TrimSpace(m.customTextInput) != "" {
				m.customTexts = append(m.customTexts, strings.TrimSpace(m.customTextInput))
				// Persist to storage
				if m.store != nil {
					m.store.SetTypingTestCustomTexts(strings.Join(m.customTexts, "\n---\n"))
				}
			}
			m.inCustomTextInput = false
			m.customTextInput = ""
			return m, nil
		case tea.KeyBackspace:
			if len(m.customTextInput) > 0 {
				m.customTextInput = m.customTextInput[:len(m.customTextInput)-1]
			}
			return m, nil
		case tea.KeyRunes, tea.KeySpace:
			char := string(msg.Runes)
			if msg.Type == tea.KeySpace {
				char = " "
			}
			m.customTextInput += char
			return m, nil
		}
		return m, nil
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.showCustomPanel = false
		return m, nil
	case tea.KeyRunes:
		// 'a' to add new text, 'd' to delete selected
		key := string(msg.Runes)
		if key == "a" || key == "A" {
			m.inCustomTextInput = true
			m.customTextInput = ""
		} else if (key == "d" || key == "D") && len(m.customTexts) > 0 {
			// Delete last custom text
			m.customTexts = m.customTexts[:len(m.customTexts)-1]
			if m.store != nil {
				m.store.SetTypingTestCustomTexts(strings.Join(m.customTexts, "\n---\n"))
			}
		}
		return m, nil
	}
	return m, nil
}

func (m TypingTestModel) View() string {
	if m.state == StateOptions {
		return m.centerContent(m.renderOptions())
	}

	// Show stats panel if active
	if m.showStats {
		return m.centerContent(m.renderStatsPanel())
	}

	// Show custom text panel if active
	if m.showCustomPanel {
		return m.centerContent(m.renderCustomPanel())
	}

	var b strings.Builder

	// Calculate box width based on terminal width
	boxWidth := m.width - 8
	if boxWidth < 40 {
		boxWidth = 40
	}
	if boxWidth > 100 {
		boxWidth = 100
	}

	// Only show menubar when NOT running
	if m.state != StateRunning {
		b.WriteString(m.renderMenuBar())
		b.WriteString("\n\n")
	}

	// Build typing test content for the box
	var testContent strings.Builder

	// Show average pace and info ONLY before typing starts
	if m.state == StateReady {
		if m.avgWPM > 0 && m.testCount > 0 {
			testContent.WriteString(promptStyle.Render(fmt.Sprintf("Average: %.0f WPM", m.avgWPM)))
			if m.personalBest > 0 {
				testContent.WriteString(promptStyle.Render(fmt.Sprintf("  •  Best: %.0f WPM", m.personalBest)))
			}
			testContent.WriteString("\n\n")
		}
		testContent.WriteString(promptStyle.Render("Start typing to begin..."))
		testContent.WriteString("\n\n")
	} else if m.state == StateRunning {
		// Add top padding to maintain consistent box height during running
		testContent.WriteString("\n")
	}

	// Render the text with highlighting
	testContent.WriteString(m.renderText())

	// Stats during typing (always show during running to maintain consistent box height)
	if m.state == StateRunning {
		testContent.WriteString("\n\n")
		if m.options.LiveWPM {
			elapsed := time.Since(m.startTime).Seconds()
			wordsTyped := float64(len(m.typed)) / 5.0
			wpm := 0.0
			if elapsed > 0 {
				wpm = (wordsTyped / elapsed) * 60
			}

			accuracy := 100.0
			if len(m.typed) > 0 {
				correctChars := 0
				for i := 0; i < len(m.typed) && i < len(m.targetText); i++ {
					if m.typed[i] == m.targetText[i] {
						correctChars++
					}
				}
				accuracy = float64(correctChars) / float64(len(m.typed)) * 100
			}

			testContent.WriteString(fmt.Sprintf(
				"%s %.0f  %s %.0f%%",
				resultLabelStyle.Render("WPM:"),
				wpm,
				resultLabelStyle.Render("Acc:"),
				accuracy,
			))
		} else {
			// Empty line to maintain box height when LiveWPM is off
			testContent.WriteString(" ")
		}
	}

	// Final results (inside box)
	if m.state == StateFinished {
		testContent.WriteString("\n")
		testContent.WriteString(m.renderResults())
	}

	// Create the typing test box
	typingBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(CurrentTheme.Border)).
		Padding(1, 2).
		Width(boxWidth)

	b.WriteString(typingBoxStyle.Render(testContent.String()))

	// Help text OUTSIDE the box - hide during running for cleaner UI
	if m.state != StateRunning {
		b.WriteString("\n\n")
		if m.state == StateFinished {
			b.WriteString(helpStyle.Render("enter: new test • tab: restart • esc: options • ↑: menu • ctrl+c: quit"))
		} else {
			b.WriteString(helpStyle.Render("tab: restart • esc: options • ↑: menu • ctrl+c: quit"))
		}
	}

	return m.centerContent(b.String())
}

// renderMenuBar renders the top menubar
func (m TypingTestModel) renderMenuBar() string {
	// Stats button
	statsLabel := "[ Stats ]"
	if m.menuFocus == FocusMenuBar && m.menuSelection == 0 {
		statsLabel = selectedOptionStyle.Render("[ Stats ]")
	} else {
		statsLabel = promptStyle.Render("[ Stats ]")
	}

	// Custom button
	customLabel := "[ Custom ]"
	if m.menuFocus == FocusMenuBar && m.menuSelection == 1 {
		customLabel = selectedOptionStyle.Render("[ Custom ]")
	} else {
		customLabel = promptStyle.Render("[ Custom ]")
	}

	// Title
	title := titleStyle.Render(":: Typing Test")

	// Combine menubar elements
	return fmt.Sprintf("%s  %s    %s", statsLabel, customLabel, title)
}

// renderStatsPanel renders the statistics panel
func (m TypingTestModel) renderStatsPanel() string {
	var b strings.Builder

	b.WriteString(optionsTitleStyle.Render(":: Statistics"))
	b.WriteString("\n\n")

	if m.testCount == 0 {
		b.WriteString(promptStyle.Render("No tests completed yet."))
		b.WriteString("\n\n")
		b.WriteString(promptStyle.Render("Complete a typing test to see your stats!"))
	} else {
		b.WriteString(fmt.Sprintf("%s %s\n",
			resultLabelStyle.Render("Tests Completed:"),
			resultValueStyle.Render(fmt.Sprintf("%d", m.testCount))))
		b.WriteString(fmt.Sprintf("%s %s\n",
			resultLabelStyle.Render("Personal Best:"),
			resultValueStyle.Render(fmt.Sprintf("%.1f WPM", m.personalBest))))
		b.WriteString(fmt.Sprintf("%s %s\n",
			resultLabelStyle.Render("Average WPM:"),
			resultValueStyle.Render(fmt.Sprintf("%.1f WPM", m.avgWPM))))
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("enter/esc: close"))

	return optionsBoxStyle.Render(b.String())
}

// renderCustomPanel renders the custom text management panel
func (m TypingTestModel) renderCustomPanel() string {
	var b strings.Builder

	b.WriteString(optionsTitleStyle.Render(":: Custom Texts"))
	b.WriteString("\n\n")

	if m.inCustomTextInput {
		b.WriteString(promptStyle.Render("Paste or type your custom text:"))
		b.WriteString("\n\n")
		inputDisplay := m.customTextInput
		if inputDisplay == "" {
			inputDisplay = "_"
		}
		// Truncate display if too long
		if len(inputDisplay) > 60 {
			inputDisplay = inputDisplay[:57] + "..."
		}
		b.WriteString(searchBoxStyle.Render(inputDisplay))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("enter: save • esc: cancel"))
	} else {
		if len(m.customTexts) == 0 {
			b.WriteString(promptStyle.Render("No custom texts added yet."))
			b.WriteString("\n\n")
			b.WriteString(promptStyle.Render("Press 'a' to add a custom text."))
		} else {
			b.WriteString(fmt.Sprintf("%s %d\n\n",
				resultLabelStyle.Render("Custom texts:"),
				len(m.customTexts)))

			// Show preview of custom texts
			for i, text := range m.customTexts {
				preview := text
				if len(preview) > 50 {
					preview = preview[:47] + "..."
				}
				b.WriteString(fmt.Sprintf("%d. %s\n", i+1, promptStyle.Render(preview)))
				if i >= 4 { // Show max 5 texts
					if len(m.customTexts) > 5 {
						b.WriteString(fmt.Sprintf("   ... and %d more\n", len(m.customTexts)-5))
					}
					break
				}
			}
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("a: add text • d: delete last • esc: close"))
	}

	return optionsBoxStyle.Render(b.String())
}

// centerContent centers the content both horizontally and vertically
func (m TypingTestModel) centerContent(content string) string {
	if m.width == 0 || m.height == 0 {
		return content
	}

	lines := strings.Split(content, "\n")

	// Calculate max visible width of content (strip ANSI codes for width calculation)
	maxWidth := 0
	for _, line := range lines {
		// Strip ANSI escape codes for width calculation
		visibleLen := lipgloss.Width(line)
		if visibleLen > maxWidth {
			maxWidth = visibleLen
		}
	}

	// Calculate horizontal padding
	hPadding := (m.width - maxWidth) / 2
	if hPadding < 0 {
		hPadding = 0
	}

	// Add horizontal padding to each line
	paddedLines := make([]string, len(lines))
	for i, line := range lines {
		paddedLines[i] = strings.Repeat(" ", hPadding) + line
	}

	// Calculate vertical padding
	vPadding := (m.height - len(lines)) / 2
	if vPadding < 0 {
		vPadding = 0
	}

	// Add vertical padding
	var result strings.Builder
	for i := 0; i < vPadding; i++ {
		result.WriteString("\n")
	}
	result.WriteString(strings.Join(paddedLines, "\n"))

	return result.String()
}

func (m TypingTestModel) renderOptions() string {
	var b strings.Builder

	b.WriteString(optionsTitleStyle.Render(":: Options"))
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
	b.WriteString(searchBoxStyle.Render("> " + searchContent))
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

	// Use box-appropriate width for wrapping
	maxWidth := m.width - 16 // Account for box padding and borders
	if maxWidth <= 0 {
		maxWidth = 70
	}
	if maxWidth > 90 {
		maxWidth = 90
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
			charsPerSecond := (targetWPM * 5) / 60
			pacePos = int(charsPerSecond * elapsed)
			if pacePos > len(m.targetText)-1 {
				pacePos = len(m.targetText) - 1
			}
		}
	}

	target := m.targetText
	typed := m.typed

	// Split target into words for proper wrapping
	words := strings.Split(target, " ")

	lineLen := 0
	charIdx := 0

	for wordIdx, word := range words {
		wordLen := len(word)

		// Check if word would overflow - wrap to next line if needed
		// +1 for the space after the word (except last word)
		spaceNeeded := wordLen
		if wordIdx < len(words)-1 {
			spaceNeeded++
		}

		if lineLen > 0 && lineLen+spaceNeeded > maxWidth {
			b.WriteString("\n")
			lineLen = 0
		}

		// Render each character of the word
		for _, char := range word {
			if charIdx < len(typed) {
				// Character has been typed
				if typed[charIdx] == byte(char) {
					b.WriteString(correctStyle.Render(string(char)))
				} else {
					b.WriteString(incorrectStyle.Render(string(char)))
				}
			} else if charIdx == len(typed) {
				// Cursor position
				b.WriteString(cursorStyle.Render(string(char)))
			} else if charIdx == pacePos {
				b.WriteString(paceCaretStyle.Render(string(char)))
			} else {
				b.WriteString(remainingStyle.Render(string(char)))
			}
			charIdx++
			lineLen++
		}

		// Handle extra typed characters that overflow the current word
		// These push the remaining text along
		if charIdx <= len(typed) && wordIdx < len(words)-1 {
			// Check if user typed more characters than this word contains
			// Look for the space position in typed
			nextSpaceInTarget := charIdx // This is where space should be
			if nextSpaceInTarget < len(typed) {
				// User has typed past this word - check for extra chars before space
				for typedIdx := nextSpaceInTarget; typedIdx < len(typed); typedIdx++ {
					if typed[typedIdx] == ' ' {
						break
					}
					// Extra character - render as error
					b.WriteString(incorrectStyle.Render(string(typed[typedIdx])))
					lineLen++
				}
			}
		}

		// Add space between words (if not last word)
		if wordIdx < len(words)-1 {
			spaceChar := " "
			if charIdx < len(typed) {
				if typed[charIdx] == ' ' {
					b.WriteString(correctStyle.Render(spaceChar))
				} else {
					b.WriteString(incorrectStyle.Render(spaceChar))
				}
			} else if charIdx == len(typed) {
				b.WriteString(cursorStyle.Render(spaceChar))
			} else if charIdx == pacePos {
				b.WriteString(paceCaretStyle.Render(spaceChar))
			} else {
				b.WriteString(remainingStyle.Render(spaceChar))
			}
			charIdx++
			lineLen++
		}
	}

	// Render any extra characters typed beyond the target text
	if len(typed) > len(target) {
		for i := len(target); i < len(typed); i++ {
			b.WriteString(incorrectStyle.Render(string(typed[i])))
			lineLen++
			if lineLen >= maxWidth {
				b.WriteString("\n")
				lineLen = 0
			}
		}
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
		pbIndicator = " ** NEW PB! **"
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

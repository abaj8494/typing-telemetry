package tui

import (
	_ "embed"
	"strings"
)

//go:embed wordlists/english_common.txt
var englishCommonWords string

//go:embed wordlists/eff_words.txt
var effWords string

//go:embed wordlists/programming.txt
var programmingWords string

// LoadEmbeddedWordLists returns the combined word list from embedded files
func LoadEmbeddedWordLists() []string {
	var words []string

	// Parse english common words
	for _, line := range strings.Split(englishCommonWords, "\n") {
		word := strings.TrimSpace(line)
		if word != "" && len(word) >= 2 && len(word) <= 15 {
			words = append(words, word)
		}
	}

	// Parse EFF words (good quality, no offensive content)
	for _, line := range strings.Split(effWords, "\n") {
		word := strings.TrimSpace(line)
		if word != "" && len(word) >= 2 {
			words = append(words, word)
		}
	}

	// Parse programming words
	for _, line := range strings.Split(programmingWords, "\n") {
		word := strings.TrimSpace(line)
		if word != "" && len(word) >= 2 {
			words = append(words, word)
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	unique := make([]string, 0, len(words))
	for _, word := range words {
		lower := strings.ToLower(word)
		if !seen[lower] {
			seen[lower] = true
			unique = append(unique, lower)
		}
	}

	return unique
}

func init() {
	// Load word lists from embedded files
	defaultWords = LoadEmbeddedWordLists()
}

package tui

import (
	"strings"
	"testing"
)

func TestLoadEmbeddedWordLists(t *testing.T) {
	words := LoadEmbeddedWordLists()

	if len(words) == 0 {
		t.Error("LoadEmbeddedWordLists returned empty word list")
	}

	// Should have a reasonable number of words (combining all sources)
	if len(words) < 1000 {
		t.Errorf("Expected at least 1000 words, got %d", len(words))
	}

	t.Logf("Loaded %d unique words", len(words))
}

func TestWordListsAreUnique(t *testing.T) {
	words := LoadEmbeddedWordLists()

	seen := make(map[string]bool)
	for _, word := range words {
		lower := strings.ToLower(word)
		if seen[lower] {
			t.Errorf("Duplicate word found: %q", word)
		}
		seen[lower] = true
	}
}

func TestWordListsAreLowercase(t *testing.T) {
	words := LoadEmbeddedWordLists()

	for _, word := range words {
		if word != strings.ToLower(word) {
			t.Errorf("Word is not lowercase: %q", word)
		}
	}
}

func TestWordListsHaveValidLength(t *testing.T) {
	words := LoadEmbeddedWordLists()

	for _, word := range words {
		if len(word) < 2 {
			t.Errorf("Word is too short (< 2 chars): %q", word)
		}
	}
}

func TestDefaultWordsIsPopulated(t *testing.T) {
	// After init(), defaultWords should be populated
	if len(defaultWords) == 0 {
		t.Error("defaultWords is empty after init()")
	}
}

func TestWordListContainsCommonWords(t *testing.T) {
	words := LoadEmbeddedWordLists()
	wordSet := make(map[string]bool)
	for _, w := range words {
		wordSet[w] = true
	}

	// Check for some common English words that should be present
	commonWords := []string{"the", "and", "is", "it", "to", "of", "in", "for", "on", "with"}
	for _, common := range commonWords {
		if !wordSet[common] {
			t.Errorf("Common word %q not found in word list", common)
		}
	}
}

func TestWordListContainsProgrammingTerms(t *testing.T) {
	words := LoadEmbeddedWordLists()
	wordSet := make(map[string]bool)
	for _, w := range words {
		wordSet[w] = true
	}

	// Check for programming terms
	programmingTerms := []string{"function", "variable", "class", "method", "return", "import"}
	for _, term := range programmingTerms {
		if !wordSet[term] {
			t.Errorf("Programming term %q not found in word list", term)
		}
	}
}

func TestEmbeddedFilesExist(t *testing.T) {
	// Check that embedded file variables are not empty
	if len(englishCommonWords) == 0 {
		t.Error("englishCommonWords is empty")
	}
	if len(effWords) == 0 {
		t.Error("effWords is empty")
	}
	if len(programmingWords) == 0 {
		t.Error("programmingWords is empty")
	}

	t.Logf("englishCommonWords: %d bytes", len(englishCommonWords))
	t.Logf("effWords: %d bytes", len(effWords))
	t.Logf("programmingWords: %d bytes", len(programmingWords))
}

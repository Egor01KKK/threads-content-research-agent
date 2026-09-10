package research

import (
	"regexp"
	"strings"
	"unicode"
)

var firstPersonRe = regexp.MustCompile(`(?i)(^|[^\p{L}])(i|i'm|i’ve|i've|me|my|mine|we|we’re|we're|our|ours|us)([^\p{L}]|$)`)
var urlRe = regexp.MustCompile(`(?i)https?://[^\s]+`)

// CalculateMechanics extracts only observable writing mechanics. It never
// infers a semantic label or uses engagement/performance data.
func CalculateMechanics(text string) WritingMechanics {
	runes := []rune(text)
	mechanics := WritingMechanics{
		CharacterCount:     len(runes),
		WordCount:          countWords(runes),
		SentenceCount:      countSentences(runes),
		LineCount:          countLines(text),
		QuestionCount:      strings.Count(text, "?") + strings.Count(text, "？"),
		EmojiCount:         countEmoji(runes),
		FirstPersonPresent: firstPersonRe.MatchString(text),
		NumbersPresent:     containsNumber(runes),
		URLPresent:         urlRe.MatchString(text),
	}
	if mechanics.SentenceCount > 0 {
		mechanics.AverageSentenceLength = float64(mechanics.WordCount) / float64(mechanics.SentenceCount)
	}
	if mechanics.CharacterCount > 0 {
		mechanics.LineBreakDensity = float64(countLineBreaks(runes)) / float64(mechanics.CharacterCount)
	}
	return mechanics
}

func countWords(runes []rune) int {
	count := 0
	inWord := false
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !inWord {
				count++
				inWord = true
			}
			continue
		}
		inWord = false
	}
	return count
}

func countSentences(runes []rune) int {
	count := 0
	inTerminator := false
	for index, r := range runes {
		// A dot inside a URL, domain, decimal, or token such as "v1.2" is
		// not a sentence boundary.
		if r == '.' && index > 0 && index+1 < len(runes) &&
			(unicode.IsLetter(runes[index-1]) || unicode.IsDigit(runes[index-1])) &&
			(unicode.IsLetter(runes[index+1]) || unicode.IsDigit(runes[index+1])) {
			continue
		}
		if isSentenceTerminator(r) {
			if !inTerminator {
				count++
				inTerminator = true
			}
			continue
		}
		if !unicode.IsSpace(r) {
			inTerminator = false
		}
	}
	if count == 0 && countWords(runes) > 0 {
		return 1
	}
	return count
}

func isSentenceTerminator(r rune) bool {
	switch r {
	case '.', '!', '?', '。', '！', '？':
		return true
	default:
		return false
	}
}

func countLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func countLineBreaks(runes []rune) int {
	count := 0
	for _, r := range runes {
		if r == '\n' {
			count++
		}
	}
	return count
}

func containsNumber(runes []rune) bool {
	for _, r := range runes {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func countEmoji(runes []rune) int {
	count := 0
	for _, r := range runes {
		if isEmojiRune(r) {
			count++
		}
	}
	return count
}

func isEmojiRune(r rune) bool {
	return (r >= 0x1F000 && r <= 0x1FAFF) ||
		(r >= 0x2600 && r <= 0x27BF) ||
		(r >= 0x2300 && r <= 0x23FF)
}

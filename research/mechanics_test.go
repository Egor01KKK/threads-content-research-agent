package research

import "testing"

func TestCalculateMechanicsIsDeterministicAndObservable(t *testing.T) {
	text := "I built 2 things.\nYou can use https://example.com? Wow 😄"
	mechanics := CalculateMechanics(text)
	if mechanics.CharacterCount != len([]rune(text)) {
		t.Fatalf("character count = %d, want %d", mechanics.CharacterCount, len([]rune(text)))
	}
	if mechanics.WordCount == 0 || mechanics.WordCount < 8 {
		t.Errorf("word count = %d, want a non-trivial unicode word count", mechanics.WordCount)
	}
	if mechanics.SentenceCount != 2 {
		t.Errorf("sentence count = %d, want 2", mechanics.SentenceCount)
	}
	if mechanics.AverageSentenceLength != float64(mechanics.WordCount)/2 {
		t.Errorf("average sentence length = %v", mechanics.AverageSentenceLength)
	}
	if mechanics.LineCount != 2 || mechanics.LineBreakDensity <= 0 {
		t.Errorf("line mechanics = %+v", mechanics)
	}
	if mechanics.QuestionCount != 1 || mechanics.EmojiCount != 1 {
		t.Errorf("question/emoji counts = %d/%d", mechanics.QuestionCount, mechanics.EmojiCount)
	}
	if !mechanics.FirstPersonPresent || !mechanics.NumbersPresent || !mechanics.URLPresent {
		t.Errorf("presence flags = %+v", mechanics)
	}

	if empty := CalculateMechanics(""); empty != (WritingMechanics{}) {
		t.Errorf("empty mechanics = %+v, want zero value", empty)
	}
	if noFirstPerson := CalculateMechanics("The team shipped a release."); noFirstPerson.FirstPersonPresent {
		t.Error("third-person text was marked first-person")
	}
}

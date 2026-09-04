package model

import "strings"

// ThaiEntry is one English-to-Thai dictionary sense.
type ThaiEntry struct {
	ID           int
	English      string
	Thai         string
	PartOfSpeech string
	Related      string
	Synonyms     string
	Antonyms     string
}

// NormalizeEnglish preserves punctuation within headwords while normalizing
// case and whitespace, including line breaks in a selected phrase.
func NormalizeEnglish(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

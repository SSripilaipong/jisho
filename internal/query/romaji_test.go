package query

import "testing"

func TestToHiragana(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// Basic kana
		{"ha", "は"},
		{"ki", "き"},
		// Doubled consonants → っ (geminate)
		{"hakki", "はっき"},
		{"kitte", "きって"},
		{"zasshi", "ざっし"},
		{"motto", "もっと"},
		{"kippu", "きっぷ"},
		// nn → ん
		{"nn", "ん"},
		// Long vowels
		{"aa", "ああ"},
		// Non-romaji returns empty
		{"hello!", ""},
	}
	for _, tc := range tests {
		got := toHiragana(tc.in)
		if got != tc.want {
			t.Errorf("toHiragana(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

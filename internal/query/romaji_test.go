package query

import "testing"

// TestRomajiWildcardConversion verifies the kana conversion used for prefix/suffix
// romaji wildcard queries (e.g. "tabe*" and "*tabe").
func TestRomajiWildcardConversion(t *testing.T) {
	tests := []struct {
		romaji      string
		wantHira    string
		wantHiraRev string
		wantKata    string
		wantKataRev string
	}{
		{"tabe", "たべ", "べた", "タベ", "ベタ"},
		{"ta", "た", "た", "タ", "タ"},
		{"shi", "し", "し", "シ", "シ"},
		{"tsu", "つ", "つ", "ツ", "ツ"},
		{"kitte", "きって", "てっき", "キッテ", "テッキ"},
	}
	for _, tc := range tests {
		hira := toHiragana(tc.romaji)
		if hira != tc.wantHira {
			t.Errorf("toHiragana(%q) = %q, want %q", tc.romaji, hira, tc.wantHira)
		}
		hiraRev := reverseRunes(hira)
		if hiraRev != tc.wantHiraRev {
			t.Errorf("reverseRunes(toHiragana(%q)) = %q, want %q", tc.romaji, hiraRev, tc.wantHiraRev)
		}
		kata := toKatakana(hira)
		if kata != tc.wantKata {
			t.Errorf("toKatakana(toHiragana(%q)) = %q, want %q", tc.romaji, kata, tc.wantKata)
		}
		kataRev := reverseRunes(kata)
		if kataRev != tc.wantKataRev {
			t.Errorf("reverseRunes(toKatakana(toHiragana(%q))) = %q, want %q", tc.romaji, kataRev, tc.wantKataRev)
		}
	}
}

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
		// Long vowels
		{"aa", "ああ"},
		// n before vowel: first n → ん (before consonant), second n starts syllable
		{"nna", "んな"},
		{"nni", "んに"},
		{"nnu", "んぬ"},
		{"nne", "んね"},
		{"nno", "んの"},
		// n before y compound
		{"nnya", "んにゃ"},
		{"nnyu", "んにゅ"},
		{"nnyo", "んにょ"},
		// Realistic words
		{"shinnyo", "しんにょ"},
		{"shinna", "しんな"},
		{"kanna", "かんな"},
		{"kanni", "かんに"},
		{"sennyu", "せんにゅ"},
		// Apostrophe as ん-separator
		{"shina", "しな"},
		{"shin'a", "しんあ"},
		{"ren'ai", "れんあい"},
		{"renai", "れない"},
		{"shin'you", "しんよう"},
		{"kan'i", "かんい"},
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

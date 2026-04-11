package query

import "strings"

// toHiragana converts a Hepburn romaji string to hiragana.
// Returns empty string if the input cannot be fully converted (i.e. it's not romaji).
func toHiragana(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var out []rune
	i := 0
	runes := []rune(s)
	n := len(runes)
	for i < n {
		// Try longest match first (3 chars, then 2, then 1).
		matched := false
		for l := 3; l >= 1; l-- {
			if i+l > n {
				continue
			}
			chunk := string(runes[i : i+l])
			if h, ok := romajiMap[chunk]; ok {
				out = append(out, []rune(h)...)
				i += l
				matched = true
				break
			}
		}
		if !matched {
			// Doubled consonant (e.g. "kk", "tt") → っ + continue.
			// "nn" is already in the map, so exclude 'n' here.
			if i+1 < n && runes[i] == runes[i+1] && runes[i] != 'n' {
				out = append(out, []rune("っ")...)
				i++
				continue
			}
			// Non-romaji character encountered — give up.
			return ""
		}
	}
	return string(out)
}

// toKatakana shifts hiragana codepoints to katakana (offset +96).
func toKatakana(hiragana string) string {
	runes := []rune(hiragana)
	for i, r := range runes {
		if r >= 0x3041 && r <= 0x3096 {
			runes[i] = r + 0x60
		}
	}
	return string(runes)
}

// romajiMap is a Hepburn romanization → hiragana mapping.
// Longer sequences must be tried before shorter ones (handled in toHiragana).
var romajiMap = map[string]string{
	// Three-character sequences
	"shi": "し", "chi": "ち", "tsu": "つ",
	"sha": "しゃ", "shu": "しゅ", "sho": "しょ",
	"cha": "ちゃ", "chu": "ちゅ", "cho": "ちょ",
	"tya": "ちゃ", "tyu": "ちゅ", "tyo": "ちょ",
	"dzu": "づ", "dji": "ぢ",
	"kya": "きゃ", "kyu": "きゅ", "kyo": "きょ",
	"nya": "にゃ", "nyu": "にゅ", "nyo": "にょ",
	"hya": "ひゃ", "hyu": "ひゅ", "hyo": "ひょ",
	"mya": "みゃ", "myu": "みゅ", "myo": "みょ",
	"rya": "りゃ", "ryu": "りゅ", "ryo": "りょ",
	"gya": "ぎゃ", "gyu": "ぎゅ", "gyo": "ぎょ",
	"zya": "じゃ", "zyu": "じゅ", "zyo": "じょ",
	"bya": "びゃ", "byu": "びゅ", "byo": "びょ",
	"pya": "ぴゃ", "pyu": "ぴゅ", "pyo": "ぴょ",
	"dya": "ぢゃ", "dyu": "ぢゅ", "dyo": "ぢょ",

	// Two-character sequences
	"ka": "か", "ki": "き", "ku": "く", "ke": "け", "ko": "こ",
	"sa": "さ", "si": "し", "su": "す", "se": "せ", "so": "そ",
	"ta": "た", "ti": "ち", "te": "て", "to": "と",
	"na": "な", "ni": "に", "nu": "ぬ", "ne": "ね", "no": "の",
	"ha": "は", "hi": "ひ", "fu": "ふ", "he": "へ", "ho": "ほ",
	"ma": "ま", "mi": "み", "mu": "む", "me": "め", "mo": "も",
	"ya": "や", "yu": "ゆ", "yo": "よ",
	"ra": "ら", "ri": "り", "ru": "る", "re": "れ", "ro": "ろ",
	"wa": "わ", "wi": "ゐ", "we": "ゑ", "wo": "を",
	"ga": "が", "gi": "ぎ", "gu": "ぐ", "ge": "げ", "go": "ご",
	"za": "ざ", "zi": "じ", "zu": "ず", "ze": "ぜ", "zo": "ぞ",
	"da": "だ", "di": "ぢ", "de": "で", "do": "ど",
	"ba": "ば", "bi": "び", "bu": "ぶ", "be": "べ", "bo": "ぼ",
	"pa": "ぱ", "pi": "ぴ", "pu": "ぷ", "pe": "ぺ", "po": "ぽ",
	"ja": "じゃ", "ji": "じ", "ju": "じゅ", "jo": "じょ",
	"hu": "ふ",
	"tu": "つ",
	// Long vowels / doubled consonants
	"aa": "ああ", "ii": "いい", "uu": "うう", "ee": "ええ", "oo": "おお",
	// n before consonant or end
	"nn": "ん",

	// Single characters
	"a": "あ", "i": "い", "u": "う", "e": "え", "o": "お",
	"n": "ん",
}

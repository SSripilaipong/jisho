package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/shsnail/jisho/internal/model"
)

// PrintWords writes a formatted word list to w.
func PrintWords(w io.Writer, words []model.Word) {
	if len(words) == 0 {
		fmt.Fprintln(w, "No results found.")
		return
	}
	for i, word := range words {
		if i > 0 {
			fmt.Fprintln(w)
		}
		printWord(w, word)
	}
}

func printWord(w io.Writer, word model.Word) {
	// Headword line: primary kanji + primary kana reading.
	kanji := primaryText(word.Kanji)
	kana := primaryKanaText(word.Kana)

	head := kanji
	if head == "" {
		head = kana
	}
	fmt.Fprintf(w, "%s", bold(w, head))
	if kanji != "" && kana != "" && kana != kanji {
		fmt.Fprintf(w, "  %s", dim(w, "【"+kana+"】"))
	}

	// Tags line.
	var tags []string
	if word.IsCommon {
		tags = append(tags, "common")
	}
	if word.JLPTLevel > 0 {
		tags = append(tags, fmt.Sprintf("JLPT N%d", word.JLPTLevel))
	}
	if len(tags) > 0 {
		fmt.Fprintf(w, "  [%s]", strings.Join(tags, ", "))
	}
	fmt.Fprintln(w)

	// Senses.
	for i, sense := range word.Senses {
		// Part of speech header (print when it changes from previous sense).
		if len(sense.PartOfSpeech) > 0 {
			fmt.Fprintf(w, "  %s\n", dim(w, strings.Join(sense.PartOfSpeech, ", ")))
		}
		// English glosses for this sense.
		var glosses []string
		for _, g := range sense.Gloss {
			if g.Lang == "eng" {
				glosses = append(glosses, g.Text)
			}
		}
		if len(glosses) > 0 {
			fmt.Fprintf(w, "  %d. %s\n", i+1, strings.Join(glosses, "; "))
		}
		// Extra info.
		if len(sense.Info) > 0 {
			fmt.Fprintf(w, "     %s\n", dim(w, strings.Join(sense.Info, " ")))
		}
	}

	// Alternate kanji forms (beyond the primary).
	altKanji := altTexts(word.Kanji)
	if len(altKanji) > 0 {
		fmt.Fprintf(w, "  %s %s\n", dim(w, "also:"), strings.Join(altKanji, "、"))
	}
}

func primaryText(forms []model.KanjiForm) string {
	if len(forms) == 0 {
		return ""
	}
	return forms[0].Text
}

func primaryKanaText(forms []model.KanaForm) string {
	if len(forms) == 0 {
		return ""
	}
	return forms[0].Text
}

func altTexts(forms []model.KanjiForm) []string {
	if len(forms) <= 1 {
		return nil
	}
	out := make([]string, len(forms)-1)
	for i, f := range forms[1:] {
		out[i] = f.Text
	}
	return out
}

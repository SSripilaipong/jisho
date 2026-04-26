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

	// Alternate forms (all valid kanji+kana pairs beyond the primary).
	alts := buildAltForms(word.Kanji, word.Kana)
	if len(alts) > 0 {
		fmt.Fprintf(w, "  %s %s\n", dim(w, "also:"), strings.Join(alts, "、"))
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

func kanaApplies(appliesToKanji []string, kanjiText string) bool {
	if len(appliesToKanji) == 0 {
		return true
	}
	for _, k := range appliesToKanji {
		if k == "*" || k == kanjiText {
			return true
		}
	}
	return false
}

func buildAltForms(kanji []model.KanjiForm, kana []model.KanaForm) []string {
	if len(kanji) == 0 {
		if len(kana) <= 1 {
			return nil
		}
		out := make([]string, len(kana)-1)
		for i, f := range kana[1:] {
			out[i] = f.Text
		}
		return out
	}

	primaryKanji := kanji[0].Text
	primaryKana := ""
	if len(kana) > 0 {
		primaryKana = kana[0].Text
	}

	var out []string
	for _, k := range kanji {
		for _, n := range kana {
			if k.Text == primaryKanji && n.Text == primaryKana {
				continue
			}
			if kanaApplies(n.AppliesToKanji, k.Text) {
				out = append(out, k.Text+" 【"+n.Text+"】")
			}
		}
	}
	return out
}

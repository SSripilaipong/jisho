package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/shsnail/jisho/internal/model"
)

// PrintKanji writes a formatted kanji entry to w.
func PrintKanji(w io.Writer, k *model.Kanji) {
	if k == nil {
		fmt.Fprintln(w, "Kanji not found.")
		return
	}

	fmt.Fprintf(w, "%s\n", bold(w, k.Literal))

	if len(k.OnReadings) > 0 {
		fmt.Fprintf(w, "  On:      %s\n", strings.Join(k.OnReadings, "、"))
	}
	if len(k.KunReadings) > 0 {
		fmt.Fprintf(w, "  Kun:     %s\n", strings.Join(k.KunReadings, "、"))
	}
	if len(k.Nanori) > 0 {
		fmt.Fprintf(w, "  Nanori:  %s\n", strings.Join(k.Nanori, "、"))
	}
	if len(k.MeaningsEN) > 0 {
		fmt.Fprintf(w, "  Meaning: %s\n", strings.Join(k.MeaningsEN, ", "))
	}

	var meta []string
	if k.JLPTLevel > 0 {
		meta = append(meta, fmt.Sprintf("JLPT N%d", k.JLPTLevel))
	}
	if k.Grade > 0 {
		meta = append(meta, fmt.Sprintf("grade %d", k.Grade))
	}
	if k.StrokeCount > 0 {
		meta = append(meta, fmt.Sprintf("%d strokes", k.StrokeCount))
	}
	if k.Frequency > 0 {
		meta = append(meta, fmt.Sprintf("freq #%d", k.Frequency))
	}
	if len(meta) > 0 {
		fmt.Fprintf(w, "  %s\n", strings.Join(meta, "  "))
	}

	if len(k.Radicals) > 0 {
		fmt.Fprintf(w, "  Radicals: %s\n", strings.Join(k.Radicals, " "))
	}
}

// PrintKanjiList writes a compact list of kanji (for radical search results).
func PrintKanjiList(w io.Writer, kanji []model.Kanji) {
	if len(kanji) == 0 {
		fmt.Fprintln(w, "No kanji found.")
		return
	}
	for i, k := range kanji {
		if i > 0 {
			fmt.Fprintln(w)
		}
		PrintKanji(w, &k)
	}
}
